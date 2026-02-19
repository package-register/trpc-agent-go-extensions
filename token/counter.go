package token

import (
	"context"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// SimpleCounter implements pipeline.TokenCounter using the framework's
// SimpleTokenCounter with a fallback to len(content)/4 on error.
type SimpleCounter struct {
	inner model.TokenCounter
}

// NewSimpleCounter creates a counter backed by the framework's SimpleTokenCounter.
func NewSimpleCounter() *SimpleCounter {
	return &SimpleCounter{
		inner: model.NewSimpleTokenCounter(),
	}
}

// Count estimates the total token count for a list of messages.
func (c *SimpleCounter) Count(ctx context.Context, msgs []model.Message) int {
	total, err := c.inner.CountTokensRange(ctx, msgs, 0, len(msgs))
	if err != nil {
		total = 0
		for _, m := range msgs {
			total += len(m.Content) / 4
		}
	}
	return total
}
