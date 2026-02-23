package prompt

import (
	"context"
	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// Assembler constructs the full system instruction for an LLM node.
//
//   - BuildStatic returns the build-time instruction (Layer1 + Layer2 body).
//   - BuildDynamic returns the runtime instruction with dynamic context injected.
//   - HasDynamicContent reports whether runtime rebuild is needed.
type Assembler interface {
	BuildStatic(step *pipeline.StepDefinition, vars map[string]string) (string, error)
	BuildDynamic(ctx context.Context, step *pipeline.StepDefinition, vars map[string]string) (string, error)
	HasDynamicContent() bool
}

// ContextSnapshot builds a runtime context snapshot (progress, inputs, tools, output contract)
// for injection into the system message.
type ContextSnapshot interface {
	BuildSnapshot(ctx context.Context, currentStepID string, step *pipeline.StepDefinition) string
}

// InputSummarizer generates a concise summary for an input file or directory.
type InputSummarizer interface {
	Summarize(ctx context.Context, path string) (string, error)
}
