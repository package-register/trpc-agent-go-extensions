package memory

import (
	"context"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// stubTokenCounter returns a fixed token count.
type stubTokenCounter struct {
	count int
}

func (s stubTokenCounter) Count(_ context.Context, _ []model.Message) int {
	return s.count
}

// stubModel returns a fixed summary from GenerateContent.
type stubModel struct {
	summary string
	err     error
}

func (s stubModel) Info() model.Info {
	return model.Info{Name: "stub"}
}

func (s stubModel) GenerateContent(_ context.Context, req *model.Request) (<-chan *model.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	ch := make(chan *model.Response, 1)
	ch <- &model.Response{
		Choices: []model.Choice{
			{Message: model.Message{Content: s.summary}},
		},
	}
	close(ch)
	return ch, nil
}

func TestLLMCompressor_BelowThreshold(t *testing.T) {
	c := NewLLMCompressor(stubModel{summary: "test"}, stubTokenCounter{count: 100}, 10000, 0.7, 3)
	msgs := []model.Message{
		model.NewSystemMessage("system"),
		model.NewUserMessage("hello"),
	}
	result, didCompress, err := c.CompressIfNeeded(context.Background(), msgs, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if didCompress {
		t.Fatal("should not compress below threshold")
	}
	if len(result) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(result))
	}
}

func TestLLMCompressor_AboveThreshold(t *testing.T) {
	c := NewLLMCompressor(stubModel{summary: "compressed summary"}, stubTokenCounter{count: 8000}, 10000, 0.7, 1)

	msgs := []model.Message{
		model.NewSystemMessage("system prompt"),
		model.NewUserMessage("msg1"),
		{Role: model.RoleAssistant, Content: "reply1"},
		model.NewUserMessage("msg2"),
		{Role: model.RoleAssistant, Content: "reply2"},
		model.NewUserMessage("msg3"),
		{Role: model.RoleAssistant, Content: "reply3"},
	}

	result, didCompress, err := c.CompressIfNeeded(context.Background(), msgs, 8000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !didCompress {
		t.Fatal("should have compressed above threshold")
	}
	// Should have: system + summary + last 2 msgs (keepRecentTurns=1 → 2 msgs)
	if len(result) < 3 {
		t.Fatalf("expected at least 3 messages after compression, got %d", len(result))
	}
	// Check summary message exists
	found := false
	for _, m := range result {
		if IsSummaryMessage(m.Content) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected summary message in compressed result")
	}
}

func TestLLMCompressor_TooFewMessages(t *testing.T) {
	c := NewLLMCompressor(stubModel{summary: "test"}, stubTokenCounter{count: 8000}, 10000, 0.7, 3)

	msgs := []model.Message{
		model.NewSystemMessage("system"),
		model.NewUserMessage("hello"),
		{Role: model.RoleAssistant, Content: "hi"},
	}

	result, didCompress, err := c.CompressIfNeeded(context.Background(), msgs, 8000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if didCompress {
		t.Fatal("should not compress when too few conversation messages")
	}
	if len(result) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(result))
	}
}

func TestLLMCompressor_ZeroContextWindow(t *testing.T) {
	c := NewLLMCompressor(stubModel{summary: "test"}, stubTokenCounter{count: 100}, 0, 0.7, 3)
	msgs := []model.Message{model.NewUserMessage("hello")}

	_, didCompress, err := c.CompressIfNeeded(context.Background(), msgs, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if didCompress {
		t.Fatal("should not compress with zero context window")
	}
}

func TestLLMCompressor_DefaultParams(t *testing.T) {
	c := NewLLMCompressor(stubModel{summary: "test"}, stubTokenCounter{count: 0}, 10000, 0, 0)
	if c.threshold != 0.7 {
		t.Fatalf("expected default threshold 0.7, got %f", c.threshold)
	}
	if c.keepRecentTurns != 3 {
		t.Fatalf("expected default keepRecentTurns 3, got %d", c.keepRecentTurns)
	}
}
