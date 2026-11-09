//

package tests

import (
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func newTestLogH04(enableIncremental bool) *session.SessionLog {
	sl := session.NewSessionLog(brand.NewSessionID("h04-sess"))
	if enableIncremental {
		sl.EnableIncrementalProjection()
	}
	return sl
}

func fixedTimeH04() time.Time { return time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) }

// ============================================================================

// ============================================================================

func TestH04IncrementalEquivalentAfterEachAppend(t *testing.T) {
	sl := newTestLogH04(true)
	if !sl.IncrementalEnabled() {
		t.Fatal("EnableIncrementalProjection should be true")
	}

	payloads := []session.EventData{
		session.UserMessageData{Content: "u1"},
		session.AssistantMessageData{Content: "a1"},
		session.RequestHeaderData{ConfigEpoch: 1, SystemHash: "h1", ToolCount: 3},
		session.PresetChangeData{Preset: "danger"},
		session.GoalChangeData{GoalID: "g1", Phase: session.GoalPhase("in-progress"), Description: "d1", MaxRounds: 5, Revision: 1},
		session.TodoWriteData{Items: []string{"t1", "t2"}},
		session.PlanModeData{Mode: "on"},
		session.SessionTitleData{Title: "Session Title"},
		session.UserMessageData{Content: "u2"},
		session.AssistantMessageData{Content: "a2", ToolCallIDs: []string{"call-1"}},
		session.PresetChangeData{Preset: "safe"},
		session.GoalChangeData{GoalID: "g1", Phase: session.GoalPhase("in-progress"), Description: "d2", MaxRounds: 5, Revision: 2},
		session.TodoWriteData{Items: []string{"t1-new"}},
		session.PlanModeData{Mode: "off"},
		session.RequestHeaderData{ConfigEpoch: 2, SystemHash: "h2", ToolCount: 7},
		session.SessionTitleData{Title: "Title V2"},
	}

	for i, p := range payloads {
		if _, err := sl.Append(p); err != nil {
			t.Fatalf("item %d Append failed: %v", i, err)
		}
		got := sl.Projection()
		want := session.FoldAll(sl.Events())
		if !session.SessionProjectionEqual(got, want) {
			t.Fatalf("item %d incremental != full after Append\n got=%+v\nwant=%+v", i, got, want)
		}
	}

	stats := sl.IncrementalStats()
	if stats.IncrementalAppends != uint64(len(payloads)) {
		t.Fatalf("IncrementalAppends = %d, want %d", stats.IncrementalAppends, len(payloads))
	}

	if stats.DirtyRebuilds != 0 {
		t.Fatalf("DirtyRebuilds = %d, want 0", stats.DirtyRebuilds)
	}
}

// ============================================================================

// ============================================================================

func TestH04WarmStartEquivalence(t *testing.T) {
	sl := newTestLogH04(false)

	base := []session.EventData{
		session.UserMessageData{Content: "old-u"},
		session.AssistantMessageData{Content: "old-a"},
		session.PresetChangeData{Preset: "review"},
		session.TodoWriteData{Items: []string{"old"}},
		session.GoalChangeData{GoalID: "g-old", Phase: session.GoalPhase("pending"), Description: "old", MaxRounds: 1, Revision: 1},
	}
	for _, d := range base {
		if _, err := sl.Append(d); err != nil {
			t.Fatal(err)
		}
	}

	baseline := session.FoldAll(sl.Events())

	sl.EnableIncrementalProjection()
	got := sl.Projection()
	if !session.SessionProjectionEqual(got, baseline) {
		t.Fatalf("warm start immediate got != baseline FoldAll")
	}
	stats1 := sl.IncrementalStats()

	if int(stats1.EventsScanned) < len(base) {
		t.Fatalf("warm start EventsScanned = %d, expect >= %d", stats1.EventsScanned, len(base))
	}

	extra := []session.EventData{
		session.UserMessageData{Content: "new-u"},
		session.AssistantMessageData{Content: "new-a"},
		session.TodoWriteData{Items: []string{"new1", "new2"}},
		session.PlanModeData{Mode: "on"},
		session.SessionTitleData{Title: "warm"},
	}
	for _, d := range extra {
		if _, err := sl.Append(d); err != nil {
			t.Fatal(err)
		}
	}
	got2 := sl.Projection()
	want2 := session.FoldAll(sl.Events())
	if !session.SessionProjectionEqual(got2, want2) {
		t.Fatalf("after warm start Append M items got != FoldAll()")
	}
	stats2 := sl.IncrementalStats()
	if stats2.IncrementalAppends != stats1.IncrementalAppends+uint64(len(extra)) {
		t.Fatalf("incremental Append count not accumulated correctly: stats1=%+v stats2=%+v", stats1, stats2)
	}
}

// ============================================================================

// ============================================================================

func TestH04SurfaceReplaceDirtyRebuild(t *testing.T) {
	sl := newTestLogH04(true)

	for _, c := range []string{"u1", "u2", "u3"} {
		if _, err := sl.Append(session.UserMessageData{Content: c}); err != nil {
			t.Fatal(err)
		}
	}

	before := sl.Projection()
	if len(before.Messages) != 3 {
		t.Fatalf("messages before = %d, want 3", len(before.Messages))
	}

	//

	if _, err := sl.Append(session.UserMessageData{Content: "u2-replaced"}); err != nil {
		t.Fatal(err)
	}

	evs := sl.Events()
	evs[3].SurfaceOp = &session.SurfaceOp{Op: session.SurfaceReplace, Start: 2, End: 2}

	f := session.NewIncrementalFolder()

	f.AppendMany(evs)
	snap := f.Snapshot(evs)
	expected := session.FoldAll(evs)
	if !session.SessionProjectionEqual(snap, expected) {
		t.Fatalf("incremental Snapshot != FoldAll() after SurfaceReplace\n got=%+v\nwant=%+v", snap, expected)
	}

	if len(snap.Messages) != 3 {
		t.Fatalf("message count after dirty rebuild = %d, want 3", len(snap.Messages))
	}
	if snap.Messages[0].Content != "u1" || snap.Messages[1].Content != "u3" || snap.Messages[2].Content != "u2-replaced" {
		t.Fatalf("messages after dirty rebuild = [%+v], order should be u1/u3/u2-replaced (EffectiveEvents hides seq=2, replacement preserves seq=4)", snap.Messages)
	}
	stats := f.StatsCopy()
	if stats.DirtyRebuilds != 1 {
		t.Fatalf("DirtyRebuilds after SurfaceReplace = %d, want 1", stats.DirtyRebuilds)
	}
}

// ============================================================================

// ============================================================================

func TestH04GoalCASIncremental(t *testing.T) {
	f := session.NewIncrementalFolder()

	f.Append(session.SessionEvent{Seq: 1, Type: session.EventGoalChange, Data: session.GoalChangeData{
		GoalID: "g", Revision: 1, Description: "r1", MaxRounds: 3,
	}})

	f.Append(session.SessionEvent{Seq: 2, Type: session.EventGoalChange, Data: session.GoalChangeData{
		GoalID: "g", Revision: 2, Description: "r2", MaxRounds: 3,
	}})

	f.Append(session.SessionEvent{Seq: 3, Type: session.EventGoalChange, Data: session.GoalChangeData{
		GoalID: "g", Revision: 1, Description: "stale", MaxRounds: 3,
	}})
	snap := f.Snapshot(nil)
	if !snap.Goal.Present || snap.Goal.Revision != 2 || snap.Goal.Description != "r2" {
		t.Fatalf("Goal CAS incremental failed: Goal=%+v", snap.Goal)
	}
}

func TestH04PresetSandboxApprovalDerivation(t *testing.T) {
	f := session.NewIncrementalFolder()
	check := func(t *testing.T, wantMode session.SandboxMode, wantPolicy session.ApprovalPolicy) {
		t.Helper()
		snap := f.Snapshot(nil)
		if snap.SandboxMode.Mode != wantMode {
			t.Fatalf("sandbox mode = %v, want %v", snap.SandboxMode.Mode, wantMode)
		}
		if snap.Approval.Policy != wantPolicy {
			t.Fatalf("approval policy = %v, want %v", snap.Approval.Policy, wantPolicy)
		}
	}
	f.Append(session.SessionEvent{Seq: 1, Type: session.EventPresetChange, Data: session.PresetChangeData{Preset: "safe"}})
	check(t, session.SandboxReadOnly, session.ApprovalAskDangerous)

	f.Append(session.SessionEvent{Seq: 2, Type: session.EventPresetChange, Data: session.PresetChangeData{Preset: "danger"}})
	check(t, session.SandboxDangerFullAccess, session.ApprovalAllowAll)

	f.Append(session.SessionEvent{Seq: 3, Type: session.EventPresetChange, Data: session.PresetChangeData{Preset: "review"}})
	check(t, session.SandboxReadOnly, session.ApprovalAskDangerousEdit)
}

// ============================================================================

// ============================================================================

func benchEventsH04(n int) []session.SessionEvent {
	evs := make([]session.SessionEvent, 0, n)
	for i := 0; i < n; i++ {
		seq := uint64(i + 1)
		switch i % 6 {
		case 0:
			evs = append(evs, session.SessionEvent{Seq: seq, Type: session.EventUserMessage, Data: session.UserMessageData{Content: "hello"}})
		case 1:
			evs = append(evs, session.SessionEvent{Seq: seq, Type: session.EventAssistantMessage, Data: session.AssistantMessageData{Content: "world"}})
		case 2:
			evs = append(evs, session.SessionEvent{Seq: seq, Type: session.EventRequestHeader, Data: session.RequestHeaderData{ConfigEpoch: uint64(i), SystemHash: "x"}})
		case 3:
			evs = append(evs, session.SessionEvent{Seq: seq, Type: session.EventTodoWrite, Data: session.TodoWriteData{Items: []string{"a", "b", "c"}}})
		case 4:
			evs = append(evs, session.SessionEvent{Seq: seq, Type: session.EventGoalChange, Data: session.GoalChangeData{GoalID: "g", Revision: seq, MaxRounds: 3}})
		case 5:
			evs = append(evs, session.SessionEvent{Seq: seq, Type: session.EventPlanMode, Data: session.PlanModeData{Mode: "on"}})
		}
	}
	return evs
}

func BenchmarkIncrementalH04FullBaseline(b *testing.B) {
	evs := benchEventsH04(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc := make([]session.SessionEvent, 0, len(evs))
		var lastGoalRev uint64
		for _, ev := range evs {
			acc = append(acc, ev)
			p := session.FoldAll(acc)
			lastGoalRev = p.Goal.Revision
		}
		_ = lastGoalRev
	}
}

func BenchmarkIncrementalH04IncrementalPath(b *testing.B) {
	evs := benchEventsH04(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := session.NewIncrementalFolder()
		var lastGoalRev uint64
		for _, ev := range evs {
			f.Append(ev)
			p := f.SnapshotMeta(nil)
			lastGoalRev = p.Goal.Revision
		}
		_ = lastGoalRev
	}
}
