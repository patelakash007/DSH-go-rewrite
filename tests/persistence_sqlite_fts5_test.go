//

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	sqlstore "github.com/JopenChen/dsh-go/pkg/persistence/sqlite"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func buildEvents(n int) []session.SessionEvent {
	evs := make([]session.SessionEvent, 0, n)
	for i := 1; i <= n; i++ {
		msg := "User message number " + time.Now().Format("150405") + " padding content to fill length"
		if i%500 == 0 {
			msg = "Target search keyword: database evaluation report"
		}
		evs = append(evs, session.SessionEvent{
			Seq:  uint64(i),
			Time: time.Now().Add(time.Duration(i) * time.Microsecond),
			Type: session.EventUserMessage,
			Data: session.UserMessageData{Content: msg},
		})
	}
	return evs
}

func TestSQLiteWriteReadRoundtrip(t *testing.T) {
	ctx := context.Background()
	st, err := sqlstore.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sid := brand.NewSessionID("s-1k")
	const n = 10000
	if err := st.AppendBatch(ctx, sid, buildEvents(n)); err != nil {
		t.Fatal(err)
	}
	if c, _ := st.Count(ctx, sid); c != n {
		t.Fatalf("should have %d events, got %d", n, c)
	}
	loaded, err := st.Load(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != n {
		t.Fatalf("read back %d != %d", len(loaded), n)
	}

	for i, ev := range loaded {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("seq should be %d, got %d", i+1, ev.Seq)
		}
	}
}

func TestSQLiteFTSSearch(t *testing.T) {
	ctx := context.Background()
	st, err := sqlstore.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sid := brand.NewSessionID("s-fts")

	for _, ev := range buildEvents(2000) {
		if err := st.Append(ctx, sid, ev); err != nil {
			t.Fatal(err)
		}
	}

	_ = st.Append(ctx, sid, session.SessionEvent{
		Seq: 3001, Time: time.Now(), Type: session.EventSessionTitle,
		Data: session.SessionTitleData{Title: "Competitive analysis and cache hit rate alignment plan"},
	})
	_ = st.Append(ctx, sid, session.SessionEvent{
		Seq: 3002, Time: time.Now(), Type: session.EventUserMessage,
		Data: session.UserMessageData{Content: "Please analyze the database selection comparison report"},
	})

	if !st.FTSEnabled() {
		t.Skip("current SQLite driver does not support FTS5")
	}
	matches, err := st.Search(ctx, "cache hit rate")
	if err != nil {
		t.Fatal(err)
	}
	foundTitle := false
	for _, m := range matches {
		if m.Seq == 3001 {
			foundTitle = true
		}
	}
	if !foundTitle {
		t.Fatalf("FTS5 should match session/title event (seq=3001), got matches=%+v", matches)
	}

	matches2, err := st.Search(ctx, "database selection")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches2) == 0 {
		t.Fatal("FTS5 should match keywords in message content")
	}
}

func TestSQLiteSearchPerf(t *testing.T) {
	ctx := context.Background()
	st, err := sqlstore.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sid := brand.NewSessionID("s-perf")
	if err := st.AppendBatch(ctx, sid, buildEvents(10000)); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = st.Search(ctx, "database")
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("single query %v > 100ms, does not meet acceptance", elapsed)
	}
}

func TestSQLiteSearchEmpty(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlstore.Open(":memory:")
	defer st.Close()
	ms, err := st.Search(ctx, "   ")
	if err != nil || len(ms) != 0 {
		t.Fatalf("empty keyword should return empty, got %v err=%v", ms, err)
	}
}
