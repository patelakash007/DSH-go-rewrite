package jobs

import (
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/subprocess"
)

type SubprocessSpec struct {
	Kind JobKind

	Label string

	OwnerSession brand.SessionID

	Argv []string

	Cwd string

	StdoutMaxBytes int

	SpillRoot string

	GraceMs int
}

func (r *Runtime) StartSubprocess(spec SubprocessSpec) (*Job, error) {
	kind := spec.Kind
	if kind == "" {
		kind = KindBash
	}
	rt := subprocess.NewLocal()
	handle := rt.Spawn(subprocess.SubprocessSpawnSpec{
		Argv:           spec.Argv,
		Cwd:            spec.Cwd,
		StdoutMaxBytes: spec.StdoutMaxBytes,
		SpillRoot:      spec.SpillRoot,
		GraceMs:        spec.GraceMs,
	})
	return r.Start(JobStart{
		Kind:         kind,
		Label:        spec.Label,
		OwnerSession: spec.OwnerSession,
		Run: func() *JobHooks {
			return &JobHooks{

				Cancel: func(reason string) { handle.Terminate() },
				Done:   translateDone(handle),
				ReadOutput: func() string {
					return handle.Collected().Stdout().ReadOutput(0).Text
				},
			}
		},
	})
}

func translateDone(handle *subprocess.SubprocessHandle) chan JobOutcome {
	ch := make(chan JobOutcome, 1)
	go func() {
		res := <-handle.Done()
		out := JobOutcome{}
		switch {
		case res.Signal != "":
			out.Status = StatusKilled
			out.Detail = "signal: " + res.Signal
		case res.ExitCode == 0:
			out.Status = StatusCompleted
		default:
			out.Status = StatusFailed
			out.Detail = "exit code: " + itoa(res.ExitCode)
		}
		ch <- out
	}()
	return ch
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
