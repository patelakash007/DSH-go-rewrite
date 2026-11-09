//

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	sqlstore "github.com/JopenChen/dsh-go/pkg/persistence/sqlite"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sessionquery"
)

func seedSessions(t *testing.T, ctx context.Context, st *sqlstore.Store) {
	t.Helper()
	base := time.Now().Add(-24 * time.Hour)
	specs := []struct {
		id    string
		title string
		msg   string
		hour  int
	}{
		{"s1", "Database comparison report", "Analyze differences between MySQL and PostgreSQL", 2},
		{"s2", "Cache hit rate alignment plan", "How to achieve 99% cache hit rate", 6},
		{"s3", "Deployment launch process", "Deploy services using docker", 10},
	}
	for _, sp := range specs {
		sid := brand.NewSessionID(sp.id)

		msgEv := session.SessionEvent{
			Seq: 1, Time: base.Add(time.Duration(sp.hour) * time.Hour),
			Type: session.EventUserMessage, Data: session.UserMessageData{Content: sp.msg},
		}
		titleEv := session.SessionEvent{
			Seq: 2, Time: msgEv.Time.Add(time.Second),
			Type: session.EventSessionTitle, Data: session.SessionTitleData{Title: sp.title},
		}
		if err := st.Append(ctx, sid, msgEv); err != nil {
			t.Fatal(err)
		}
		if err := st.Append(ctx, sid, titleEv); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSessionQueryTripleFilter(t *testing.T) {
	ctx := context.Background()
	st, err := sqlstore.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seedSessions(t, ctx, st)
	q := sessionquery.New(st)

	from := time.Now().Add(-24 * time.Hour).Add(5 * time.Hour)
	to := time.Now().Add(-24 * time.Hour).Add(11 * time.Hour)

	list := mustList(t, q, sessionquery.ListRequest{TitlePrefix: "Data"})
	if len(list) != 1 || list[0].ID != "s1" {
		t.Fatalf("title prefix 'Data' should only match s1, got %+v", list)
	}

	list2 := mustList(t, q, sessionquery.ListRequest{CreatedFrom: &from, CreatedTo: &to})
	ids := idsOf(list2)
	if len(ids) != 2 || !has(ids, "s2") || !has(ids, "s3") {
		t.Fatalf("time range should match s2/s3, got %v", ids)
	}

	list3 := mustList(t, q, sessionquery.ListRequest{TitlePrefix: "", Limit: 1})
	if len(list3) != 1 || list3[0].ID != "s3" {
		t.Fatalf("Limit=1 and desc should match latest s3, got %+v", list3)
	}
}

func TestSessionQueryContentSearch(t *testing.T) {
	ctx := context.Background()
	st, err := sqlstore.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seedSessions(t, ctx, st)
	q := sessionquery.New(st)

	if !st.FTSEnabled() {
		t.Skip("current driver does not support FTS5")
	}

	res := mustSearch(t, q, sessionquery.SearchRequest{Keyword: "PostgreSQL"})
	if len(res) != 1 || res[0].ID != "s1" {
		t.Fatalf("keyword PostgreSQL should match s1, got %+v", res)
	}

	res2 := mustSearch(t, q, sessionquery.SearchRequest{Keyword: "Cache hit rate"})
	if len(res2) != 1 || res2[0].ID != "s2" {
		t.Fatalf("title containing 'Cache hit rate' should match s2, got %+v", res2)
	}
}

func TestSessionQueryEmptyKeyword(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlstore.Open(":memory:")
	defer st.Close()
	q := sessionquery.New(st)
	res, err := q.Search(ctx, sessionquery.SearchRequest{Keyword: "  "})
	if err != nil || len(res) != 0 {
		t.Fatalf("empty keyword should return empty, got %v err=%v", res, err)
	}
}

// ============================================================================

// ============================================================================

func mustList(t *testing.T, q *sessionquery.QueryService, req sessionquery.ListRequest) []sessionquery.SessionSummary {
	t.Helper()
	list, err := q.ListSummaries(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func mustSearch(t *testing.T, q *sessionquery.QueryService, req sessionquery.SearchRequest) []sessionquery.SessionSummary {
	t.Helper()
	res, err := q.Search(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func idsOf(list []sessionquery.SessionSummary) map[string]bool {
	m := map[string]bool{}
	for _, s := range list {
		m[s.ID] = true
	}
	return m
}

func has(m map[string]bool, id string) bool { return m[id] }
