package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// stubSnapshot implements pipeline.ContextSnapshot for testing.
type stubSnapshot struct {
	xml string
}

func (s stubSnapshot) BuildSnapshot(_ context.Context, _ string, _ *pipeline.StepDefinition) string {
	return s.xml
}

func TestAssembler_BuildStatic(t *testing.T) {
	fs := newTestFS(map[string]string{
		"core.md":  "You are a helpful assistant.",
		"tools.md": "Tool reference content.",
	})

	a := NewAssembler("core.md", "tools.md", fs, nil)

	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{
			Step:   "1.1",
			Output: pipeline.OutputField{"docs/output.md"},
		},
		Body: "Step body for {{stage}}, output to {{output_path}}.",
	}

	result, err := a.BuildStatic(step, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "<system_core_prompt>") {
		t.Fatal("expected Layer 1 marker")
	}
	if !strings.Contains(result, "You are a helpful assistant.") {
		t.Fatal("expected core prompt content")
	}
	if !strings.Contains(result, "<tools_reference>") {
		t.Fatal("expected tools reference")
	}
	if !strings.Contains(result, "Step body for 1.1, output to docs/output.md.") {
		t.Fatal("expected rendered body with template vars")
	}
}

func TestAssembler_BuildDynamic_WithSnapshot(t *testing.T) {
	fs := newTestFS(map[string]string{
		"core.md": "Core prompt.",
	})

	snap := stubSnapshot{xml: "<WorkflowContext>test progress</WorkflowContext>"}
	a := NewAssembler("core.md", "", fs, snap)

	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{Step: "2.1"},
		Body:        "Dynamic body.",
	}

	result, err := a.BuildDynamic(context.Background(), step, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "<WorkflowContext>test progress</WorkflowContext>") {
		t.Fatal("expected snapshot content in dynamic build")
	}
	if !strings.Contains(result, "Dynamic body.") {
		t.Fatal("expected body content")
	}
}

func TestAssembler_BuildDynamic_WithoutSnapshot(t *testing.T) {
	fs := newTestFS(map[string]string{
		"core.md": "Core prompt.",
	})

	a := NewAssembler("core.md", "", fs, nil)

	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{Step: "1.1"},
		Body:        "Body only.",
	}

	result, err := a.BuildDynamic(context.Background(), step, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Body only.") {
		t.Fatal("expected body content")
	}
	if strings.Contains(result, "<WorkflowContext>") {
		t.Fatal("should not contain workflow context without snapshot")
	}
}

func TestAssembler_HasDynamicContent(t *testing.T) {
	fs := newTestFS(nil)

	a1 := NewAssembler("", "", fs, stubSnapshot{})
	if !a1.HasDynamicContent() {
		t.Fatal("expected HasDynamicContent=true with snapshot")
	}

	a2 := NewAssembler("", "", fs, nil)
	if a2.HasDynamicContent() {
		t.Fatal("expected HasDynamicContent=false without snapshot")
	}
}

func TestAssembler_TemplateVars(t *testing.T) {
	fs := newTestFS(map[string]string{"core.md": ""})
	a := NewAssembler("core.md", "", fs, nil)

	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{
			Step:   "3.1",
			Output: pipeline.OutputField{"sim/output.vcd"},
		},
		Body: "Run simulation for {{stage}}, save to {{output_path}}, base={{base_dir}}.",
	}

	result, err := a.BuildStatic(step, map[string]string{"base_dir": "/workspace"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Run simulation for 3.1, save to sim/output.vcd, base=/workspace.") {
		t.Fatalf("template vars not rendered correctly: %s", result)
	}
}

func TestAssembler_ImplementsInterface(t *testing.T) {
	fs := newTestFS(nil)
	var _ pipeline.PromptAssembler = NewAssembler("", "", fs, nil)
}
