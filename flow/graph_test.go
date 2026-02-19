package flow

import (
	"context"
	"testing"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubModel struct{}

func (s stubModel) GenerateContent(_ context.Context, _ *model.Request) (<-chan *model.Response, error) { //nolint:unused
	ch := make(chan *model.Response)
	close(ch)
	return ch, nil
}

func (s stubModel) Info() model.Info {
	return model.Info{Name: "stub"}
}

type stubToolSet struct {
	name string
}

func (s stubToolSet) Name() string                        { return s.name }
func (s stubToolSet) Tools(_ context.Context) []tool.Tool { return nil }
func (s stubToolSet) Close() error                        { return nil }

func TestGraphBuilder_MultiStep(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{
			Frontmatter: pipeline.Frontmatter{
				Step:    "1.1",
				Title:   "设计大纲",
				Output:  pipeline.OutputField{"docs/a.md"},
				Next:    "1.2",
				Advance: pipeline.AdvanceAuto,
			},
			Body: "Step 1.1 body",
		},
		{
			Frontmatter: pipeline.Frontmatter{
				Step:     "1.2",
				Title:    "需求确认",
				Output:   pipeline.OutputField{"docs/b.md"},
				Fallback: map[string]string{"default": "1.1"},
				Advance:  pipeline.AdvanceConfirm,
			},
			Body: "Step 1.2 body",
		},
	}

	builder := NewGraphBuilder()
	g, err := builder.Build(steps, pipeline.FlowOptions{
		Model:    stubModel{},
		ToolSets: map[string]tool.ToolSet{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil graph")
	}
}

func TestGraphBuilder_WithTools(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{
			Frontmatter: pipeline.Frontmatter{
				Step:    "2.1",
				Title:   "RTL开发",
				Output:  pipeline.OutputField{"docs/rtl.md"},
				Next:    "3.1",
				Advance: pipeline.AdvanceAuto,
			},
			Body: "Step 2.1 body",
		},
		{
			Frontmatter: pipeline.Frontmatter{
				Step:    "3.1",
				Title:   "功能仿真",
				Tools:   []string{"eda"},
				Output:  pipeline.OutputField{"docs/sim.md"},
				Next:    "",
				Advance: pipeline.AdvanceAuto,
				Fallback: map[string]string{
					"default":       "2.1",
					"compile_error": "2.1",
				},
			},
			Body: "Step 3.1 body",
		},
	}

	builder := NewGraphBuilder()
	g, err := builder.Build(steps, pipeline.FlowOptions{
		Model:        stubModel{},
		ToolSets:     map[string]tool.ToolSet{"eda": stubToolSet{name: "eda"}},
		AllowMissing: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil graph")
	}
}

func TestGraphBuilder_EmptySteps(t *testing.T) {
	builder := NewGraphBuilder()
	_, err := builder.Build(nil, pipeline.FlowOptions{Model: stubModel{}})
	if err == nil {
		t.Fatal("expected error for empty steps")
	}
}

func TestGraphBuilder_DuplicateStep(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "1.1"}, Body: "a"},
		{Frontmatter: pipeline.Frontmatter{Step: "1.1"}, Body: "b"},
	}

	builder := NewGraphBuilder()
	_, err := builder.Build(steps, pipeline.FlowOptions{Model: stubModel{}})
	if err == nil {
		t.Fatal("expected error for duplicate step")
	}
}
