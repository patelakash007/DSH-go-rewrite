//

//

package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ============================================================================

// ============================================================================

type WaitReason string

const (
	WaitExited    WaitReason = "exited"
	WaitSignalled WaitReason = "signalled"
	WaitClosed    WaitReason = "closed"
	WaitError     WaitReason = "error"
)

type TerminalConfig struct {
	Command     string
	Args        []string
	MaxLines    int
	ReadLineMax int
}

func (c TerminalConfig) withDefaults() TerminalConfig {
	if c.MaxLines <= 0 {
		c.MaxLines = 1000
	}
	if c.ReadLineMax <= 0 {
		c.ReadLineMax = 64 * 1024
	}
	return c
}

// ============================================================================
// TerminalBackend
// ============================================================================

type TerminalBackend struct {
	mu     sync.Mutex
	active *Session
	cfg    TerminalConfig
}

func NewBackend(cfg TerminalConfig) *TerminalBackend {
	return &TerminalBackend{cfg: cfg.withDefaults()}
}

func (b *TerminalBackend) Spawn(ctx context.Context) (*Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil && b.active.running() {
		return nil, fmt.Errorf("terminal: single-agent exclusive activity: a session is already active")
	}
	s, err := spawnSession(ctx, b.cfg)
	if err != nil {
		return nil, err
	}
	b.active = s
	return s, nil
}

func (b *TerminalBackend) Active() *Session {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil && b.active.running() {
		return b.active
	}
	return nil
}

// ============================================================================
// Session
// ============================================================================

type Session struct {
	cfg TerminalConfig
	cmd *exec.Cmd
	w   io.WriteCloser // stdin

	mu       sync.Mutex
	lines    []string
	done     chan struct{}
	readDone chan struct{}
	reason   WaitReason
	started  time.Time
	exited   bool
}

func spawnSession(ctx context.Context, cfg TerminalConfig) (*Session, error) {
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &Session{
		cfg:      cfg,
		cmd:      cmd,
		w:        stdin,
		lines:    []string{},
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
		reason:   WaitClosed,
		started:  time.Now(),
	}

	go s.readLoop(stdout)
	go s.waitLoop()
	return s, nil
}

func (s *Session) readLoop(r io.Reader) {
	defer close(s.readDone)
	sc := bufio.NewReaderSize(r, s.cfg.ReadLineMax)
	for {
		line, err := readLineLimited(sc, s.cfg.ReadLineMax)
		if len(line) > 0 {
			s.pushLine(line)
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) waitLoop() {
	err := s.cmd.Wait()
	<-s.readDone
	s.mu.Lock()
	if s.reason == WaitClosed {
		s.reason = WaitExited
	}
	if err != nil && s.reason == WaitClosed {

		s.reason = WaitExited
	}
	s.exited = true
	s.mu.Unlock()
	close(s.done)
}

func readLineLimited(sc *bufio.Reader, max int) (string, error) {
	sb := strings.Builder{}
	for {
		chunk, err := sc.ReadString('\n')
		sb.WriteString(chunk)
		if err != nil {
			return sb.String(), err
		}
		if len(chunk) > 0 && strings.HasSuffix(chunk, "\n") {
			return strings.TrimSuffix(sb.String(), "\n"), nil
		}
	}
}

func (s *Session) pushLine(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, line)
	if len(s.lines) > s.cfg.MaxLines {

		excess := len(s.lines) - s.cfg.MaxLines
		s.lines = append([]string(nil), s.lines[excess:]...)
	}
}

func (s *Session) Send(data string) error {
	if _, err := s.w.Write([]byte(data)); err != nil {
		return err
	}
	return nil
}

func (s *Session) SendLine(line string) error {
	return s.Send(line + "\n")
}

func (s *Session) Read() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.lines))
	copy(out, s.lines)
	return out
}

func (s *Session) ReadString() string {
	lines := s.Read()
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Session) Contains(sub string) bool {
	return strings.Contains(s.ReadString(), sub)
}

func (s *Session) Wait() WaitReason {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

func (s *Session) Signal() error {
	return s.cmd.Process.Kill()
}

func (s *Session) Close() error {
	_ = s.w.Close()
	if !s.exitedState() {
		_ = s.cmd.Process.Kill()
	}
	s.mu.Lock()
	s.reason = WaitClosed
	s.mu.Unlock()
	return nil
}

func (s *Session) running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.exited
}

func (s *Session) exitedState() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exited
}

// ============================================================================

// ============================================================================

func ShellCommand(script string) (name string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/S", "/C", script}
	}
	return "/bin/sh", []string{"-c", script}
}
