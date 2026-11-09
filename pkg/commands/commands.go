//

//

package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/registry"
	"github.com/JopenChen/dsh-go/pkg/session"
)

type CommandHandler func(ctx context.Context, args string, sl *session.SessionLog) (output string, err error)

type CommandDefinition struct {
	Name string

	Description string

	Handler CommandHandler
}

type Registry struct {
	cmds *registry.Freezable[string, *CommandDefinition]
}

func NewRegistry() *Registry {
	r := &Registry{cmds: registry.NewFreezable[string, *CommandDefinition]()}
	_ = r.cmds.Put("plan", &CommandDefinition{Name: "plan", Description: "Toggle plan mode (on/off)", Handler: handlePlan})
	_ = r.cmds.Put("goal", &CommandDefinition{Name: "goal", Description: "Set goal", Handler: handleGoal})
	return r
}

func (r *Registry) Register(def *CommandDefinition) error {
	if def == nil || def.Name == "" {
		return fmt.Errorf("commands: name required")
	}
	return r.cmds.Put(def.Name, def)
}

func (r *Registry) Freeze() { r.cmds.Freeze() }

func (r *Registry) IsFrozen() bool { return r.cmds.IsFrozen() }

func (r *Registry) List() []string {
	names := r.cmds.Keys()
	sort.Strings(names)
	return names
}

func (r *Registry) Get(name string) (*CommandDefinition, bool) {
	return r.cmds.Get(name)
}

func (r *Registry) Dispatch(ctx context.Context, raw string, sl *session.SessionLog) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '/' {
		return "", fmt.Errorf("commands: not a command: %q", raw)
	}
	line := trimmed[1:]
	parts := strings.SplitN(line, " ", 2)
	name := strings.TrimSpace(parts[0])
	args := ""
	if len(parts) == 2 {
		args = strings.TrimSpace(parts[1])
	}

	def, ok := r.Get(name)
	if !ok {
		return "", ErrUnknownCommand(name)
	}

	_, _ = sl.Append(session.CommandRunData{Command: "/" + name, Args: args})

	output, err := def.Handler(ctx, args, sl)
	if err != nil {
		return "", err
	}

	_, _ = sl.Append(session.CommandDoneData{Command: "/" + name})
	return output, nil
}

type ErrUnknownCommand string

func (e ErrUnknownCommand) Error() string {
	return fmt.Sprintf("commands: unknown command %q", string(e))
}

func handlePlan(ctx context.Context, args string, sl *session.SessionLog) (string, error) {
	mode := "off"
	if args == "on" {
		mode = "on"
	}
	if _, err := sl.Append(session.PlanModeData{Mode: mode}); err != nil {
		return "", err
	}
	return "plan mode = " + mode, nil
}

func handleGoal(ctx context.Context, args string, sl *session.SessionLog) (string, error) {
	fold := session.FoldGoalChange(sl.Events())
	revision := fold.Revision + 1
	desc := args
	if desc == "" {
		desc = "(empty goal)"
	}
	if _, err := sl.Append(session.GoalChangeData{
		GoalID:      brand.NewSessionID("g_" + itoa64(revision)).Raw(),
		Phase:       "active",
		Description: desc,
		MaxRounds:   5,
		Revision:    revision,
	}); err != nil {
		return "", err
	}
	return "goal set: " + desc, nil
}

func itoa64(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
