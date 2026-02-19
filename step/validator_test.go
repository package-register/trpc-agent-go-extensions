package step

import (
	"testing"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

func TestValidateReferences_Valid(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "1.1", Next: "1.2"}},
		{Frontmatter: pipeline.Frontmatter{Step: "1.2", Next: "2.1", Fallback: map[string]string{"default": "1.1"}}},
		{Frontmatter: pipeline.Frontmatter{Step: "2.1"}},
	}

	errs := ValidateReferences(steps)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateReferences_DanglingNext(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "1.1", Next: "9.9"}},
	}

	errs := ValidateReferences(steps)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "next" || errs[0].Reference != "9.9" {
		t.Fatalf("unexpected error: %v", errs[0])
	}
}

func TestValidateReferences_DanglingFallback(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "1.1", Fallback: map[string]string{"default": "5.5"}}},
	}

	errs := ValidateReferences(steps)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "fallback.default" {
		t.Fatalf("unexpected field: %s", errs[0].Field)
	}
}

func TestValidateReferences_SelfLoop(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "1.1", Next: "1.1"}},
	}

	errs := ValidateReferences(steps)
	found := false
	for _, e := range errs {
		if e.Message == "self-loop detected" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected self-loop error")
	}
}

func TestValidateReferences_NullNext(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "9.1", Next: ""}},
	}

	errs := ValidateReferences(steps)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty next (terminal step), got %d", len(errs))
	}
}

func TestValidateReferences_MultipleDangling(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "1.1", Next: "1.2"}},
		{Frontmatter: pipeline.Frontmatter{Step: "1.2", Next: "3.0", Fallback: map[string]string{"default": "4.0"}}},
	}

	errs := ValidateReferences(steps)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}
