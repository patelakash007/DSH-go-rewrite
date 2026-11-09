//

//

package goal

import (
	"context"

	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

// ============================================================================

// ============================================================================

type Phase string

const (
	PhaseActive   Phase = "active"
	PhasePaused   Phase = "paused"
	PhaseBlocked  Phase = "blocked"
	PhaseComplete Phase = "complete"
)

func (p Phase) Valid() bool {
	switch p {
	case PhaseActive, PhasePaused, PhaseBlocked, PhaseComplete:
		return true
	default:
		return false
	}
}

type Goal struct {
	ID          string   `json:"id"`
	Phase       Phase    `json:"phase"`
	Description string   `json:"description"`
	MaxRounds   int      `json:"maxRounds"`
	Revision    uint64   `json:"revision"`
	Blockers    []string `json:"blockers,omitempty"`

	BlockReason *GoalBlockReason `json:"blockedReason,omitempty"`
}

func FromLog(sl *session.SessionLog) (Goal, bool) {
	fold := session.FoldGoalChange(sl.Events())
	if !fold.Present {
		return Goal{}, false
	}
	g := Goal{
		ID:          fold.GoalID,
		Phase:       phaseFrom(fold.Phase),
		Description: fold.Description,
		MaxRounds:   fold.MaxRounds,
		Revision:    fold.Revision,
	}

	if fold.BlockReason != nil {
		g.BlockReason = &GoalBlockReason{Code: fold.BlockReason.Code, Message: fold.BlockReason.Message}
	}
	return g, true
}

func toSessionBlockReason(r *GoalBlockReason) *session.GoalBlockReason {
	if r == nil {
		return nil
	}
	return &session.GoalBlockReason{Code: r.Code, Message: r.Message}
}

func phaseFrom(p session.GoalPhase) Phase {
	return Phase(p)
}

func writeGoal(sl *session.SessionLog, g Goal) error {
	_, err := sl.Append(session.GoalChangeData{
		GoalID:      g.ID,
		Phase:       session.GoalPhase(g.Phase),
		Description: g.Description,
		MaxRounds:   g.MaxRounds,
		Revision:    g.Revision,
		BlockReason: toSessionBlockReason(g.BlockReason),
	})
	return err
}

func NewGoal(id, desc string, maxRounds int) Goal {
	return Goal{ID: id, Phase: PhaseActive, Description: desc, MaxRounds: maxRounds, Revision: 1}
}

// ============================================================================

// ============================================================================

type RoundDriver struct {
	roundCh chan int
}

func NewRoundDriver() *RoundDriver {
	return &RoundDriver{roundCh: make(chan int, 1)}
}

func (d *RoundDriver) OnTurnStopping(sl *session.SessionLog, round int) bool {
	g, ok := FromLog(sl)

	if !ok || g.Phase != PhaseActive {
		return false
	}

	if round < g.MaxRounds {

		_, _ = sl.Append(session.GoalRoundData{Round: round + 1})
		select {
		case d.roundCh <- round + 1:
		default:
		}
		return true
	}
	return false
}

func (d *RoundDriver) WaitRound(ctx context.Context) (int, error) {
	select {
	case r := <-d.roundCh:
		return r, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// ============================================================================

// ============================================================================

type GoalToolset struct {
	sl *session.SessionLog
}

func NewGoalToolset(sl *session.SessionLog) *GoalToolset {
	return &GoalToolset{sl: sl}
}

func (g *GoalToolset) Tools() []*tools.Tool {
	return []*tools.Tool{
		{Name: "goal_list", Execute: g.goalList},
		{Name: "goal_set_phase", Execute: g.goalSetPhase},
		{Name: "goal_set_description", Execute: g.goalSetDescription},
		{Name: "goal_set_max_rounds", Execute: g.goalSetMaxRounds},
		{Name: "goal_add_blocker", Execute: g.goalAddBlocker},
		{Name: "goal_report_blocker", Execute: g.goalReportBlocker},
	}
}

func (g *GoalToolset) current() (Goal, bool) {
	return FromLog(g.sl)
}

func (g *GoalToolset) goalList(ctx context.Context, input map[string]any) (any, error) {
	if cur, ok := g.current(); ok {
		return map[string]any{"goal": cur}, nil
	}
	return map[string]any{"goal": nil}, nil
}

func (g *GoalToolset) goalSetPhase(ctx context.Context, input map[string]any) (any, error) {
	phase, _ := input["phase"].(string)
	cur, ok := g.current()
	if !ok {
		return nil, NewGoalError(ErrorNotFound, "no active goal", nil)
	}

	target := Phase(phase)
	if !target.Valid() {
		return nil, NewGoalError(ErrorInvalidTransition, "invalid phase: "+phase, nil)
	}

	if !CanTransition(cur.Phase, target) {
		return nil, NewGoalError(ErrorInvalidTransition,
			"cannot transition "+string(cur.Phase)+" -> "+phase, nil)
	}
	cur.Phase = target
	cur.Revision++
	if err := writeGoal(g.sl, cur); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "phase": phase}, nil
}

func (g *GoalToolset) goalSetDescription(ctx context.Context, input map[string]any) (any, error) {
	desc, _ := input["description"].(string)
	cur, ok := g.current()
	if !ok {
		return nil, NewGoalError(ErrorNotFound, "no active goal", nil)
	}
	cur.Description = desc
	cur.Revision++
	if err := writeGoal(g.sl, cur); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "description": desc}, nil
}

func (g *GoalToolset) goalSetMaxRounds(ctx context.Context, input map[string]any) (any, error) {
	rounds, _ := input["maxRounds"].(float64)
	cur, ok := g.current()
	if !ok {
		return nil, NewGoalError(ErrorNotFound, "no active goal", nil)
	}

	if int(rounds) <= 0 {
		return nil, NewGoalError(ErrorInvalidMaxRounds, "maxRounds must be positive", nil)
	}
	cur.MaxRounds = int(rounds)
	cur.Revision++
	if err := writeGoal(g.sl, cur); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "maxRounds": cur.MaxRounds}, nil
}

func (g *GoalToolset) goalAddBlocker(ctx context.Context, input map[string]any) (any, error) {
	blocker, _ := input["blocker"].(string)
	cur, ok := g.current()
	if !ok {
		return nil, NewGoalError(ErrorNotFound, "no active goal", nil)
	}

	if blocker == "" {
		return nil, NewGoalError(ErrorInvalidBlockReason, "blocker required", nil)
	}
	cur.Blockers = append(cur.Blockers, blocker)
	cur.Revision++
	if err := writeGoal(g.sl, cur); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "blockers": cur.Blockers}, nil
}

func (g *GoalToolset) goalReportBlocker(ctx context.Context, input map[string]any) (any, error) {
	blocker, _ := input["blocker"].(string)
	phase := PhaseBlocked
	if ph, ok := input["phase"].(string); ok && ph != "" {
		phase = Phase(ph)
	}
	cur, ok := g.current()
	if !ok {
		return nil, NewGoalError(ErrorNotFound, "no active goal", nil)
	}

	if blocker == "" {
		return nil, NewGoalError(ErrorInvalidBlockReason, "blocker reason required", nil)
	}
	cur.Phase = phase
	cur.Blockers = append(cur.Blockers, blocker)

	if phase == PhaseBlocked {
		cur.BlockReason = &GoalBlockReason{Code: "user-reported", Message: blocker}
	}
	cur.Revision++
	if err := writeGoal(g.sl, cur); err != nil {
		return nil, err
	}

	if conclude := tools.ConcludeFrom(ctx); conclude != nil {
		conclude("goal-blocker")
	}
	out := map[string]any{"ok": true, "phase": phase, "blockers": cur.Blockers}
	if cur.BlockReason != nil {
		out["blockReason"] = cur.BlockReason
	}
	return out, nil
}
