package pipeline

import (
	"context"
	"io/fs"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// ──────────────────── Flow 层 ────────────────────

// FlowBuilder constructs an executable graph from step definitions.
// Implementations provide different execution topologies (chain, graph, agent).
type FlowBuilder interface {
	Build(steps []*StepDefinition, opts FlowOptions) (*graph.Graph, error)
}

// FlowOptions configures graph construction. This replaces the old BuildOptions.
type FlowOptions struct {
	Model           model.Model
	ToolSets        map[string]tool.ToolSet
	AllowMissing    bool
	MaxOutputTokens int
	Middlewares     []Middleware
	Assembler       PromptAssembler   // optional; builds LLM system instructions
	BaseVars        map[string]string // template variables passed to Assembler
}

// Middleware wraps LLM node callbacks for cross-cutting concerns
// (compression, prompt injection, artifact recording, etc.).
type Middleware interface {
	WrapPreNode(stepID string, step *StepDefinition) graph.BeforeNodeCallback
	WrapPostNode(stepID string, step *StepDefinition) graph.AfterNodeCallback
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

// PromptAssembler constructs the full system instruction for an LLM node.
type PromptAssembler interface {
	BuildStatic(step *StepDefinition, vars map[string]string) (string, error)
	BuildDynamic(ctx context.Context, step *StepDefinition, vars map[string]string) (string, error)
	HasDynamicContent() bool
}
