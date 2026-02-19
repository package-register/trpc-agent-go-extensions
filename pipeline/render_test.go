package pipeline

import "testing"

func TestRenderTemplate(t *testing.T) {
	input := "Hello {{name}}, output: {{output_path}}."
	vars := map[string]string{
		"name":        "WitGlow",
		"output_path": "docs/report.md",
	}

	got := RenderTemplate(input, vars)
	want := "Hello WitGlow, output: docs/report.md."
	if got != want {
		t.Fatalf("unexpected render: got %q want %q", got, want)
	}
}

func TestRenderTemplateEmptyVars(t *testing.T) {
	input := "Hello {{name}}"
	got := RenderTemplate(input, nil)
	if got != input {
		t.Fatalf("expected input unchanged, got %q", got)
	}
}
