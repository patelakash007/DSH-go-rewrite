package main

import "github.com/JopenChen/dsh-go/pkg/uuid"

func (s *Server) dispatchAppAuthRPC(endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "appAuth/list":
		return map[string]any{"providers": []map[string]any{
			{"id": "openai-codex", "signedIn": false},
			{"id": "anthropic", "signedIn": false},
		}}, nil
	case "appAuth/begin":
		return map[string]any{"attemptId": uuid.MustV4()}, nil
	case "appAuth/poll":
		return map[string]any{
			"phase": "failed",
			"error": "Subscription sign-in is not available in the native engine yet",
		}, nil
	case "appAuth/answer", "appAuth/cancel", "appAuth/signOut":
		return map[string]any{}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown app auth method"}
	}
}
