//

//

//

package subprocess

import (
	"bytes"
	"sync"
)

// ============================================================================

// ============================================================================

type CollectedOutput struct {
	Text string `json:"text"`

	Truncated bool `json:"truncated"`

	SpillPath string `json:"spillPath,omitempty"`
}

type SubprocessCollect struct {
	MaxBytes int

	SpillMaxBytes int
}

type SubprocessStdinMode string

const (
	StdinIgnore SubprocessStdinMode = "ignore"
	StdinPipe   SubprocessStdinMode = "pipe"

	StdinData = "data"
)

type SubprocessSpawnSpec struct {
	Argv []string

	Cwd string

	StdoutMaxBytes int
	StderrMaxBytes int

	SpillRoot string

	GraceMs int

	Env map[string]string

	StdinData string
}

type SubprocessOutcome struct {
	ExitCode int

	Signal string
}

type SubprocessHandle struct {
	Pid int

	collected *collectedOutputs

	done        chan SubprocessOutcome
	doneErr     chan error
	termOnce    sync.Once
	terminateFn func()
}

func (h *SubprocessHandle) Collected() *collectedOutputs { return h.collected }

func (h *SubprocessHandle) Done() <-chan SubprocessOutcome { return h.done }

func (h *SubprocessHandle) Terminate() {
	h.termOnce.Do(h.terminateFn)
}

// ============================================================================

// ============================================================================

type collectedOutputs struct {
	stdout *outputReader
	stderr *outputReader
}

func (c *collectedOutputs) Stdout() *outputReader { return c.stdout }

func (c *collectedOutputs) Stderr() *outputReader { return c.stderr }

type outputReader struct {
	mu        sync.Mutex
	trunc     bytes.Buffer
	fullSpill string
	truncated bool
	total     int
}

func (r *outputReader) readFrom(fromByte int) (text string, nextOffset int, lossy bool, spillPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.truncated {

		lossy = true
		return r.trunc.String(), r.total, lossy, r.fullSpill
	}
	cur := r.trunc.Bytes()
	if fromByte >= len(cur) {
		return "", fromByte, false, r.fullSpill
	}
	delta := cur[fromByte:]
	return string(delta), fromByte + len(delta), false, r.fullSpill
}

func (r *outputReader) ReadOutput(from int) CollectedOutput {
	text, _, lossy, spill := r.readFrom(from)
	return CollectedOutput{Text: text, Truncated: lossy, SpillPath: spill}
}

func (r *outputReader) append(data []byte, memCap, spillCap int, spillDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(data) == 0 {
		return
	}
	r.total += len(data)
	switch {
	case r.truncated:

		if spillDir != "" {
			r.fullSpill = writeSpillFile(spillDir, r.fullSpill, data)
		}
		r.appendTail(data, memCap)
	default:
		if r.trunc.Len()+len(data) <= memCap {
			r.trunc.Write(data)
			return
		}

		if spillDir != "" {
			r.fullSpill = writeSpillFile(spillDir, r.fullSpill, r.trunc.Bytes())
			r.trunc.Reset()
			r.truncated = true
			r.fullSpill = writeSpillFile(spillDir, r.fullSpill, data)
		} else {
			r.truncated = true
		}
		r.appendTail(data, memCap)
	}
}

func (r *outputReader) appendTail(data []byte, memCap int) {
	r.trunc.Write(data)
	if r.trunc.Len() <= memCap {
		return
	}
	over := r.trunc.Len() - memCap
	buf := r.trunc.Bytes()
	r.trunc.Reset()
	r.trunc.Write(buf[over:])
}
