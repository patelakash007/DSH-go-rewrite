//

//

package userq

import (
	"context"
	"fmt"
)

type QuestionOptions struct {
	Prompt string

	Choices []string

	DefaultIndex int

	MultiSelect bool

	Intent string
}

type QuestionResult struct {
	SelectedIndex int

	Custom string

	SelectedIndices []int
}

type Provider interface {
	Ask(ctx context.Context, opts QuestionOptions) (*QuestionResult, error)
}

type Service struct {
	provider Provider
}

func New(p Provider) *Service {
	return &Service{provider: p}
}

func (s *Service) SetProvider(p Provider) {
	s.provider = p
}

func (s *Service) Provider() Provider {
	return s.provider
}

func (s *Service) Ask(ctx context.Context, opts QuestionOptions) (int, string, error) {
	if s.provider == nil {
		return 0, "", fmt.Errorf("userq: no provider installed")
	}
	res, err := s.provider.Ask(ctx, opts)
	if err != nil {
		return 0, "", err
	}
	return res.SelectedIndex, res.Custom, nil
}

func (s *Service) AskDetailed(ctx context.Context, opts QuestionOptions) (*QuestionResult, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("userq: no provider installed")
	}
	return s.provider.Ask(ctx, opts)
}

// ============================================================================

// ============================================================================

type StubProvider struct {
	DefaultIndex int
}

func NewStub() *StubProvider {
	return &StubProvider{DefaultIndex: 0}
}

func (s *StubProvider) Ask(ctx context.Context, opts QuestionOptions) (*QuestionResult, error) {
	idx := opts.DefaultIndex
	if idx < 0 || idx >= len(opts.Choices) {
		idx = s.DefaultIndex
		if idx < 0 || idx >= len(opts.Choices) {
			idx = 0
		}
	}
	selected := []int{idx}
	if opts.MultiSelect {

		selected = make([]int, 0, len(opts.Choices))
		for i := range opts.Choices {
			selected = append(selected, i)
		}
		idx = 0
	}
	return &QuestionResult{SelectedIndex: idx, SelectedIndices: selected}, nil
}
