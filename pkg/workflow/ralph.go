package workflow

import (
	"context"
	"encoding/json"
	"errors"
)

type RoundStatus string

const (
	StatusContinue RoundStatus = "continue"
	StatusComplete RoundStatus = "complete"
	StatusBlocked  RoundStatus = "blocked"
)

type RoundReport struct {
	Status    RoundStatus `json:"status"`
	Summary   string      `json:"summary"`
	Evidence  string      `json:"evidence"`
	NextSteps []string    `json:"nextSteps"`
	Blocker   string      `json:"blocker"`
}

type RunStatus string

const (
	RunCompleted     RunStatus = "complete"
	RunBlocked       RunStatus = "blocked"
	RunBudgetLimited RunStatus = "budget-limited"
)

type RalphResult struct {
	Status RunStatus
	Rounds int
	Final  RoundReport
}

type RoundRunner func(ctx context.Context, objective, priorHandoff string) (RoundReport, error)

const DefaultRalphMaxRounds = 256

func ValidateReport(r RoundReport) error {
	switch r.Status {
	case StatusComplete:
		if r.Evidence == "" || len(r.NextSteps) > 0 || r.Blocker != "" {
			return errors.New("complete report must have evidence, no nextSteps, and empty blocker")
		}
	case StatusContinue:
		if len(r.NextSteps) == 0 {
			return errors.New("continue report must have at least one nextStep")
		}
	case StatusBlocked:
		if r.Blocker == "" {
			return errors.New("blocked report must provide a blocker")
		}
	default:
		return errors.New("invalid round status")
	}
	return nil
}

func RunRalph(ctx context.Context, objective string, runner RoundRunner, maxRounds, maxHandoffChars int) (*RalphResult, error) {
	if maxRounds <= 0 {
		maxRounds = DefaultRalphMaxRounds
	}
	prior := ""
	var last RoundReport
	for round := 1; round <= maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		report, err := runner(ctx, objective, prior)
		if err != nil {
			return nil, err
		}
		if err := ValidateReport(report); err != nil {
			return nil, err
		}
		last = report
		switch report.Status {
		case StatusComplete:
			return &RalphResult{Status: RunCompleted, Rounds: round, Final: report}, nil
		case StatusBlocked:
			return &RalphResult{Status: RunBlocked, Rounds: round, Final: report}, nil
		}
		b, _ := json.Marshal(report)
		if maxHandoffChars > 0 && len(b) > maxHandoffChars {
			return nil, errors.New("handoff exceeds maxHandoffChars")
		}
		prior = string(b)
	}
	return &RalphResult{Status: RunBudgetLimited, Rounds: maxRounds, Final: last}, nil
}
