//

//

package sessionquery

import (
	"context"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	sqlstore "github.com/JopenChen/dsh-go/pkg/persistence/sqlite"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sessiontitle"
)

// ============================================================================

// ============================================================================

type SessionSummary struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	EventCount int       `json:"eventCount"`
}

type ListRequest struct {
	TitlePrefix string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int
}

type SearchRequest struct {
	Keyword string
	Limit   int
}

type QueryService struct {
	store *sqlstore.Store
}

func New(store *sqlstore.Store) *QueryService {
	return &QueryService{store: store}
}

// ============================================================================

// ============================================================================

func deriveSummary(id string, events []session.SessionEvent) SessionSummary {
	sum := SessionSummary{ID: id, EventCount: len(events)}
	if len(events) == 0 {
		return sum
	}
	sum.CreatedAt = events[0].Time
	sum.UpdatedAt = events[len(events)-1].Time

	title := session.FoldSessionTitle(events).Title
	if title == "" {
		title = firstUserMessage(events)
	}
	sum.Title = title
	return sum
}

func firstUserMessage(events []session.SessionEvent) string {
	for _, ev := range events {
		if ev.Type == session.EventUserMessage {
			if d, ok := ev.Data.(session.UserMessageData); ok {
				return sessiontitle.Fallback(d.Content)
			}
		}
	}
	return ""
}

// ============================================================================

// ============================================================================

func (q *QueryService) ListSummaries(ctx context.Context, req ListRequest) ([]SessionSummary, error) {
	ids, err := q.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	var out []SessionSummary
	for _, id := range ids {
		events, err := q.store.Load(ctx, brand.NewSessionID(id))
		if err != nil {
			return nil, err
		}
		sum := deriveSummary(id, events)

		if req.TitlePrefix != "" && !strings.HasPrefix(sum.Title, req.TitlePrefix) {
			continue
		}

		if req.CreatedFrom != nil && sum.CreatedAt.Before(*req.CreatedFrom) {
			continue
		}
		if req.CreatedTo != nil && sum.CreatedAt.After(*req.CreatedTo) {
			continue
		}
		out = append(out, sum)
	}

	sortByUpdatedDesc(out)
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

// ============================================================================

// ============================================================================

func (q *QueryService) Search(ctx context.Context, req SearchRequest) ([]SessionSummary, error) {
	keyword := strings.TrimSpace(req.Keyword)
	if keyword == "" {
		return nil, nil
	}

	matches, err := q.store.Search(ctx, keyword)
	if err != nil {
		return nil, err
	}
	hitSessions := map[string]bool{}
	for _, m := range matches {
		hitSessions[m.SessionID] = true
	}

	ids, err := q.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	var out []SessionSummary
	for _, id := range ids {
		events, err := q.store.Load(ctx, brand.NewSessionID(id))
		if err != nil {
			return nil, err
		}
		sum := deriveSummary(id, events)
		if hitSessions[id] || strings.Contains(sum.Title, keyword) {
			out = append(out, sum)
		}
	}
	sortByUpdatedDesc(out)
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

// ============================================================================

// ============================================================================

func sortByUpdatedDesc(list []SessionSummary) {

	for i := 1; i < len(list); i++ {
		j := i
		for j > 0 && list[j].UpdatedAt.After(list[j-1].UpdatedAt) {
			list[j], list[j-1] = list[j-1], list[j]
			j--
		}
	}
}
