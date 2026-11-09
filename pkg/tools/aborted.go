package tools

import "github.com/JopenChen/dsh-go/pkg/brand"

const AbortedBeforeDispatch = "TOOL_ABORTED_BEFORE_DISPATCH"

const AbortedBeforeDispatchMessage = "tool call aborted before dispatch"

func AbortedBeforeDispatchResult(callID brand.ToolCallID) *ToolCallResult {
	return &ToolCallResult{
		CallID:    callID,
		IsError:   true,
		Error:     AbortedBeforeDispatchMessage,
		ErrorCode: AbortedBeforeDispatch,
	}
}
