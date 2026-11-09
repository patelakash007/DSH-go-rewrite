//

package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/tools"
)

func mkTool(name string) *tools.Tool {
	return &tools.Tool{Name: name, Execute: func(ctx context.Context, input map[string]any) (any, error) {
		return "ok", nil
	}}
}

func TestToolRestrictionHostDenyScopeExempt(t *testing.T) {
	rs := tools.NewRestrictionSet()

	rs.Host(tools.RestrictionDeny("bash", "rm"))

	rs.Scope("session:main", tools.RestrictionAllow("rm"))

	if !rs.Allowed("rm") {
		t.Fatal("scope exempt should override host deny, rm should be available")
	}
	if rs.Allowed("bash") {
		t.Fatal("bash without scope exempt should still be rejected by host deny")
	}
	if !rs.Allowed("goal_list") {
		t.Fatal("unmentioned tools should pass by default")
	}
}

func TestToolRestrictionIntersect(t *testing.T) {
	rs := tools.NewRestrictionSet()
	rs.Host(tools.RestrictionDeny("rm", "bash"))
	rs.Scope("session:child", tools.RestrictionDeny("bash"))

	if rs.Allowed("bash") {
		t.Fatal("both layers deny bash, should reject")
	}
	if !rs.Allowed("ls") {
		t.Fatal("unmentioned ls should pass")
	}
}

func TestToolRestrictionNearestScopeWins(t *testing.T) {
	rs := tools.NewRestrictionSet()
	rs.Host(tools.RestrictionAllow("web_fetch"))
	rs.Scope("session:a", tools.RestrictionDeny("web_fetch"))

	if rs.Allowed("web_fetch") {
		t.Fatal("nearest session layer deny should override host allow")
	}
}

func TestToolRestrictionFilterHideForPreset(t *testing.T) {
	defs := []*tools.Tool{mkTool("bash"), mkTool("rm"), mkTool("ls"), mkTool("goal_list")}
	rs := tools.NewRestrictionSet()
	rs.Host(tools.RestrictionDeny("bash", "rm"))

	kept := rs.Filter(defs)
	if len(kept) != 2 {
		t.Fatalf("should retain 2 tools (lts/goal), got %v", names(kept))
	}
	if kept[0].Name != "ls" || kept[1].Name != "goal_list" {
		t.Fatalf("retained order/content error: %v", names(kept))
	}
}

func TestToolRestrictionSubagentParentLimitsChild(t *testing.T) {
	defs := []*tools.Tool{mkTool("bash"), mkTool("fs_edit"), mkTool("goal_list")}

	rs := tools.NewRestrictionSet()
	rs.Host(tools.RestrictionDeny("bash", "fs_edit"))

	rs.Scope("subagent:child", tools.RestrictionAllow("goal_list"))

	kept := rs.Filter(defs)
	if len(kept) != 1 || kept[0].Name != "goal_list" {
		t.Fatalf("subagent should only retain goal_list, got %v", names(kept))
	}
}

func names(ts []*tools.Tool) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		if t != nil {
			out = append(out, t.Name)
		}
	}
	return out
}
