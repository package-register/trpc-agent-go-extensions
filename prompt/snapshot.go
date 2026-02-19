package prompt

import (
	"context"
	"fmt"
	"strings"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// Snapshot implements pipeline.ContextSnapshot.
// It builds the <WorkflowContext> XML snapshot injected into LLM system messages,
// using only interfaces for its dependencies (no concrete types).
type Snapshot struct {
	steps      []*pipeline.StepDefinition
	tracker    pipeline.ArtifactTracker
	summarizer pipeline.InputSummarizer
	toolNames  func(string) []string // stepID → tool names for that step
	fs         pipeline.FileSystem
}

// NewSnapshot creates a snapshot builder.
//
//   - steps: all step definitions (needed for progress rendering)
//   - tracker: artifact tracker (interface)
//   - summarizer: input file summarizer (interface)
//   - toolNames: function that returns tool names for a given stepID
//   - fs: filesystem for checking input file/dir existence
func NewSnapshot(
	steps []*pipeline.StepDefinition,
	tracker pipeline.ArtifactTracker,
	summarizer pipeline.InputSummarizer,
	toolNames func(string) []string,
	fs pipeline.FileSystem,
) *Snapshot {
	return &Snapshot{
		steps:      steps,
		tracker:    tracker,
		summarizer: summarizer,
		toolNames:  toolNames,
		fs:         fs,
	}
}

// BuildSnapshot produces the full <WorkflowContext> XML for a given step.
func (s *Snapshot) BuildSnapshot(ctx context.Context, currentStepID string, step *pipeline.StepDefinition) string {
	var sb strings.Builder
	sb.WriteString("<WorkflowContext>\n")

	sb.WriteString(s.buildProgress(currentStepID))
	sb.WriteString(s.buildInputSummaries(ctx, step))
	sb.WriteString(s.buildAvailableTools(step))
	sb.WriteString(s.buildOutputContract(step))

	sb.WriteString("</WorkflowContext>")
	return sb.String()
}

// buildProgress renders the workflow progress section.
func (s *Snapshot) buildProgress(currentStepID string) string {
	var sb strings.Builder
	sb.WriteString("  <Progress>\n")

	allArtifacts := s.tracker.GetAll()
	totalSteps := len(s.steps)
	completedCount := 0

	for _, p := range s.steps {
		sid := p.Frontmatter.Step
		title := p.Frontmatter.Title
		output := p.Frontmatter.PrimaryOutput()

		if a, ok := allArtifacts[sid]; ok && a.Status == "completed" {
			completedCount++
			sb.WriteString(fmt.Sprintf("    ✅ %s %s → %s (已生成, %d行)\n",
				sid, title, output, a.LineCount))
		} else if sid == currentStepID {
			sb.WriteString(fmt.Sprintf("    🔄 %s %s → %s (当前任务)\n",
				sid, title, output))
		} else {
			sb.WriteString(fmt.Sprintf("    ⬚ %s %s\n", sid, title))
		}
	}

	sb.WriteString(fmt.Sprintf("    进度: 第%d步/共%d步\n", completedCount+1, totalSteps))
	sb.WriteString("  </Progress>\n")
	return sb.String()
}

// buildInputSummaries generates summaries for input files/dirs via the InputSummarizer interface.
func (s *Snapshot) buildInputSummaries(ctx context.Context, step *pipeline.StepDefinition) string {
	inputs := step.Frontmatter.Input
	if len(inputs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("  <InputSummaries>\n")

	for _, inputPath := range inputs {
		info, err := s.fs.Stat(inputPath)
		if err != nil {
			sb.WriteString(fmt.Sprintf("    <File path=%q status=\"not_found\"/>\n", inputPath))
			continue
		}

		if info.IsDir() {
			s.summarizeDir(ctx, &sb, inputPath)
		} else {
			summary, _ := s.summarizer.Summarize(ctx, inputPath)
			sb.WriteString(fmt.Sprintf("    <File path=%q>\n      %s\n    </File>\n",
				inputPath, summary))
		}
	}

	sb.WriteString("  </InputSummaries>\n")
	return sb.String()
}

// summarizeDir walks a directory and summarises each file.
func (s *Snapshot) summarizeDir(ctx context.Context, sb *strings.Builder, relDir string) {
	entries, err := s.fs.ReadDir(relDir)
	if err != nil {
		sb.WriteString(fmt.Sprintf("    <Dir path=%q status=\"read_error\"/>\n", relDir))
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		relPath := relDir + "/" + entry.Name()
		summary, _ := s.summarizer.Summarize(ctx, relPath)
		sb.WriteString(fmt.Sprintf("    <File path=%q>\n      %s\n    </File>\n",
			relPath, summary))
	}
}

// buildAvailableTools lists tools available for the current step.
func (s *Snapshot) buildAvailableTools(step *pipeline.StepDefinition) string {
	var sb strings.Builder
	sb.WriteString("  <AvailableTools>\n")

	mcpNames := step.Frontmatter.EffectiveTools()
	if len(mcpNames) == 0 {
		sb.WriteString("    当前步骤无额外工具。内置工具: file_read, file_write, file_list\n")
		sb.WriteString("  </AvailableTools>\n")
		return sb.String()
	}

	if s.toolNames != nil {
		for _, name := range mcpNames {
			tools := s.toolNames(name)
			if len(tools) == 0 {
				sb.WriteString(fmt.Sprintf("    [%s] (未加载)\n", name))
			} else {
				sb.WriteString(fmt.Sprintf("    [%s] %d个工具: %s\n",
					name, len(tools), strings.Join(tools, ", ")))
			}
		}
	}

	sb.WriteString("  </AvailableTools>\n")
	return sb.String()
}

// buildOutputContract specifies what this step must produce.
func (s *Snapshot) buildOutputContract(step *pipeline.StepDefinition) string {
	var sb strings.Builder
	sb.WriteString("  <OutputContract>\n")
	for _, out := range step.Frontmatter.Output {
		sb.WriteString(fmt.Sprintf("    目标文件: %s\n", out))
	}

	if step.Frontmatter.Next != "" {
		sb.WriteString(fmt.Sprintf("    下一步: %s\n", step.Frontmatter.Next))
	} else {
		sb.WriteString("    下一步: (流程结束)\n")
	}

	if len(step.Frontmatter.Fallback) > 0 {
		for code, target := range step.Frontmatter.Fallback {
			sb.WriteString(fmt.Sprintf("    回退[%s]: → %s\n", code, target))
		}
	}

	sb.WriteString("  </OutputContract>\n")
	return sb.String()
}
