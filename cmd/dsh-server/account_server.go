package main

import (
	"context"
	"time"

	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const (
	deepSeekUsageURL = "https://platform.deepseek.com/usage"
	deepSeekTopUpURL = "https://platform.deepseek.com/top_up"
)

func signedOutAccountView() map[string]any {
	return map[string]any{
		"status": "signed-out",
		"links":  map[string]any{"usageUrl": deepSeekUsageURL, "topUpUrl": deepSeekTopUpURL},
	}
}

func (s *Server) dispatchAccountRPC(endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "account/getState":
		return signedOutAccountView(), nil
	case "account/startSignIn":
		attemptID := uuid.MustV4()
		view := signedOutAccountView()
		view["attempt"] = map[string]any{
			"id": attemptID, "phase": "failed", "errorCode": "protocol",
			"expiresAt": time.Now().Add(5 * time.Minute).UnixMilli(),
		}
		return view, nil
	case "account/cancelSignIn", "account/signOut":
		return signedOutAccountView(), nil
	case "account/getProfile", "account/getBalance":
		return nil, nil
	case "account/hasRunningAccountTasks":
		return false, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown account method"}
	}
}

func (s *Server) openAccountWatch(ctx context.Context, send func(map[string]any) error) {
	if err := send(signedOutAccountView()); err != nil {
		return
	}
	<-ctx.Done()
}
