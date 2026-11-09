//

//

//

package jobs

import (
	"fmt"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

type JobKind string

const (
	KindBash     JobKind = "bash"
	KindSubagent JobKind = "subagent"
)

type JobStatus string

const (
	StatusRunning   JobStatus = "running"
	StatusStopping  JobStatus = "stopping"
	StatusCompleted JobStatus = "completed"
	StatusKilled    JobStatus = "killed"
	StatusFailed    JobStatus = "failed"
)

type JobOutcome struct {
	Status JobStatus `json:"status"`

	Detail string `json:"detail,omitempty"`

	Output string `json:"output,omitempty"`
}

type JobSnapshot struct {
	ID brand.JobID `json:"id"`

	Kind JobKind `json:"kind"`

	Label string `json:"label"`

	OwnerSession brand.SessionID `json:"ownerSession,omitempty"`

	Status JobStatus `json:"status"`

	Detail string `json:"detail,omitempty"`

	Output string `json:"output,omitempty"`
}

type JobHooks struct {
	Cancel func(reason string)

	Done chan JobOutcome

	ReadOutput func() string
}

type JobStart struct {
	Kind JobKind

	Label string

	OwnerSession brand.SessionID

	Run func() *JobHooks
}

// ============================================================================

// ============================================================================

type Job struct {
	ID brand.JobID

	Kind JobKind

	Label string

	OwnerSession brand.SessionID

	mu     sync.Mutex
	status JobStatus
	detail string
	cancel func(reason string)
	done   chan JobOutcome
	read   func() string

	finished chan struct{}
}

func (j *Job) Snapshot() JobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	output := ""
	if j.read != nil {
		output = j.read()
	}
	return JobSnapshot{
		ID:           j.ID,
		Kind:         j.Kind,
		Label:        j.Label,
		OwnerSession: j.OwnerSession,
		Status:       j.status,
		Detail:       j.detail,
		Output:       output,
	}
}

func (j *Job) ReadOutput() string {
	if j.read == nil {
		return ""
	}
	return j.read()
}

func (j *Job) Cancel(reason string) {
	j.mu.Lock()
	done := j.cancel
	j.mu.Unlock()
	if done != nil {
		done(reason)
	}
}

func (j *Job) Done() <-chan JobOutcome { return j.done }

func (j *Job) resolveOutcome(o JobOutcome) {
	j.mu.Lock()
	j.status = o.Status
	j.detail = o.Detail
	if o.Output != "" {

		j.read = func() string { return o.Output }
	}
	j.mu.Unlock()
	select {
	case j.done <- o:
	default:
	}
}

type Runtime struct {
	mu     sync.Mutex
	seq    int
	jobs   map[int]*Job
	byKind map[string]int // kmd-N
}

func NewRuntime() *Runtime {
	return &Runtime{jobs: map[int]*Job{}, byKind: map[string]int{}}
}

func (r *Runtime) Start(start JobStart) (*Job, error) {
	if start.Run == nil {
		return nil, fmt.Errorf("jobs: Run hook required")
	}
	hooks := start.Run()
	if hooks == nil {
		return nil, fmt.Errorf("jobs: producer refused to start")
	}
	r.mu.Lock()
	r.seq++
	r.byKind[string(start.Kind)]++
	n := r.byKind[string(start.Kind)]
	id := brand.NewJobID(fmt.Sprintf("%s-%d", start.Kind, n))
	job := &Job{
		ID:           id,
		Kind:         start.Kind,
		Label:        start.Label,
		OwnerSession: start.OwnerSession,
		status:       StatusRunning,
		cancel:       hooks.Cancel,
		done:         hooks.Done,
		read:         hooks.ReadOutput,
		finished:     make(chan struct{}),
	}
	r.jobs[r.seq] = job
	r.mu.Unlock()

	go func(j *Job, h *JobHooks) {
		select {
		case o := <-h.Done:
			j.resolveOutcome(o)
		}
		r.unregister(j.ID)
		close(j.finished)
	}(job, hooks)
	return job, nil
}

func (r *Runtime) unregister(id brand.JobID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, j := range r.jobs {
		if j.ID == id {
			delete(r.jobs, k)
		}
	}
}

func (r *Runtime) JobByID(id brand.JobID) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (r *Runtime) ListByOwner(owner brand.SessionID) []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Job
	for _, j := range r.jobs {
		if j.OwnerSession == owner {
			out = append(out, j)
		}
	}
	return out
}

func (r *Runtime) All() []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, j)
	}
	return out
}

func (r *Runtime) DisposeOwner(owner brand.SessionID, reason string) {
	for _, j := range r.ListByOwner(owner) {
		j.Cancel(reason)
		<-j.finished
	}
}

func (j *Job) Finished() <-chan struct{} { return j.finished }
