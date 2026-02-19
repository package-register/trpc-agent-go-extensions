// Deprecated: This file is superseded by pkg/prompt/assembler.go, pkg/prompt/snapshot.go,
// and pkg/prompt/markers.go. Kept for backward compatibility; will be removed in a future release.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// PromptBuilder constructs the 3-layer prompt for each LLM node.
//
// Layer 1: <system_core_prompt> — role, principles, base tools (never compressed)
// Layer 2: <pkg_inject_prompt> — step context, progress, tools, output contract (never compressed)
// Layer 3: conversation history — user/assistant messages (compression target)
type PromptBuilder struct {
	corePrompt     string // Layer 1: loaded from core.md
	toolsReference string // Layer 1 appendix: loaded from tools_reference.md
	envBuilder     *EnvironmentBuilder
}

// NewPromptBuilder creates a builder with Layer 1 content loaded from disk.
func NewPromptBuilder(corePromptPath, toolsRefPath string, envBuilder *EnvironmentBuilder) *PromptBuilder {
	pb := &PromptBuilder{
		envBuilder: envBuilder,
	}
	if data, err := os.ReadFile(corePromptPath); err == nil {
		pb.corePrompt = string(data)
	}
	if data, err := os.ReadFile(toolsRefPath); err == nil {
		pb.toolsReference = string(data)
	}
	return pb
}

// BuildStaticInstruction constructs the initial system instruction (Layer 1 + static Layer 2 body).
// This is used at graph build time as the LLM node's instruction.
// The dynamic parts of Layer 2 (progress, input summaries) are injected at runtime via PreNodeCallback.
func (b *PromptBuilder) BuildStaticInstruction(prompt *PromptFile, promptRoot string, baseVars map[string]string) (string, error) {
	vars := map[string]string{
		"output_path": prompt.Frontmatter.PrimaryOutput(),
		"stage":       prompt.Frontmatter.Step,
	}
	for k, v := range baseVars {
		vars[k] = v
	}
	if vars["base_dir"] == "" {
		vars["base_dir"] = promptRoot
	}

	body := RenderTemplate(prompt.Body, vars)

	var sb strings.Builder

	// Layer 1: system core prompt
	sb.WriteString("<system_core_prompt>\n")
	sb.WriteString(b.corePrompt)
	if b.toolsReference != "" {
		sb.WriteString("\n\n<tools_reference>\n")
		sb.WriteString(b.toolsReference)
		sb.WriteString("\n</tools_reference>")
	}
	sb.WriteString("\n</system_core_prompt>\n\n")

	// Layer 2: pkg inject prompt (static part — body only; dynamic parts injected at runtime)
	sb.WriteString("<pkg_inject_prompt>\n")
	sb.WriteString("<pkg_prompt>\n")
	sb.WriteString(body)
	sb.WriteString("\n</pkg_prompt>\n")
	sb.WriteString("</pkg_inject_prompt>")

	return sb.String(), nil
}

// MakePreNodeCallback returns a BeforeNodeCallback that rebuilds the full
// Layer 1 + Layer 2 system message at runtime, injecting dynamic context
// (progress, input summaries, available tools, output contract).
func (b *PromptBuilder) MakePreNodeCallback(stepID string, stepPrompt *PromptFile, baseVars map[string]string) graph.BeforeNodeCallback {
	return func(ctx context.Context, _ *graph.NodeCallbackContext, state graph.State) (any, error) {
		msgs, ok := state[graph.StateKeyMessages].([]model.Message)
		if !ok || len(msgs) == 0 {
			return nil, nil
		}

		// Find the existing system message (should be msgs[0])
		if msgs[0].Role != model.RoleSystem {
			return nil, nil
		}

		// Rebuild the full system message with dynamic Layer 2 content
		fullInstruction := b.buildFullInstruction(ctx, stepID, stepPrompt, baseVars)

		newMsgs := make([]model.Message, len(msgs))
		copy(newMsgs, msgs)
		newMsgs[0] = model.NewSystemMessage(fullInstruction)

		return graph.State{
			graph.StateKeyMessages: newMsgs,
		}, nil
	}
}

// buildFullInstruction constructs the complete system message with both static and dynamic content.
func (b *PromptBuilder) buildFullInstruction(ctx context.Context, stepID string, stepPrompt *PromptFile, baseVars map[string]string) string {
	vars := map[string]string{
		"output_path": stepPrompt.Frontmatter.PrimaryOutput(),
		"stage":       stepPrompt.Frontmatter.Step,
	}
	for k, v := range baseVars {
		vars[k] = v
	}

	body := RenderTemplate(stepPrompt.Body, vars)

	var sb strings.Builder

	// Layer 1: system core prompt (static)
	sb.WriteString("<system_core_prompt>\n")
	sb.WriteString(b.corePrompt)
	if b.toolsReference != "" {
		sb.WriteString("\n\n<tools_reference>\n")
		sb.WriteString(b.toolsReference)
		sb.WriteString("\n</tools_reference>")
	}
	sb.WriteString("\n</system_core_prompt>\n\n")

	// Layer 2: pkg inject prompt (dynamic + body)
	sb.WriteString("<pkg_inject_prompt>\n")

	// Dynamic context from EnvironmentBuilder
	if b.envBuilder != nil {
		snapshot := b.envBuilder.BuildSnapshot(ctx, stepID, stepPrompt)
		sb.WriteString(snapshot)
		sb.WriteString("\n")
	}

	// Static body from prompt file
	sb.WriteString("<pkg_prompt>\n")
	sb.WriteString(body)
	sb.WriteString("\n</pkg_prompt>\n")
	sb.WriteString("</pkg_inject_prompt>")

	return sb.String()
}

// HasDynamicContent returns true if the builder has an EnvironmentBuilder
// that provides runtime-dynamic content (progress, input summaries).
func (b *PromptBuilder) HasDynamicContent() bool {
	return b.envBuilder != nil
}

// FormatLayerMarker returns the XML tag used to identify Layer 1+2 content
// in system messages. Used by ContextCompressor to identify protected content.
func FormatLayerMarker() string {
	return "<system_core_prompt>"
}

// IsProtectedSystemMessage checks if a system message contains Layer 1+2 content
// that should never be compressed.
func IsProtectedSystemMessage(content string) bool {
	return strings.Contains(content, "<system_core_prompt>") ||
		strings.Contains(content, "<pkg_inject_prompt>")
}

// IsSummaryMessage checks if a system message is a compression summary.
func IsSummaryMessage(content string) bool {
	return strings.HasPrefix(content, "[上下文摘要")
}

// SummaryPrefix is the prefix used for compression summary messages.
const SummaryPrefix = "[上下文摘要 — 以下是之前对话的压缩总结]\n"

// FormatSummaryMessage wraps a summary string into a system message.
func FormatSummaryMessage(summary string) model.Message {
	return model.Message{
		Role:    model.RoleSystem,
		Content: fmt.Sprintf("%s%s", SummaryPrefix, summary),
	}
}
