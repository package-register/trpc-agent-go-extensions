package flow

import (
	"fmt"
	"strings"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// AgentBuilder implements pipeline.FlowBuilder.
// It constructs a single-LLM-node agent that dynamically selects stages.
// Each step is registered as a tool; the LLM decides which stage to execute.
// This is suitable for exploratory, non-linear tasks.
type AgentBuilder struct{}

// NewAgentBuilder creates a new agent-based flow builder.
func NewAgentBuilder() *AgentBuilder {
	return &AgentBuilder{}
}

// Build constructs a single-agent graph where each step becomes a tool call.
func (b *AgentBuilder) Build(steps []*pipeline.StepDefinition, opts pipeline.FlowOptions) (*graph.Graph, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps to build agent")
	}

	sg := graph.NewStateGraph(graph.MessagesStateSchema())

	// Build a combined instruction from all steps
	instruction := b.buildCombinedInstruction(steps)

	var nodeOpts []graph.Option
	nodeOpts = append(nodeOpts,
		graph.WithName("agent-router"),
		graph.WithDescription("Dynamic agent that selects pipeline stages"),
	)

	if opts.MaxOutputTokens > 0 {
		mt := opts.MaxOutputTokens
		nodeOpts = append(nodeOpts, graph.WithGenerationConfig(model.GenerationConfig{
			Stream:    true,
			MaxTokens: &mt,
		}))
	}

	// Collect all tool sets from all steps
	allToolSets := make(map[string]bool)
	for _, step := range steps {
		for _, name := range step.Frontmatter.EffectiveTools() {
			allToolSets[name] = true
		}
	}
	var combinedToolSets []string
	for name := range allToolSets {
		combinedToolSets = append(combinedToolSets, name)
	}
	toolSets, err := resolveToolSets(combinedToolSets, opts.ToolSets, opts.AllowMissing)
	if err != nil {
		return nil, err
	}
	if len(toolSets) > 0 {
		nodeOpts = append(nodeOpts, graph.WithToolSets(toolSets))
	}

	// Middleware pre-node callbacks
	chain := NewMiddlewareChain(opts.Middlewares...)
	// Use the first step as the representative for middleware (agent has one node)
	preCb := chain.WrapPreNode("agent", steps[0])
	if preCb != nil {
		nodeOpts = append(nodeOpts, graph.WithPreNodeCallback(preCb))
	}

	sg.AddLLMNode("agent", opts.Model, instruction, nil, nodeOpts...)

	// Tools node
	if len(toolSets) > 0 {
		tid := "agent:tools"
		toolsNode := graph.NewToolsNodeFunc(nil, graph.WithToolSets(toolSets))
		sg.AddNode(tid, toolsNode, graph.WithName(tid), graph.WithNodeType(graph.NodeTypeTool))
		sg.AddToolsConditionalEdges("agent", tid, graph.End)
		sg.AddEdge(tid, "agent")
	} else {
		sg.AddEdge("agent", graph.End)
	}

	sg.SetEntryPoint("agent")

	return sg.Compile()
}

// buildCombinedInstruction creates a single instruction that describes all available stages.
func (b *AgentBuilder) buildCombinedInstruction(steps []*pipeline.StepDefinition) string {
	var sb fmt.Stringer = nil
	_ = sb

	var result strings.Builder
	result.WriteString("你是一个 IC 设计助手。以下是可用的设计阶段，请根据用户需求选择合适的阶段执行：\n\n")
	for _, step := range steps {
		fmt.Fprintf(&result, "## 阶段 %s: %s\n", step.Frontmatter.Step, step.Frontmatter.Title)
		if step.Frontmatter.Description != "" {
			result.WriteString(step.Frontmatter.Description + "\n")
		}
		if len(step.Frontmatter.Output) > 0 {
			fmt.Fprintf(&result, "输出: %s\n", step.Frontmatter.PrimaryOutput())
		}
		result.WriteString("\n")
	}
	return result.String()
}

// Verify interface compliance at compile time.
var _ pipeline.FlowBuilder = (*AgentBuilder)(nil)
