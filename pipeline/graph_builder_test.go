package pipeline

import (
	"context"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubModel struct{}

func (s stubModel) GenerateContent(ctx context.Context, request *model.Request) (<-chan *model.Response, error) {
	ch := make(chan *model.Response)
	close(ch)
	return ch, nil
}

func (s stubModel) Info() model.Info {
	return model.Info{Name: "stub"}
}

type stubToolSet struct{ name string }

func (s stubToolSet) Tools(context.Context) []tool.Tool { return nil }
func (s stubToolSet) Close() error                      { return nil }
func (s stubToolSet) Name() string                      { return s.name }

func TestMakeConfirmNode_AutoAdvanceDoesNotInterrupt(t *testing.T) {
	node := makeConfirmNode("1.1", AdvanceAuto)
	result, err := node(context.Background(), graph.State{})
	if err != nil {
		t.Fatalf("expected no interrupt, got %v", err)
	}
	if _, ok := result.(graph.State); !ok {
		t.Fatalf("expected graph.State result, got %T", result)
	}
}

func TestMakeConfirmNode_ConfirmInterrupts(t *testing.T) {
	node := makeConfirmNode("1.1", AdvanceConfirm)
	_, err := node(context.Background(), graph.State{})
	if err == nil {
		t.Fatalf("expected interrupt error")
	}
	if !graph.IsInterruptError(err) {
		t.Fatalf("expected interrupt error type, got %T", err)
	}
}

func TestBuildGraphFromPrompts_WithFallbackConfirm(t *testing.T) {
	prompts := []*PromptFile{
		{
			Path: "1.1.md",
			Frontmatter: Frontmatter{
				Step:     "1.1",
				Title:    "规划",
				Output:   OutputField{"docs/01.md"},
				MCP:      []string{"eda"},
				Next:     "2.1",
				Advance:  AdvanceConfirm,
				Fallback: map[string]string{"default": "2.1"},
			},
			Body: "# 规划",
		},
		{
			Path: "2.1.md",
			Frontmatter: Frontmatter{
				Step:   "2.1",
				Title:  "实现",
				Output: OutputField{"docs/02.md"},
			},
			Body: "# 实现",
		},
	}

	graph, err := BuildGraphFromPrompts(prompts, BuildOptions{
		Model:    stubModel{},
		ToolSets: map[string]tool.ToolSet{"eda": stubToolSet{name: "eda"}},
	})
	if err != nil {
		t.Fatalf("build graph error: %v", err)
	}

	if _, ok := graph.Node("1.1:confirm"); !ok {
		t.Fatalf("expected confirm node")
	}

	stepCond, ok := graph.ConditionalEdge("1.1")
	if !ok {
		t.Fatalf("expected step conditional edge")
	}
	if stepCond.PathMap["1.1:tools"] != "1.1:tools" {
		t.Fatalf("unexpected tools path: %s", stepCond.PathMap["1.1:tools"])
	}
	if stepCond.PathMap["1.1:confirm"] != "1.1:confirm" {
		t.Fatalf("unexpected confirm path: %s", stepCond.PathMap["1.1:confirm"])
	}

	toolsCond, ok := graph.ConditionalEdge("1.1:tools")
	if !ok {
		t.Fatalf("expected tools conditional edge")
	}
	if toolsCond.PathMap["success"] != "1.1" {
		t.Fatalf("unexpected tools success path: %s", toolsCond.PathMap["success"])
	}
	if toolsCond.PathMap["default"] != "2.1" {
		t.Fatalf("unexpected tools default path: %s", toolsCond.PathMap["default"])
	}

	edges := graph.Edges("1.1:confirm")
	if len(edges) != 1 || edges[0].To != "2.1" {
		t.Fatalf("expected confirm edge to 2.1")
	}
}
