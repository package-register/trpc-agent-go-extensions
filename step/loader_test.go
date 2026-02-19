package step

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// testFS wraps fstest.MapFS to implement pipeline.FileSystem.
type testFS struct {
	fstest.MapFS
}

func (t testFS) Stat(name string) (fs.FileInfo, error) {
	f, err := t.MapFS.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (t testFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(t.MapFS, name)
}

const step11 = `---
step: "1.1"
title: "设计大纲"
output: "docs/a.md"
next: "1.2"
---
Step 1.1 body.
`

const step12 = `---
step: "1.2"
title: "需求确认"
output: "docs/b.md"
next: "2.1"
fallback:
  default: "1.1"
---
Step 1.2 body.
`

const step21 = `---
step: "2.1"
title: "RTL开发"
output: "docs/c.md"
---
Step 2.1 body.
`

func TestFileStepLoader_Normal(t *testing.T) {
	tfs := testFS{fstest.MapFS{
		"prompts/1.1_design.md":  &fstest.MapFile{Data: []byte(step11)},
		"prompts/1.2_confirm.md": &fstest.MapFile{Data: []byte(step12)},
		"prompts/2.1_rtl.md":     &fstest.MapFile{Data: []byte(step21)},
	}}

	loader := NewFileStepLoader(tfs, "prompts")
	steps, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	// Should be sorted by step ID
	if steps[0].Frontmatter.Step != "1.1" {
		t.Fatalf("expected first step=1.1, got %s", steps[0].Frontmatter.Step)
	}
	if steps[2].Frontmatter.Step != "2.1" {
		t.Fatalf("expected last step=2.1, got %s", steps[2].Frontmatter.Step)
	}
}

func TestFileStepLoader_SkipSystemDir(t *testing.T) {
	tfs := testFS{fstest.MapFS{
		"prompts/1.1_design.md":     &fstest.MapFile{Data: []byte(step11)},
		"prompts/system/core.md":    &fstest.MapFile{Data: []byte("system file")},
		"prompts/templates/tpl.md":  &fstest.MapFile{Data: []byte("template file")},
	}}

	loader := NewFileStepLoader(tfs, "prompts")
	steps, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step (system/ and templates/ skipped), got %d", len(steps))
	}
}

func TestFileStepLoader_SkipUnderscorePrefix(t *testing.T) {
	tfs := testFS{fstest.MapFS{
		"prompts/1.1_design.md":  &fstest.MapFile{Data: []byte(step11)},
		"prompts/_disabled.md":   &fstest.MapFile{Data: []byte(step12)},
	}}

	loader := NewFileStepLoader(tfs, "prompts")
	steps, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step (_ prefix skipped), got %d", len(steps))
	}
}

func TestFilteredStepLoader_ByPrefix(t *testing.T) {
	inner := NewInMemoryStepLoader(
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "1.1"}},
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "1.2"}},
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "2.1"}},
	)

	loader := NewFilteredStepLoader(inner, "1.")
	steps, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps with prefix '1.', got %d", len(steps))
	}
}

func TestFilteredStepLoader_NoMatch(t *testing.T) {
	inner := NewInMemoryStepLoader(
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "1.1"}},
	)

	loader := NewFilteredStepLoader(inner, "9.")
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for no matching steps")
	}
}

func TestCompositeStepLoader_Merge(t *testing.T) {
	l1 := NewInMemoryStepLoader(
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "1.1"}},
	)
	l2 := NewInMemoryStepLoader(
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "2.1"}},
	)

	loader := NewCompositeStepLoader(l1, l2)
	steps, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Frontmatter.Step != "1.1" || steps[1].Frontmatter.Step != "2.1" {
		t.Fatal("expected sorted order 1.1, 2.1")
	}
}

func TestCompositeStepLoader_DuplicateStep(t *testing.T) {
	l1 := NewInMemoryStepLoader(
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "1.1"}},
	)
	l2 := NewInMemoryStepLoader(
		&pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "1.1"}},
	)

	loader := NewCompositeStepLoader(l1, l2)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for duplicate step ID")
	}
}

func TestInMemoryStepLoader(t *testing.T) {
	s := &pipeline.StepDefinition{Frontmatter: pipeline.Frontmatter{Step: "1.1"}}
	loader := NewInMemoryStepLoader(s)
	steps, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 || steps[0].Frontmatter.Step != "1.1" {
		t.Fatal("expected single step 1.1")
	}
}

func TestInMemoryStepLoader_Empty(t *testing.T) {
	loader := NewInMemoryStepLoader()
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for empty loader")
	}
}
