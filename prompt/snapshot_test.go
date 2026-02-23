package prompt

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/package-register/trpc-agent-go-extensions/memory"
	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// stubTracker implements memory.ArtifactTracker for testing.
type stubTracker struct {
	data map[string]*memory.ArtifactInfo
}

func (s stubTracker) RecordCompleted(_, _, _ string) bool { return false }
func (s stubTracker) GetArtifact(stepID string) *memory.ArtifactInfo {
	return s.data[stepID]
}
func (s stubTracker) GetAll() map[string]*memory.ArtifactInfo {
	return s.data
}

// stubSummarizer implements InputSummarizer for testing.
type stubSummarizer struct {
	summary string
}

func (s stubSummarizer) Summarize(_ context.Context, _ string) (string, error) {
	return s.summary, nil
}

func TestSnapshot_BuildProgress(t *testing.T) {
	steps := []*pipeline.StepDefinition{
		{Frontmatter: pipeline.Frontmatter{Step: "1.1", Title: "设计大纲", Output: pipeline.OutputField{"docs/a.md"}}},
		{Frontmatter: pipeline.Frontmatter{Step: "1.2", Title: "需求确认", Output: pipeline.OutputField{"docs/b.md"}}},
		{Frontmatter: pipeline.Frontmatter{Step: "1.3", Title: "规划", Output: pipeline.OutputField{"docs/c.md"}}},
	}

	tracker := stubTracker{data: map[string]*memory.ArtifactInfo{
		"1.1": {StepID: "1.1", Status: "completed", LineCount: 50},
	}}

	snap := NewSnapshot(steps, tracker, stubSummarizer{}, nil, newTestFS(nil))
	result := snap.BuildSnapshot(context.Background(), "1.2", steps[1])

	if !strings.Contains(result, "✅ 1.1 设计大纲") {
		t.Fatal("expected completed step 1.1")
	}
	if !strings.Contains(result, "🔄 1.2 需求确认") {
		t.Fatal("expected current step 1.2")
	}
	if !strings.Contains(result, "⬚ 1.3 规划") {
		t.Fatal("expected pending step 1.3")
	}
	if !strings.Contains(result, "第2步/共3步") {
		t.Fatal("expected progress counter")
	}
}

func TestSnapshot_BuildInputSummaries(t *testing.T) {
	fs := testFS{fstest.MapFS{
		"docs/input.md": &fstest.MapFile{Data: []byte("test content")},
	}}

	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{
			Step:  "1.2",
			Input: []string{"docs/input.md", "docs/missing.md"},
		},
	}

	snap := NewSnapshot(nil, stubTracker{data: map[string]*memory.ArtifactInfo{}}, stubSummarizer{summary: "摘要内容"}, nil, fs)
	result := snap.BuildSnapshot(context.Background(), "1.2", step)

	if !strings.Contains(result, "摘要内容") {
		t.Fatal("expected summarizer output")
	}
	if !strings.Contains(result, `status="not_found"`) {
		t.Fatal("expected not_found for missing file")
	}
}

func TestSnapshot_BuildAvailableTools(t *testing.T) {
	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{
			Step:  "3.1",
			Tools: []string{"eda", "file"},
		},
	}

	toolNames := func(name string) []string {
		switch name {
		case "eda":
			return []string{"simulate_verilog", "synthesize_verilog"}
		case "file":
			return []string{"file_read", "file_write"}
		}
		return nil
	}

	snap := NewSnapshot(nil, stubTracker{data: map[string]*memory.ArtifactInfo{}}, stubSummarizer{}, toolNames, newTestFS(nil))
	result := snap.BuildSnapshot(context.Background(), "3.1", step)

	if !strings.Contains(result, "[eda] 2个工具: simulate_verilog, synthesize_verilog") {
		t.Fatal("expected eda tools listed")
	}
	if !strings.Contains(result, "[file] 2个工具: file_read, file_write") {
		t.Fatal("expected file tools listed")
	}
}

func TestSnapshot_BuildAvailableTools_NoTools(t *testing.T) {
	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{Step: "1.1"},
	}

	snap := NewSnapshot(nil, stubTracker{data: map[string]*memory.ArtifactInfo{}}, stubSummarizer{}, nil, newTestFS(nil))
	result := snap.BuildSnapshot(context.Background(), "1.1", step)

	if !strings.Contains(result, "当前步骤无额外工具") {
		t.Fatal("expected no-tools message")
	}
}

func TestSnapshot_BuildOutputContract(t *testing.T) {
	step := &pipeline.StepDefinition{
		Frontmatter: pipeline.Frontmatter{
			Step:     "1.2",
			Output:   pipeline.OutputField{"docs/output.md"},
			Next:     "1.3",
			Fallback: map[string]string{"default": "1.1", "compile_error": "2.1"},
		},
	}

	snap := NewSnapshot(nil, stubTracker{data: map[string]*memory.ArtifactInfo{}}, stubSummarizer{}, nil, newTestFS(nil))
	result := snap.BuildSnapshot(context.Background(), "1.2", step)

	if !strings.Contains(result, "目标文件: docs/output.md") {
		t.Fatal("expected output file")
	}
	if !strings.Contains(result, "下一步: 1.3") {
		t.Fatal("expected next step")
	}
	if !strings.Contains(result, "回退[default]: → 1.1") {
		t.Fatal("expected default fallback")
	}
}

func TestSnapshot_ImplementsInterface(t *testing.T) {
	var _ ContextSnapshot = NewSnapshot(nil, stubTracker{data: map[string]*memory.ArtifactInfo{}}, stubSummarizer{}, nil, newTestFS(nil))
}
