package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sysprompt"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func TestSyspromptSectionOrdering(t *testing.T) {
	a := sysprompt.New()

	a.Register("tools", sysprompt.SectionOrderToolsSchema, "TOOLS_SECTION")
	a.Register("persona", sysprompt.SectionOrderPersona, "PERSONA_SECTION")
	a.Register("plan_policy", sysprompt.SectionOrderPlanPolicy, "PLAN_POLICY_SECTION")
	a.Register("policy", sysprompt.SectionOrderPolicy, "POLICY_SECTION")
	a.Register("runtime_ctx", sysprompt.SectionOrderRuntimeContext, "RUNTIME_CTX_SECTION")

	out := a.Assemble()

	idxPersona := strings.Index(out, "PERSONA_SECTION")
	idxPolicy := strings.Index(out, "POLICY_SECTION")
	idxCtx := strings.Index(out, "RUNTIME_CTX_SECTION")
	idxPlan := strings.Index(out, "PLAN_POLICY_SECTION")
	idxTools := strings.Index(out, "TOOLS_SECTION")

	if !(idxPersona < idxPolicy && idxPolicy < idxCtx && idxCtx < idxPlan && idxPlan < idxTools) {
		t.Fatalf("section ordering error: persona=%d policy=%d ctx=%d plan=%d tools=%d",
			idxPersona, idxPolicy, idxCtx, idxPlan, idxTools)
	}
}

func TestSyspromptRegisterOverwrite(t *testing.T) {
	a := sysprompt.New()
	a.Register("policy", sysprompt.SectionOrderPolicy, "v1")
	a.Register("policy", sysprompt.SectionOrderPolicy, "v2")

	out := a.Assemble()
	if !strings.Contains(out, "v2") || strings.Contains(out, "v1") {
		t.Fatalf("overwrite with same name failed: %q", out)
	}

	a.Unregister("policy")
	if strings.Contains(a.Assemble(), "v2") {
		t.Fatal("policy section should not appear after Unregister")
	}
	if a.Has("policy") {
		t.Fatal("Has should be false after Unregister")
	}
}

func TestSyspromptOrderConstants(t *testing.T) {
	if sysprompt.SectionOrderPersona != 100 {
		t.Fatal("persona order should be 100")
	}
	if sysprompt.SectionOrderPolicy != 200 {
		t.Fatal("policy order should be 200")
	}
	if sysprompt.SectionOrderRuntimeContext != 300 {
		t.Fatal("runtime-context order should be 300")
	}
	if sysprompt.SectionOrderPlanPolicy != 500 {
		t.Fatal("plan:policy order should be 500")
	}
	if sysprompt.SectionOrderToolsSchema != 600 {
		t.Fatal("tools schema order should be 600")
	}
}

func TestSyspromptToolsSchemaInjection(t *testing.T) {

	toolList := []*tools.Tool{
		{Name: "zebra_tool", Description: "z tool", Schema: mustCompile(t, `{"type":"object","properties":{"z":{"type":"string"}}}`)},
		{Name: "alpha_tool", Description: "a tool", Schema: mustCompile(t, `{"type":"object","properties":{"a":{"type":"integer"}}}`)},
	}

	text1, err := sysprompt.ToolsSectionText(toolList)
	if err != nil {
		t.Fatalf("ToolsSectionText failed: %v", err)
	}
	text2, _ := sysprompt.ToolsSectionText(toolList)

	if text1 != text2 {
		t.Fatal("tools schema output non-deterministic")
	}

	idxAlpha := strings.Index(text1, "alpha_tool")
	idxZebra := strings.Index(text1, "zebra_tool")
	if !(idxAlpha > 0 && idxZebra > idxAlpha) {
		t.Fatalf("tools should be lexicographical: %q", text1)
	}

	jsonStart := strings.Index(text1, "[")
	if jsonStart < 0 {
		t.Fatal("tools schema should contain JSON array")
	}
	var parsed []any
	if err := json.Unmarshal([]byte(text1[jsonStart:]), &parsed); err != nil {
		t.Fatalf("tools schema is not valid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("should contain 2 tools: %d", len(parsed))
	}
}

func mustCompile(t *testing.T, raw string) *tools.JsonSchemaNode {
	t.Helper()
	node, err := tools.Compile([]byte(raw))
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	return node
}

func TestSyspromptEmpty(t *testing.T) {
	a := sysprompt.New()
	if a.Assemble() != "" {
		t.Fatalf("empty assembler should return empty string: %q", a.Assemble())
	}
}
