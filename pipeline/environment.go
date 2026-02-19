// Deprecated: This file is superseded by pkg/prompt/snapshot.go and pkg/prompt/summarizer.go.
// Kept for backward compatibility; will be removed in a future release.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/logger"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// EnvironmentBuilder constructs a <WorkflowContext> XML snapshot that is
// injected into each LLM node's system message via PreNodeCallback.
type EnvironmentBuilder struct {
	llmModel  model.Model
	baseDir   string
	prompts   []*PromptFile
	artifacts *FileArtifactTracker
	toolSets  map[string]tool.ToolSet

	// summaryCache avoids re-summarising the same file.
	mu           sync.Mutex
	summaryCache map[string]string
}

// NewEnvironmentBuilder creates a builder with all the metadata it needs.
func NewEnvironmentBuilder(
	llmModel model.Model,
	baseDir string,
	prompts []*PromptFile,
	artifacts *FileArtifactTracker,
	toolSets map[string]tool.ToolSet,
) *EnvironmentBuilder {
	return &EnvironmentBuilder{
		llmModel:     llmModel,
		baseDir:      baseDir,
		prompts:      prompts,
		artifacts:    artifacts,
		toolSets:     toolSets,
		summaryCache: make(map[string]string),
	}
}

// BuildSnapshot produces the full <WorkflowContext> XML for a given step.
func (b *EnvironmentBuilder) BuildSnapshot(ctx context.Context, currentStepID string, stepPrompt *PromptFile) string {
	var sb strings.Builder
	sb.WriteString("<WorkflowContext>\n")

	sb.WriteString(b.buildProgress(currentStepID))
	sb.WriteString(b.buildInputSummaries(ctx, stepPrompt))
	sb.WriteString(b.buildAvailableTools(stepPrompt))
	sb.WriteString(b.buildOutputContract(stepPrompt))

	sb.WriteString("</WorkflowContext>")
	return sb.String()
}

// buildProgress renders the workflow progress section.
func (b *EnvironmentBuilder) buildProgress(currentStepID string) string {
	var sb strings.Builder
	sb.WriteString("  <Progress>\n")

	artifacts := b.artifacts.GetArtifacts()
	totalSteps := len(b.prompts)
	completedCount := 0

	for _, p := range b.prompts {
		sid := p.Frontmatter.Step
		title := p.Frontmatter.Title
		output := p.Frontmatter.PrimaryOutput()

		if a, ok := artifacts[sid]; ok && a.Status == "completed" {
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

// buildInputSummaries generates LLM-powered summaries for input files/dirs.
func (b *EnvironmentBuilder) buildInputSummaries(ctx context.Context, stepPrompt *PromptFile) string {
	inputs := stepPrompt.Frontmatter.Input
	if len(inputs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("  <InputSummaries>\n")

	for _, inputPath := range inputs {
		absPath := filepath.Join(b.baseDir, inputPath)
		info, err := os.Stat(absPath)
		if err != nil {
			sb.WriteString(fmt.Sprintf("    <File path=%q status=\"not_found\"/>\n", inputPath))
			continue
		}

		if info.IsDir() {
			b.summarizeDir(ctx, &sb, inputPath, absPath)
		} else {
			summary := b.summarizeFile(ctx, inputPath, absPath)
			sb.WriteString(fmt.Sprintf("    <File path=%q>\n      %s\n    </File>\n",
				inputPath, summary))
		}
	}

	sb.WriteString("  </InputSummaries>\n")
	return sb.String()
}

// summarizeDir walks a directory and summarises each file.
func (b *EnvironmentBuilder) summarizeDir(ctx context.Context, sb *strings.Builder, relDir, absDir string) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		sb.WriteString(fmt.Sprintf("    <Dir path=%q status=\"read_error\"/>\n", relDir))
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		relPath := filepath.Join(relDir, entry.Name())
		absPath := filepath.Join(absDir, entry.Name())
		summary := b.summarizeFile(ctx, relPath, absPath)
		sb.WriteString(fmt.Sprintf("    <File path=%q>\n      %s\n    </File>\n",
			relPath, summary))
	}
}

// summarizeFile returns a cached or freshly-generated LLM summary.
func (b *EnvironmentBuilder) summarizeFile(ctx context.Context, relPath, absPath string) string {
	b.mu.Lock()
	if cached, ok := b.summaryCache[relPath]; ok {
		b.mu.Unlock()
		return cached
	}
	b.mu.Unlock()

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "(读取失败)"
	}

	text := string(content)
	// Truncate very large files to ~4000 chars before summarising.
	const maxChars = 4000
	if len(text) > maxChars {
		text = text[:maxChars] + "\n...(已截断)"
	}

	summary := b.llmSummarize(ctx, relPath, text)

	b.mu.Lock()
	b.summaryCache[relPath] = summary
	b.mu.Unlock()
	return summary
}

// llmSummarize calls the model to produce a 2-3 line summary.
func (b *EnvironmentBuilder) llmSummarize(ctx context.Context, filename, content string) string {
	if b.llmModel == nil {
		return fallbackSummary(content)
	}

	prompt := fmt.Sprintf(
		"请用2-3行中文概括以下文件(%s)的核心内容，保留关键数据点和技术指标。只输出摘要，不要任何前缀。\n\n%s",
		filename, content,
	)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := &model.Request{
		Messages: []model.Message{
			model.NewUserMessage(prompt),
		},
		GenerationConfig: model.GenerationConfig{
			Stream: false,
		},
	}

	ch, err := b.llmModel.GenerateContent(ctx, req)
	if err != nil {
		logger.L().Warn("LLM summary failed", "file", filename, "error", err)
		return fallbackSummary(content)
	}

	var result string
	for resp := range ch {
		if resp.Error != nil {
			logger.L().Warn("LLM summary error", "file", filename, "error", resp.Error.Message)
			return fallbackSummary(content)
		}
		if len(resp.Choices) > 0 {
			result += resp.Choices[0].Message.Content
		}
	}

	if trimmed := strings.TrimSpace(result); trimmed != "" {
		return trimmed
	}
	return fallbackSummary(content)
}

// fallbackSummary returns the first few lines when LLM is unavailable.
func fallbackSummary(content string) string {
	lines := strings.SplitN(content, "\n", 6)
	if len(lines) > 5 {
		lines = lines[:5]
	}
	return strings.Join(lines, "\n")
}

// buildAvailableTools lists tools available for the current step.
func (b *EnvironmentBuilder) buildAvailableTools(stepPrompt *PromptFile) string {
	var sb strings.Builder
	sb.WriteString("  <AvailableTools>\n")

	mcpNames := stepPrompt.Frontmatter.EffectiveTools()
	if len(mcpNames) == 0 {
		sb.WriteString("    当前步骤无额外工具。内置工具: file_read, file_write, file_list\n")
		sb.WriteString("  </AvailableTools>\n")
		return sb.String()
	}

	for _, name := range mcpNames {
		ts, ok := b.toolSets[name]
		if !ok {
			sb.WriteString(fmt.Sprintf("    [%s] (未加载)\n", name))
			continue
		}
		tools := ts.Tools(context.Background())
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			if decl := t.Declaration(); decl != nil {
				toolNames = append(toolNames, decl.Name)
			}
		}
		sb.WriteString(fmt.Sprintf("    [%s] %d个工具: %s\n",
			name, len(toolNames), strings.Join(toolNames, ", ")))
	}

	sb.WriteString("  </AvailableTools>\n")
	return sb.String()
}

// buildOutputContract specifies what this step must produce.
func (b *EnvironmentBuilder) buildOutputContract(stepPrompt *PromptFile) string {
	var sb strings.Builder
	sb.WriteString("  <OutputContract>\n")
	for _, out := range stepPrompt.Frontmatter.Output {
		sb.WriteString(fmt.Sprintf("    目标文件: %s\n", out))
	}

	if stepPrompt.Frontmatter.Next != "" {
		sb.WriteString(fmt.Sprintf("    下一步: %s\n", stepPrompt.Frontmatter.Next))
	} else {
		sb.WriteString("    下一步: (流程结束)\n")
	}

	if len(stepPrompt.Frontmatter.Fallback) > 0 {
		for code, target := range stepPrompt.Frontmatter.Fallback {
			sb.WriteString(fmt.Sprintf("    回退[%s]: → %s\n", code, target))
		}
	}

	sb.WriteString("  </OutputContract>\n")
	return sb.String()
}

// Deprecated: MakePreNodeCallback is replaced by PromptBuilder.MakePreNodeCallback.
// Kept for backward compatibility when PromptBuilder is not configured.
func (b *EnvironmentBuilder) MakePreNodeCallback(stepID string, stepPrompt *PromptFile) graph.BeforeNodeCallback {
	return func(ctx context.Context, _ *graph.NodeCallbackContext, state graph.State) (any, error) {
		snapshot := b.BuildSnapshot(ctx, stepID, stepPrompt)

		if msgs, ok := state[graph.StateKeyMessages].([]model.Message); ok && len(msgs) > 0 {
			if msgs[0].Role == model.RoleSystem {
				enhanced := msgs[0].Content + "\n\n" + snapshot
				newMsgs := make([]model.Message, len(msgs))
				copy(newMsgs, msgs)
				newMsgs[0] = model.NewSystemMessage(enhanced)
				return graph.State{
					graph.StateKeyMessages: newMsgs,
				}, nil
			}
		}

		return nil, nil
	}
}
