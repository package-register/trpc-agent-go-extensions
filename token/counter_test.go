package token

import (
	"context"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

func TestSimpleCounter_Normal(t *testing.T) {
	c := NewSimpleCounter()
	msgs := []model.Message{
		model.NewUserMessage("hello world"),
		model.NewSystemMessage("you are a helpful assistant"),
	}
	count := c.Count(context.Background(), msgs)
	if count <= 0 {
		t.Fatalf("expected positive token count, got %d", count)
	}
}

func TestSimpleCounter_EmptyMessages(t *testing.T) {
	c := NewSimpleCounter()
	count := c.Count(context.Background(), nil)
	if count != 0 {
		t.Fatalf("expected 0 for nil messages, got %d", count)
	}

	count = c.Count(context.Background(), []model.Message{})
	if count != 0 {
		t.Fatalf("expected 0 for empty messages, got %d", count)
	}
}

func TestSimpleCounter_LargeContent(t *testing.T) {
	c := NewSimpleCounter()
	longContent := make([]byte, 10000)
	for i := range longContent {
		longContent[i] = 'a'
	}
	msgs := []model.Message{
		model.NewUserMessage(string(longContent)),
	}
	count := c.Count(context.Background(), msgs)
	if count <= 0 {
		t.Fatalf("expected positive token count for large content, got %d", count)
	}
}
