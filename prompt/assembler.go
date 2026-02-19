package prompt

import (
	"context"
	"strings"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// Assembler implements pipeline.PromptAssembler.
// It constructs the 3-layer prompt for each LLM node:
//
//   - Layer 1: <system_core_prompt> — role, principles, base tools (never compressed)
//   - Layer 2: <pkg_inject_prompt> — step context, progress, tools, output contract (never compressed)
//   - Layer 3: conversation history — user/assistant messages (compression target)
type Assembler struct {
	corePrompt     string
	toolsReference string
	snapshot       pipeline.ContextSnapshot // nil = no dynamic content
}

// NewAssembler creates a builder with Layer 1 content loaded from the filesystem.
func NewAssembler(
	corePromptPath string,
	toolsRefPath string,
	fs pipeline.FileSystem,
	snapshot pipeline.ContextSnapshot,
) *Assembler {
	a := &Assembler{
		snapshot: snapshot,
	}
	if data, err := fs.ReadFile(corePromptPath); err == nil {
		a.corePrompt = string(data)
	}
	if data, err := fs.ReadFile(toolsRefPath); err == nil {
		a.toolsReference = string(data)
	}
	return a
}

// BuildStatic constructs the initial system instruction (Layer 1 + static Layer 2 body).
// Used at graph build time as the LLM node's instruction.
func (a *Assembler) BuildStatic(step *pipeline.StepDefinition, vars map[string]string) (string, error) {
	mergedVars := a.mergeVars(step, vars)
	body := pipeline.RenderTemplate(step.Body, mergedVars)

	var sb strings.Builder
	a.writeLayer1(&sb)
	a.writeStaticLayer2(&sb, body)
	return sb.String(), nil
}

// BuildDynamic constructs the complete system message with both static and dynamic content.
// Used at runtime via PreNodeCallback to inject progress, input summaries, etc.
func (a *Assembler) BuildDynamic(ctx context.Context, step *pipeline.StepDefinition, vars map[string]string) (string, error) {
	mergedVars := a.mergeVars(step, vars)
	body := pipeline.RenderTemplate(step.Body, mergedVars)

	var sb strings.Builder
	a.writeLayer1(&sb)

	sb.WriteString("<pkg_inject_prompt>\n")
	if a.snapshot != nil {
		snapshot := a.snapshot.BuildSnapshot(ctx, step.Frontmatter.Step, step)
		sb.WriteString(snapshot)
		sb.WriteString("\n")
	}
	sb.WriteString("<pkg_prompt>\n")
	sb.WriteString(body)
	sb.WriteString("\n</pkg_prompt>\n")
	sb.WriteString("</pkg_inject_prompt>")

	return sb.String(), nil
}

// HasDynamicContent reports whether runtime rebuild is needed.
func (a *Assembler) HasDynamicContent() bool {
	return a.snapshot != nil
}

// mergeVars creates the template variable map with step defaults + caller overrides.
func (a *Assembler) mergeVars(step *pipeline.StepDefinition, vars map[string]string) map[string]string {
	merged := map[string]string{
		"output_path": step.Frontmatter.PrimaryOutput(),
		"stage":       step.Frontmatter.Step,
	}
	for k, v := range vars {
		merged[k] = v
	}
	return merged
}

// writeLayer1 writes the <system_core_prompt> section.
func (a *Assembler) writeLayer1(sb *strings.Builder) {
	sb.WriteString("<system_core_prompt>\n")
	sb.WriteString(a.corePrompt)
	if a.toolsReference != "" {
		sb.WriteString("\n\n<tools_reference>\n")
		sb.WriteString(a.toolsReference)
		sb.WriteString("\n</tools_reference>")
	}
	sb.WriteString("\n</system_core_prompt>\n\n")
}

// writeStaticLayer2 writes the static <pkg_inject_prompt> section (body only, no dynamic context).
func (a *Assembler) writeStaticLayer2(sb *strings.Builder, body string) {
	sb.WriteString("<pkg_inject_prompt>\n")
	sb.WriteString("<pkg_prompt>\n")
	sb.WriteString(body)
	sb.WriteString("\n</pkg_prompt>\n")
	sb.WriteString("</pkg_inject_prompt>")
}
