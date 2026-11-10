package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const (
	maxJobOutputBytes = 1 << 20
	maxJobChunks      = 4000
)

type jobChunk struct {
	Sequence  int64  `json:"sequence"`
	Text      string `json:"text"`
	Channel   string `json:"channel,omitempty"`
	GapBefore bool   `json:"gapBefore,omitempty"`
}

type jobRecord struct {
	ID         string
	SessionID  string
	Kind       string
	Label      string
	Status     string
	Detail     string
	StartedAt  time.Time
	FinishedAt *time.Time

	mu       sync.Mutex
	chunks   []jobChunk
	bytes    int
	sequence int64
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	subs     map[string]chan map[string]any
}

type jobManager struct {
	mu   sync.Mutex
	jobs map[string]*jobRecord
	subs map[string]chan map[string]any
}

func newJobManager() *jobManager {
	return &jobManager{jobs: map[string]*jobRecord{}, subs: map[string]chan map[string]any{}}
}

func (m *jobManager) list(sessionID string) []*jobRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*jobRecord, 0)
	for _, job := range m.jobs {
		if job.SessionID == sessionID {
			result = append(result, job)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.Before(result[j].StartedAt) })
	return result
}

func (m *jobManager) get(sessionID, jobID string) *jobRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[jobID]
	if job == nil || job.SessionID != sessionID {
		return nil
	}
	return job
}

func (m *jobManager) add(job *jobRecord) {
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()
	m.broadcastList(job.SessionID)
}

func (m *jobManager) remove(job *jobRecord) {
	m.mu.Lock()
	delete(m.jobs, job.ID)
	m.mu.Unlock()
	m.broadcastList(job.SessionID)
}

func (m *jobManager) broadcastList(sessionID string) {
	jobs := m.list(sessionID)
	rows := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, job.info())
	}
	frame := map[string]any{"jobs": rows}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, channel := range m.subs {
		if !strings.HasPrefix(key, sessionID+"\x00") {
			continue
		}
		select {
		case channel <- frame:
		default:
		}
	}
}

func (m *jobManager) subscribe(sessionID string) (string, chan map[string]any) {
	key := sessionID + "\x00" + uuid.MustV4()
	channel := make(chan map[string]any, 32)
	m.mu.Lock()
	m.subs[key] = channel
	m.mu.Unlock()
	return key, channel
}

func (m *jobManager) unsubscribe(key string) {
	m.mu.Lock()
	channel := m.subs[key]
	delete(m.subs, key)
	if channel != nil {
		close(channel)
	}
	m.mu.Unlock()
}

func (m *jobManager) closeSession(sessionID string) {
	for _, job := range m.list(sessionID) {
		job.kill()
	}
}

func (j *jobRecord) info() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := map[string]any{
		"id": j.ID, "kind": j.Kind, "label": j.Label, "status": j.Status,
		"startedAt": j.StartedAt.UnixMilli(), "output": map[string]any{"total": j.bytes},
	}
	if j.Detail != "" {
		result["detail"] = j.Detail
	}
	if j.FinishedAt != nil {
		result["finishedAt"] = j.FinishedAt.UnixMilli()
	}
	return result
}

func (j *jobRecord) appendOutput(text, channel string) {
	if text == "" {
		return
	}
	j.mu.Lock()
	j.sequence++
	chunk := jobChunk{Sequence: j.sequence, Text: text, Channel: channel}
	j.chunks = append(j.chunks, chunk)
	j.bytes += len(text)
	for len(j.chunks) > maxJobChunks || j.bytes > maxJobOutputBytes {
		if len(j.chunks) == 0 {
			break
		}
		j.bytes -= len(j.chunks[0].Text)
		j.chunks = j.chunks[1:]
	}
	subs := make([]chan map[string]any, 0, len(j.subs))
	for _, channel := range j.subs {
		subs = append(subs, channel)
	}
	j.mu.Unlock()
	frame := map[string]any{
		"type": "output", "lossy": false,
		"chunks": []map[string]any{{"text": text, "channel": channel}},
	}
	for _, subscriber := range subs {
		select {
		case subscriber <- frame:
		default:
		}
	}
}

func (j *jobRecord) finish(status, detail string) {
	j.mu.Lock()
	if j.Status != "running" && j.Status != "stopping" {
		j.mu.Unlock()
		return
	}
	j.Status = status
	j.Detail = detail
	now := time.Now()
	j.FinishedAt = &now
	subs := make([]chan map[string]any, 0, len(j.subs))
	for _, channel := range j.subs {
		subs = append(subs, channel)
	}
	j.mu.Unlock()
	frame := map[string]any{"type": "status", "job": j.info()}
	for _, subscriber := range subs {
		select {
		case subscriber <- frame:
		default:
		}
	}
	close(j.done)
}

func (j *jobRecord) kill() {
	j.mu.Lock()
	if j.Status != "running" && j.Status != "stopping" {
		j.mu.Unlock()
		return
	}
	j.Status = "stopping"
	cmd := j.cmd
	cancel := j.cancel
	j.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		if runtime.GOOS != "windows" {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		_ = cmd.Process.Kill()
	}
}

func (j *jobRecord) follow(from int64) []map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	frames := make([]map[string]any, 0, len(j.chunks)+1)
	frames = append(frames, map[string]any{"type": "opened", "job": j.infoLocked()})
	for _, chunk := range j.chunks {
		if chunk.Sequence < from {
			continue
		}
		frames = append(frames, map[string]any{
			"type": "output", "lossy": false,
			"chunks": []map[string]any{{"text": chunk.Text, "channel": chunk.Channel}},
		})
	}
	if j.Status != "running" && j.Status != "stopping" {
		frames = append(frames, map[string]any{"type": "status", "job": j.infoLocked()})
	}
	return frames
}

func (j *jobRecord) infoLocked() map[string]any {
	result := map[string]any{
		"id": j.ID, "kind": j.Kind, "label": j.Label, "status": j.Status,
		"startedAt": j.StartedAt.UnixMilli(), "output": map[string]any{"total": j.bytes},
	}
	if j.Detail != "" {
		result["detail"] = j.Detail
	}
	if j.FinishedAt != nil {
		result["finishedAt"] = j.FinishedAt.UnixMilli()
	}
	return result
}

func (j *jobRecord) subscribe() (string, chan map[string]any) {
	key := uuid.MustV4()
	channel := make(chan map[string]any, 64)
	j.mu.Lock()
	j.subs[key] = channel
	j.mu.Unlock()
	return key, channel
}

func (j *jobRecord) unsubscribe(key string) {
	j.mu.Lock()
	channel := j.subs[key]
	delete(j.subs, key)
	if channel != nil {
		close(channel)
	}
	j.mu.Unlock()
}

func (s *Server) startShellJob(sessionID, command, cwd string, timeout time.Duration) (*jobRecord, error) {
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("background shell requires a command")
	}
	runCtx, cancel := context.WithCancel(context.Background())
	shell := nativeSystemShell()
	cmd := exec.CommandContext(runCtx, shell, "-c", command)
	cmd.Dir = cwd
	cmd.Env = nativeTerminalEnvironment(cwd)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	job := &jobRecord{
		ID: "bash-" + uuid.MustV4()[:8], SessionID: sessionID, Kind: "bash",
		Label: command, Status: "running", StartedAt: time.Now(),
		cmd: cmd, cancel: cancel, done: make(chan struct{}), subs: map[string]chan map[string]any{},
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	s.jobs.add(job)
	go func() {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			buffer := make([]byte, 32<<10)
			for {
				read, err := stdout.Read(buffer)
				if read > 0 {
					job.appendOutput(string(buffer[:read]), "stdout")
				}
				if err != nil {
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			buffer := make([]byte, 32<<10)
			for {
				read, err := stderr.Read(buffer)
				if read > 0 {
					job.appendOutput(string(buffer[:read]), "stderr")
				}
				if err != nil {
					return
				}
			}
		}()
		wg.Wait()
		waitErr := cmd.Wait()
		status := "completed"
		detail := ""
		if runCtx.Err() != nil {
			status = "killed"
			detail = "cancelled"
		} else if waitErr != nil {
			var exit *exec.ExitError
			if errors.As(waitErr, &exit) {
				status = "failed"
				detail = fmt.Sprintf("exit %d", exit.ExitCode())
			} else {
				status = "failed"
				detail = waitErr.Error()
			}
		}
		job.finish(status, detail)
		s.jobs.remove(job)
	}()
	if timeout > 0 {
		time.AfterFunc(timeout, func() {
			job.mu.Lock()
			running := job.Status == "running"
			job.mu.Unlock()
			if running {
				job.appendOutput(fmt.Sprintf("\n[job timed out after %s]\n", timeout), "log")
				job.kill()
			}
		})
	}
	return job, nil
}

func (s *Server) dispatchJobRPC(endpoint string, args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	sessionID, _ := request["sessionId"].(string)
	if s.sessionByID(sessionID) == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	jobID, _ := request["jobId"].(string)
	job := s.jobs.get(sessionID, jobID)
	if job == nil {
		return nil, &RPCError{Code: "job/not-found", Message: "job not found"}
	}
	if endpoint != "job/kill" {
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown job method"}
	}
	job.mu.Lock()
	live := job.Status == "running" || job.Status == "stopping"
	job.mu.Unlock()
	if !live {
		return map[string]any{"outcome": "already-finished"}, nil
	}
	job.kill()
	return map[string]any{"outcome": "requested"}, nil
}

func (s *Server) openJobList(sessionID string, send func(map[string]any) error) {
	key, updates := s.jobs.subscribe(sessionID)
	defer s.jobs.unsubscribe(key)
	jobs := s.jobs.list(sessionID)
	rows := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, job.info())
	}
	if err := send(map[string]any{"jobs": rows}); err != nil {
		return
	}
	for update := range updates {
		if err := send(update); err != nil {
			return
		}
	}
}

func (s *Server) openJobFollow(sessionID, jobID string, from int64, send func(map[string]any) error) {
	job := s.jobs.get(sessionID, jobID)
	if job == nil {
		_ = send(map[string]any{"type": "error", "error": map[string]any{"code": "job/not-found", "message": "job not found"}})
		return
	}
	for _, frame := range job.follow(from) {
		if err := send(frame); err != nil {
			return
		}
	}
	job.mu.Lock()
	live := job.Status == "running" || job.Status == "stopping"
	job.mu.Unlock()
	if !live {
		return
	}
	key, updates := job.subscribe()
	defer job.unsubscribe(key)
	for update := range updates {
		if err := send(update); err != nil {
			return
		}
	}
}

func (s *Server) closeAllJobs() {
	s.jobs.mu.Lock()
	jobs := make([]*jobRecord, 0, len(s.jobs.jobs))
	for _, job := range s.jobs.jobs {
		jobs = append(jobs, job)
	}
	s.jobs.mu.Unlock()
	for _, job := range jobs {
		job.kill()
	}
}

var _ = bytes.MinRead
