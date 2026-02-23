package token

import (
	"context"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// Counter estimates the total token count for a list of messages.
type Counter interface {
	Count(ctx context.Context, msgs []model.Message) int
}

// Observer is notified when token-related events occur (e.g. compression).
type Observer interface {
	OnCompression(beforeTokens, afterTokens int)
}
