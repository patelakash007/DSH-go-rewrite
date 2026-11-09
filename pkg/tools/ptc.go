//

//

//

package tools

import (
	"context"
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/coderuntime"
)

const RunCodeName = "run_code"

type RunCodeInput struct {
	Code string `json:"code"`

	Description string `json:"description"`
}

type ToolLookup func(name string) (*Tool, bool)

//

func NewRunCodeTool(runtime coderuntime.Runtime, lookup ToolLookup, pipeline *Pipeline) *Tool {
	if lookup == nil {
		lookup = func(string) (*Tool, bool) { return nil, false }
	}
	execute := func(ctx context.Context, input map[string]any) (any, error) {
		var in RunCodeInput
		in.Code, _ = input["code"].(string)
		in.Description, _ = input["description"].(string)
		if in.Code == "" {
			return nil, fmt.Errorf("run_code: empty code")
		}
		if in.Description == "" {
			return nil, fmt.Errorf("run_code: empty description")
		}

		ns := coderuntime.BindingNamespace{
			Global:     "tools",
			ErrorClass: "ToolError",
			Functions:  map[string]coderuntime.BindingFunction{},
		}

		ns.Functions["__dispatch__"] = func(c context.Context, args any) (any, error) {
			m, ok := args.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid dispatch args")
			}
			name, _ := m["name"].(string)
			toolArgs, _ := m["arguments"].(map[string]any)
			return invokeToolInProgram(c, lookup, pipeline, name, toolArgs)
		}

		res, err := runtime.Run(&coderuntime.RunRequest{
			Program:  in.Code,
			Bindings: []coderuntime.BindingNamespace{ns},
			Ctx:      ctx,
		})
		if err != nil {
			return nil, fmt.Errorf("run_code runtime error: %w", err)
		}
		if res.Failed() {

			return nil, fmt.Errorf("run_code %s: %s", res.Failure.Kind, res.Failure.Message)
		}
		return PresentRunCodeResult(res), nil
	}

	return &Tool{
		Name:        RunCodeName,
		Description: "Execute a program against the available tools; body of an async function with top-level await/return.",
		Execute:     execute,
	}
}

func invokeToolInProgram(ctx context.Context, lookup ToolLookup, pipeline *Pipeline, name string, args map[string]any) (any, error) {
	tool, ok := lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	req := &ToolCallRequest{
		CallID: brand.NewToolCallID(fmt.Sprintf("ptc-%s", name)),
		Tool:   name,
		Input:  args,
	}
	if pipeline == nil {
		pipeline = NewPipeline().WithTool(tool)
	} else {

		pipeline = pipeline.WithTool(tool)
	}
	result := pipeline.Run(ctx, req, tool)
	if result.IsError {
		return nil, fmt.Errorf("%s", result.Error)
	}
	return result.Value, nil
}

type RunCodeOutput struct {
	Logs   []string `json:"logs"`
	Result any      `json:"result,omitempty"`
}

func PresentRunCodeResult(res *coderuntime.RunResult) RunCodeOutput {
	return RunCodeOutput{Logs: res.Logs, Result: res.Value}
}
