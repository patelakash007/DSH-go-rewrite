package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const (
	maxTerminalsPerSession = 4
	maxTerminalScreenBytes = 2 << 20
	maxTerminalInputBytes  = 64 << 10
)

var terminalIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type terminalRecord struct {
	id        string
	sessionID string
	title     string
	shellPath string
	cwd       string
	cols      int
	rows      int

	cmd      *exec.Cmd
	stdin    io.WriteCloser
	master   *os.File
	masterFD int
	pty      bool
	cancel   context.CancelFunc
	done     chan struct{}

	mu         sync.Mutex
	screen     []byte
	sequence   int64
	state      string
	exitCode   *int
	failure    string
	controller string
	subs       map[string]chan map[string]any
	closed     bool
}

type terminalAttachment struct {
	record *terminalRecord
	key    string
	ch     chan map[string]any
}

func (s *Server) dispatchTerminalRPC(ctx context.Context, endpoint string, args map[string]any) (any, *RPCError) {
	sessionID, _ := args["agentId"].(string)
	if sessionID == "" {
		sessionID, _ = args["sessionId"].(string)
	}
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	rec.mu.Lock()
	cwd := rec.Cwd
	rec.mu.Unlock()

	switch endpoint {
	case "terminal/environment":
		return map[string]any{
			"cwd": cwd, "maxInputBytes": maxTerminalInputBytes, "maxBufferedBytes": 2 << 20,
			"maxCols": 500, "maxRows": 200, "scrollback": 1000,
		}, nil
	case "terminal/shells":
		path := nativeSystemShell()
		return []map[string]any{{"path": path, "name": "sh", "args": []any{}}}, nil
	case "terminal/list":
		return s.listTerminals(sessionID), nil
	case "terminal/create":
		request, _ := args["request"].(map[string]any)
		id, _ := request["id"].(string)
		if !terminalIDPattern.MatchString(id) {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "invalid terminal id"}
		}
		cols := clampTerminal(numberArg(request, "cols", 80), 2, 500)
		rows := clampTerminal(numberArg(request, "rows", 24), 1, 200)
		shellPath, _ := request["shellPath"].(string)
		if shellPath == "" {
			shellPath = nativeSystemShell()
		}
		if shellPath != nativeSystemShell() {
			return nil, &RPCError{Code: "terminal/unavailable", Message: "requested shell is not available in the native engine"}
		}
		terminal, rpcErr := s.createTerminal(ctx, sessionID, id, shellPath, cwd, cols, rows)
		if rpcErr != nil {
			return nil, rpcErr
		}
		return terminal.info(), nil
	case "terminal/rename":
		id, _ := args["id"].(string)
		title, _ := args["title"].(string)
		title = trimTerminalTitle(title)
		if title == "" {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "terminal title is empty"}
		}
		terminal := s.terminal(sessionID, id)
		if terminal == nil {
			return nil, &RPCError{Code: "terminal/unavailable", Message: "terminal not found"}
		}
		terminal.mu.Lock()
		terminal.title = title
		terminal.mu.Unlock()
		terminal.broadcastState()
		return map[string]any{}, nil
	case "terminal/close":
		id, _ := args["id"].(string)
		s.closeTerminal(sessionID, id)
		return map[string]any{}, nil
	case "terminal/write":
		id, _ := args["id"].(string)
		attachmentID, _ := args["attachmentId"].(string)
		data, _ := args["data"].(string)
		terminal := s.terminal(sessionID, id)
		if terminal == nil {
			return nil, &RPCError{Code: "terminal/unavailable", Message: "terminal not found"}
		}
		if err := terminal.writeFrom(attachmentID, data); err != nil {
			return nil, terminalRPCError(err)
		}
		return map[string]any{}, nil
	case "terminal/resize":
		id, _ := args["id"].(string)
		attachmentID, _ := args["attachmentId"].(string)
		terminal := s.terminal(sessionID, id)
		if terminal == nil {
			return nil, &RPCError{Code: "terminal/unavailable", Message: "terminal not found"}
		}
		if err := terminal.resizeFrom(attachmentID, numberArg(args, "cols", 80), numberArg(args, "rows", 24)); err != nil {
			return nil, terminalRPCError(err)
		}
		return map[string]any{}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown terminal method"}
	}
}

func (s *Server) createTerminal(_ context.Context, sessionID, id, shellPath, cwd string, cols, rows int) (*terminalRecord, *RPCError) {
	key := sessionID + "\x00" + id
	s.terminalMu.Lock()
	if existing := s.terminals[sessionID][id]; existing != nil {
		s.terminalMu.Unlock()
		return existing, nil
	}
	if s.closedTerminalIDs[key] {
		s.terminalMu.Unlock()
		return nil, &RPCError{Code: "terminal/unavailable", Message: "a closed terminal id cannot be reused"}
	}
	if len(s.terminals[sessionID]) >= maxTerminalsPerSession {
		s.terminalMu.Unlock()
		return nil, &RPCError{Code: "terminal/limit-reached", Message: "terminal limit reached"}
	}
	if s.terminals[sessionID] == nil {
		s.terminals[sessionID] = map[string]*terminalRecord{}
	}
	runCtx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(runCtx, shellPath)
	command.Dir = cwd
	command.Env = nativeTerminalEnvironment(cwd)
	terminal := &terminalRecord{
		id: id, sessionID: sessionID, title: "sh", shellPath: shellPath, cwd: cwd,
		cols: cols, rows: rows, cmd: command, cancel: cancel,
		done: make(chan struct{}), state: "running", subs: map[string]chan map[string]any{},
	}
	var (
		reader io.Reader
		writer *io.PipeWriter
		slave  *os.File
	)
	master, slaveFile, ptyErr := openPTY()
	if ptyErr == nil {
		_ = setPTYSize(master, cols, rows)
		command.Stdin = slaveFile
		command.Stdout = slaveFile
		command.Stderr = slaveFile
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
		terminal.master = master
		terminal.masterFD = int(master.Fd())
		terminal.pty = true
		terminal.stdin = master
		reader = master
		slave = slaveFile
	} else {
		if runtime.GOOS != "windows" {
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		}
		stdin, err := command.StdinPipe()
		if err != nil {
			cancel()
			s.terminalMu.Unlock()
			return nil, &RPCError{Code: "terminal/unavailable", Message: err.Error()}
		}
		pipeReader, pipeWriter := io.Pipe()
		command.Stdout = pipeWriter
		command.Stderr = pipeWriter
		terminal.stdin = stdin
		reader = pipeReader
		writer = pipeWriter
	}
	if err := command.Start(); err != nil {
		cancel()
		if slave != nil {
			_ = slave.Close()
		}
		if master != nil {
			_ = master.Close()
		}
		if closer, ok := reader.(io.Closer); ok && reader != master {
			_ = closer.Close()
		}
		if writer != nil {
			_ = writer.Close()
		}
		s.terminalMu.Unlock()
		return nil, &RPCError{Code: "terminal/unavailable", Message: err.Error()}
	}
	if slave != nil {
		_ = slave.Close()
	}
	s.terminals[sessionID][id] = terminal
	s.terminalMu.Unlock()
	readDone := make(chan struct{})
	go terminal.readOutput(reader, readDone)
	go terminal.wait(writer, readDone)
	return terminal, nil
}

func (s *Server) terminal(sessionID, id string) *terminalRecord {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	return s.terminals[sessionID][id]
}

func (s *Server) listTerminals(sessionID string) []map[string]any {
	s.terminalMu.Lock()
	terminals := make([]*terminalRecord, 0, len(s.terminals[sessionID]))
	for _, terminal := range s.terminals[sessionID] {
		terminals = append(terminals, terminal)
	}
	s.terminalMu.Unlock()
	sort.Slice(terminals, func(i, j int) bool { return terminals[i].id < terminals[j].id })
	out := make([]map[string]any, 0, len(terminals))
	for _, terminal := range terminals {
		out = append(out, terminal.info())
	}
	return out
}

func (s *Server) closeTerminal(sessionID, id string) {
	key := sessionID + "\x00" + id
	s.terminalMu.Lock()
	terminal := s.terminals[sessionID][id]
	if terminal != nil {
		delete(s.terminals[sessionID], id)
		s.closedTerminalIDs[key] = true
	}
	s.terminalMu.Unlock()
	if terminal != nil {
		terminal.close()
		terminal.endSubscriptions()
	}
}

func (s *Server) closeAllTerminals() {
	s.terminalMu.Lock()
	var all []*terminalRecord
	for _, byID := range s.terminals {
		for _, terminal := range byID {
			all = append(all, terminal)
		}
	}
	s.terminals = map[string]map[string]*terminalRecord{}
	s.terminalMu.Unlock()
	for _, terminal := range all {
		terminal.close()
		terminal.endSubscriptions()
	}
}

func (s *Server) closeSessionTerminals(sessionID string) {
	s.terminalMu.Lock()
	byID := s.terminals[sessionID]
	delete(s.terminals, sessionID)
	for id := range byID {
		s.closedTerminalIDs[sessionID+"\x00"+id] = true
	}
	s.terminalMu.Unlock()
	for _, terminal := range byID {
		terminal.close()
		terminal.endSubscriptions()
	}
}

func (t *terminalRecord) info() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.infoLocked()
}

func (t *terminalRecord) infoLocked() map[string]any {
	info := map[string]any{
		"id": t.id, "title": t.title,
		"shell": map[string]any{"path": t.shellPath, "name": "sh", "args": []any{}},
		"cwd":   t.cwd, "cols": t.cols, "rows": t.rows, "state": t.state,
	}
	if t.exitCode != nil {
		info["exitCode"] = *t.exitCode
	}
	if t.failure != "" {
		info["error"] = t.failure
	}
	return info
}

func (t *terminalRecord) readOutput(reader io.Reader, done chan<- struct{}) {
	defer close(done)
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	buffer := make([]byte, 32<<10)
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			t.appendOutput(buffer[:read])
		}
		if err != nil {
			return
		}
	}
}

func (t *terminalRecord) appendOutput(data []byte) {
	t.mu.Lock()
	t.screen = append(t.screen, data...)
	if len(t.screen) > maxTerminalScreenBytes {
		t.screen = append([]byte(nil), t.screen[len(t.screen)-maxTerminalScreenBytes:]...)
	}
	t.sequence++
	sequence := t.sequence
	text := string(data)
	subs := copyTerminalSubs(t.subs)
	t.mu.Unlock()
	frame := map[string]any{"type": "output", "sequence": sequence, "data": text}
	for _, subscriber := range subs {
		select {
		case subscriber <- frame:
		default:
		}
	}
}

func (t *terminalRecord) wait(writer *io.PipeWriter, readDone <-chan struct{}) {
	err := t.cmd.Wait()
	if writer != nil {
		_ = writer.Close()
	}
	<-readDone
	exitCode := 0
	state := "exited"
	failure := ""
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			exitCode = exit.ExitCode()
		} else {
			exitCode = -1
			state = "failed"
			failure = err.Error()
		}
	}
	t.mu.Lock()
	t.state = state
	t.exitCode = &exitCode
	t.failure = failure
	t.mu.Unlock()
	t.broadcastState()
	close(t.done)
}

func (t *terminalRecord) attach(key string) terminalAttachment {
	channel := make(chan map[string]any, 128)
	t.mu.Lock()
	t.subs[key] = channel
	t.controller = key
	snapshot := map[string]any{
		"type": "snapshot", "sequence": t.sequence, "screen": string(t.screen),
		"info": t.infoLocked(), "writable": t.state == "running",
		"inputWindow": map[string]any{"maxBytes": 128 << 10, "frameOverheadBytes": 128},
	}
	t.mu.Unlock()
	channel <- snapshot
	t.broadcastState()
	return terminalAttachment{record: t, key: key, ch: channel}
}

func (t *terminalRecord) detach(key string) {
	t.mu.Lock()
	channel := t.subs[key]
	delete(t.subs, key)
	if t.controller == key {
		t.controller = ""
	}
	t.mu.Unlock()
	if channel != nil {
		select {
		case channel <- map[string]any{"type": "__end"}:
		default:
		}
	}
	t.broadcastState()
}

func (t *terminalRecord) endSubscriptions() {
	t.mu.Lock()
	channels := copyTerminalSubs(t.subs)
	t.subs = map[string]chan map[string]any{}
	t.controller = ""
	t.mu.Unlock()
	for _, channel := range channels {
		select {
		case channel <- map[string]any{"type": "__end"}:
		default:
		}
	}
}

func (t *terminalRecord) broadcastState() {
	t.mu.Lock()
	info := t.infoLocked()
	controller := t.controller
	subs := make(map[string]chan map[string]any, len(t.subs))
	for key, channel := range t.subs {
		subs[key] = channel
	}
	t.mu.Unlock()
	for key, subscriber := range subs {
		frame := map[string]any{"type": "state", "info": info, "writable": key == controller && info["state"] == "running"}
		select {
		case subscriber <- frame:
		default:
		}
	}
}

func (t *terminalRecord) writeFrom(controller, data string) error {
	if len([]byte(data)) > maxTerminalInputBytes {
		return fmt.Errorf("terminal input exceeds %d bytes", maxTerminalInputBytes)
	}
	t.mu.Lock()
	if t.state != "running" {
		t.mu.Unlock()
		return errors.New("terminal is not running")
	}
	if controller != "" && t.controller != controller {
		t.mu.Unlock()
		return errors.New("terminal attachment is read-only")
	}
	stdin := t.stdin
	t.mu.Unlock()
	_, err := io.WriteString(stdin, data)
	return err
}

func (t *terminalRecord) resizeFrom(controller string, cols, rows int) error {
	t.mu.Lock()
	if controller != "" && t.controller != controller {
		t.mu.Unlock()
		return errors.New("terminal attachment is read-only")
	}
	t.cols = clampTerminal(cols, 2, 500)
	t.rows = clampTerminal(rows, 1, 200)
	if t.pty && !t.closed {
		_ = setPTYSizeFD(t.masterFD, t.cols, t.rows)
	}
	t.mu.Unlock()
	t.broadcastState()
	return nil
}

func (t *terminalRecord) close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	stdin := t.stdin
	master := t.master
	cancel := t.cancel
	process := t.cmd.Process
	if master != nil {
		_ = master.Close()
	}
	t.mu.Unlock()
	_ = stdin.Close()
	if process != nil && runtime.GOOS != "windows" {
		_ = syscall.Kill(-process.Pid, syscall.SIGTERM)
	}
	cancel()
	select {
	case <-t.done:
	case <-time.After(2 * time.Second):
		if process != nil && runtime.GOOS != "windows" {
			_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
		}
		if process != nil {
			_ = process.Kill()
		}
	}
}

func copyTerminalSubs(source map[string]chan map[string]any) []chan map[string]any {
	out := make([]chan map[string]any, 0, len(source))
	for _, channel := range source {
		out = append(out, channel)
	}
	return out
}

func nativeSystemShell() string {
	if override := os.Getenv("DSH_SYSTEM_SHELL"); override != "" {
		return override
	}
	if runtime.GOOS == "android" {
		return "/system/bin/sh"
	}
	return "/bin/sh"
}

func nativeTerminalEnvironment(home string) []string {
	path := os.Getenv("PATH")
	if path == "" || runtime.GOOS == "android" {
		path = "/system/bin:/system/xbin"
	}
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = os.TempDir()
	}
	return []string{"HOME=" + home, "PATH=" + path, "TMPDIR=" + tmp, "TERM=xterm-256color", "LANG=C.UTF-8"}
}

func terminalRPCError(err error) *RPCError {
	message := err.Error()
	code := "terminal/unavailable"
	if regexp.MustCompile(`read-only`).MatchString(message) {
		code = "terminal/read-only"
	}
	return &RPCError{Code: code, Message: message}
}

func clampTerminal(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func trimTerminalTitle(title string) string {
	runes := bytes.Runes([]byte(title))
	for len(runes) > 0 && (runes[0] == ' ' || runes[0] == '\n' || runes[0] == '\t' || runes[0] == '\r') {
		runes = runes[1:]
	}
	for len(runes) > 0 && (runes[len(runes)-1] == ' ' || runes[len(runes)-1] == '\n' || runes[len(runes)-1] == '\t' || runes[len(runes)-1] == '\r') {
		runes = runes[:len(runes)-1]
	}
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return string(runes)
}

func newTerminalAttachmentID() string { return uuid.MustV4() }

func terminalFrameWithController(value map[string]any, attachmentID string) map[string]any {
	copyValue := make(map[string]any, len(value))
	for key, item := range value {
		copyValue[key] = item
	}
	if rawInfo, ok := value["info"].(map[string]any); ok {
		info := make(map[string]any, len(rawInfo)+1)
		for key, item := range rawInfo {
			info[key] = item
		}
		if value["writable"] == true {
			info["controllerId"] = attachmentID
		} else {
			info["controllerId"] = ""
		}
		copyValue["info"] = info
	}
	return copyValue
}
