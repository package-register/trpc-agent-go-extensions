package pipeline

import (
	"context"
	"io/fs"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// ──────────────────── Step 层 ────────────────────

// StepLoader loads step definitions from any source (filesystem, memory, etc.).
type StepLoader interface {
	Load() ([]*PromptFile, error)
}

// ──────────────────── Prompt 层 ────────────────────

// PromptAssembler constructs the full system instruction for an LLM node.
//
//   - BuildStatic returns the build-time instruction (Layer1 + Layer2 body).
//   - BuildDynamic returns the runtime instruction with dynamic context injected.
//   - HasDynamicContent reports whether runtime rebuild is needed.
type PromptAssembler interface {
	BuildStatic(step *PromptFile, vars map[string]string) (string, error)
	BuildDynamic(ctx context.Context, step *PromptFile, vars map[string]string) (string, error)
	HasDynamicContent() bool
}

// ContextSnapshot builds a runtime context snapshot (progress, inputs, tools, output contract)
// for injection into the system message.
type ContextSnapshot interface {
	BuildSnapshot(ctx context.Context, currentStepID string, step *PromptFile) string
}

// InputSummarizer generates a concise summary for an input file or directory.
type InputSummarizer interface {
	Summarize(ctx context.Context, path string) (string, error)
}

// ──────────────────── Memory 层 ────────────────────

// Compressor compresses message history when token usage approaches the context window limit.
type Compressor interface {
	CompressIfNeeded(ctx context.Context, msgs []model.Message, currentTokens int) (compressed []model.Message, didCompress bool, err error)
}

// ArtifactTracker tracks produced documents across pipeline steps.
type ArtifactTracker interface {
	RecordCompleted(stepID, title, outputPath string) bool
	GetArtifact(stepID string) *ArtifactInfo
	GetAll() map[string]*ArtifactInfo
}

// ──────────────────── Token 层 ────────────────────

// TokenCounter estimates the total token count for a list of messages.
type TokenCounter interface {
	Count(ctx context.Context, msgs []model.Message) int
}

// TokenObserver is notified when token-related events occur (e.g. compression).
type TokenObserver interface {
	OnCompression(beforeTokens, afterTokens int)
}

// ──────────────────── Flow 层 ────────────────────

// FlowBuilder constructs an executable graph from step definitions.
// Implementations provide different execution topologies (chain, graph, agent).
type FlowBuilder interface {
	Build(steps []*PromptFile, opts FlowOptions) (*graph.Graph, error)
}

// FlowOptions configures graph construction. This replaces the old BuildOptions.
type FlowOptions struct {
	Model           model.Model
	ToolSets        map[string]tool.ToolSet
	AllowMissing    bool
	MaxOutputTokens int
	Middlewares     []Middleware
	Assembler       PromptAssembler   // optional: builds Layer 1+2 instructions per step
	BaseVars        map[string]string // template variables passed to Assembler
}

// Middleware wraps LLM node callbacks for cross-cutting concerns
// (compression, prompt injection, artifact recording, etc.).
type Middleware interface {
	WrapPreNode(stepID string, step *PromptFile) graph.BeforeNodeCallback
	WrapPostNode(stepID string, step *PromptFile) graph.AfterNodeCallback
}

// ──────────────────── 文件系统抽象 ────────────────────

// FileSystem abstracts file access so that implementations can be swapped
// for testing (e.g. fstest.MapFS) without touching os.* directly.
type FileSystem interface {
	fs.ReadFileFS
	// Stat returns file info. Mirrors os.Stat semantics.
	Stat(name string) (fs.FileInfo, error)
	// ReadDir returns directory entries. Mirrors os.ReadDir semantics.
	ReadDir(name string) ([]fs.DirEntry, error)
}
