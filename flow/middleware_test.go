package flow

import (
	"context"
	"testing"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// stubMiddleware implements pipeline.Middleware for testing.
type stubMiddleware struct {
	preResult  graph.State
	postCalled bool
}

func (s *stubMiddleware) WrapPreNode(_ string, _ *pipeline.StepDefinition) graph.BeforeNodeCallback {
	if s.preResult == nil {
		return nil
	}
	return func(_ context.Context, _ *graph.NodeCallbackContext, _ graph.State) (any, error) {
		return s.preResult, nil
	}
}

func (s *stubMiddleware) WrapPostNode(_ string, _ *pipeline.StepDefinition) graph.AfterNodeCallback {
	return func(_ context.Context, _ *graph.NodeCallbackContext, _ graph.State, _ any, nodeErr error) (any, error) {
		s.postCalled = true
		return nil, nodeErr
	}
}

func TestMiddlewareChain_PreNode(t *testing.T) {
	mw1 := &stubMiddleware{preResult: graph.State{"key1": "val1"}}
	mw2 := &stubMiddleware{preResult: graph.State{"key2": "val2"}}

	chain := NewMiddlewareChain(mw1, mw2)
	cb := chain.WrapPreNode("1.1", &pipeline.StepDefinition{})
	if cb == nil {
		t.Fatal("expected non-nil callback")
	}

	result, err := cb(context.Background(), nil, graph.State{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st, ok := result.(graph.State)
	if !ok {
		t.Fatal("expected graph.State result")
	}
	if st["key1"] != "val1" || st["key2"] != "val2" {
		t.Fatalf("expected merged state, got %v", st)
	}
}

func TestMiddlewareChain_Empty(t *testing.T) {
	chain := NewMiddlewareChain()
	cb := chain.WrapPreNode("1.1", &pipeline.StepDefinition{})
	if cb != nil {
		t.Fatal("expected nil callback for empty chain")
	}
}

func TestMiddlewareChain_PostNode(t *testing.T) {
	mw := &stubMiddleware{}
	chain := NewMiddlewareChain(mw)

	cb := chain.WrapPostNode("1.1", &pipeline.StepDefinition{})
	if cb == nil {
		t.Fatal("expected non-nil post callback")
	}

	_, err := cb(context.Background(), nil, graph.State{}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mw.postCalled {
		t.Fatal("expected post callback to be called")
	}
}

// stubCompressor implements pipeline.Compressor for testing.
type stubCompressor struct {
	shouldCompress bool
}

func (s stubCompressor) CompressIfNeeded(_ context.Context, msgs []model.Message, _ int) ([]model.Message, bool, error) {
	if s.shouldCompress {
		return msgs[:1], true, nil
	}
	return msgs, false, nil
}

// stubCounter implements pipeline.TokenCounter for testing.
type stubCounter struct {
	count int
}

func (s stubCounter) Count(_ context.Context, _ []model.Message) int {
	return s.count
}

// stubObserver implements pipeline.TokenObserver for testing.
type stubObserver struct {
	called bool
}

func (s *stubObserver) OnCompression(_, _ int) {
	s.called = true
}

func TestCompressionMiddleware_Triggered(t *testing.T) {
	obs := &stubObserver{}
	mw := NewCompressionMiddleware(stubCompressor{shouldCompress: true}, stubCounter{count: 8000}, obs)

	cb := mw.WrapPreNode("1.1", &pipeline.StepDefinition{})
	if cb == nil {
		t.Fatal("expected non-nil callback")
	}

	state := graph.State{
		graph.StateKeyMessages: []model.Message{
			model.NewSystemMessage("sys"),
			model.NewUserMessage("hello"),
			{Role: model.RoleAssistant, Content: "hi"},
		},
	}

	result, err := cb(context.Background(), nil, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when compression triggered")
	}
	if !obs.called {
		t.Fatal("expected observer to be called")
	}
}

func TestCompressionMiddleware_NotTriggered(t *testing.T) {
	mw := NewCompressionMiddleware(stubCompressor{shouldCompress: false}, stubCounter{count: 100}, nil)

	cb := mw.WrapPreNode("1.1", &pipeline.StepDefinition{})
	state := graph.State{
		graph.StateKeyMessages: []model.Message{
			model.NewSystemMessage("sys"),
			model.NewUserMessage("hello"),
		},
	}

	result, err := cb(context.Background(), nil, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result when compression not triggered")
	}
}

func TestArtifactRecordMiddleware_Record(t *testing.T) {
	tracker := &stubTracker{}
	mw := NewArtifactRecordMiddleware(tracker)

	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{
			Title:  "设计大纲",
			Output: pipeline.OutputField{"docs/output.md"},
		},
	}

	cb := mw.WrapPostNode("1.1", step)
	if cb == nil {
		t.Fatal("expected non-nil post callback")
	}

	_, err := cb(context.Background(), nil, graph.State{}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tracker.recorded {
		t.Fatal("expected tracker.RecordCompleted to be called")
	}
}

func TestArtifactRecordMiddleware_NoOutput(t *testing.T) {
	tracker := &stubTracker{}
	mw := NewArtifactRecordMiddleware(tracker)

	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{Title: "No Output"},
	}

	cb := mw.WrapPostNode("1.1", step)
	if cb != nil {
		t.Fatal("expected nil post callback for step without output")
	}
}

// stubTracker implements pipeline.ArtifactTracker for testing.
type stubTracker struct {
	recorded bool
}

func (s *stubTracker) RecordCompleted(_, _, _ string) bool {
	s.recorded = true
	return true
}
func (s *stubTracker) GetArtifact(_ string) *pipeline.ArtifactInfo { return nil }
func (s *stubTracker) GetAll() map[string]*pipeline.ArtifactInfo    { return nil }
