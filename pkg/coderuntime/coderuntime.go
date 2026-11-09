//

//

package coderuntime

import "context"

type BindingFunction func(ctx context.Context, args any) (any, error)

type BindingNamespace struct {
	Global string

	Functions map[string]BindingFunction

	ErrorClass string
}

type FailureKind string

const (
	FailException FailureKind = "exception"

	FailTimeout FailureKind = "timeout"

	FailAbort FailureKind = "abort"

	FailWorkerExit FailureKind = "worker-exit"

	FailInvalidOutput FailureKind = "invalid-output"

	FailOutputLimit FailureKind = "output-limit"
)

type RunFailure struct {
	Kind FailureKind

	Message string
}

type RunRequest struct {
	Program string

	Bindings []BindingNamespace

	Ctx context.Context
}

type RunResult struct {
	Value any

	Logs []string

	Failure *RunFailure
}

func (r *RunResult) Failed() bool { return r.Failure != nil }

type Runtime interface {
	Language() string

	Isolation() string

	Run(req *RunRequest) (*RunResult, error)
}
