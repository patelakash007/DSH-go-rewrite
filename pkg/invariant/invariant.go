//

//

//

package invariant

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

var enabled atomic.Bool

func init() {
	enabled.Store(true)
}

func SetEnabled(on bool) {
	enabled.Store(on)
}

func IsEnabled() bool {
	return enabled.Load()
}

type InvariantError struct {
	Pkg string

	Msg string
}

func (e *InvariantError) Error() string {
	return fmt.Sprintf("INVARIANT [%s]: %s", e.Pkg, e.Msg)
}

type Checker func() error

type Registry struct {
	mu       sync.Mutex
	checkers map[string][]Checker
	ledger   []string
}

func NewRegistry() *Registry {
	return &Registry{
		checkers: make(map[string][]Checker),
		ledger:   make([]string, 0),
	}
}

//

func (r *Registry) Register(pkgName string, checker Checker) error {
	if pkgName == "" {
		return fmt.Errorf("invariant: pkgName must not be empty")
	}
	if checker == nil {
		return fmt.Errorf("invariant: checker must not be nil for pkg %q", pkgName)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[pkgName] = append(r.checkers[pkgName], checker)
	return nil
}

func (r *Registry) Run() []error {
	if !IsEnabled() {
		return nil
	}
	r.mu.Lock()
	pkgs := make([]string, 0, len(r.checkers))
	for pkg := range r.checkers {
		pkgs = append(pkgs, pkg)
	}

	snapshot := make(map[string][]Checker, len(r.checkers))
	for pkg, list := range r.checkers {
		cp := make([]Checker, len(list))
		copy(cp, list)
		snapshot[pkg] = cp
	}
	r.mu.Unlock()

	sort.Strings(pkgs)
	var failures []error
	for _, pkg := range pkgs {
		for _, checker := range snapshot[pkg] {
			if err := runSafe(checker); err != nil {

				var invErr *InvariantError
				if as, ok := err.(*InvariantError); ok {
					invErr = as
				} else {
					invErr = &InvariantError{Pkg: pkg, Msg: err.Error()}
				}
				failures = append(failures, invErr)
				r.record(pkg, invErr.Error())
			}
		}
	}
	return failures
}

func runSafe(checker Checker) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("invariant checker panicked: %v", p)
		}
	}()
	return checker()
}

func (r *Registry) record(pkg, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ledger = append(r.ledger, detail)
}

func (r *Registry) Ledger() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.ledger))
	copy(out, r.ledger)
	return out
}

func (r *Registry) MustSatisfy() {
	if !IsEnabled() {
		return
	}
	failures := r.Run()
	if len(failures) > 0 {
		msgs := make([]string, 0, len(failures))
		for _, f := range failures {
			msgs = append(msgs, f.Error())
		}
		panic("invariant violated: " + strings.Join(msgs, "; "))
	}
}
