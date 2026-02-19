package flow

import (
	"fmt"
	"strings"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// ChainBuilder implements pipeline.FlowBuilder.
// It constructs a linear chain: step1 → confirm1 → step2 → confirm2 → ... → END.
// All fallback and next fields are ignored; steps execute in sorted order.
type ChainBuilder struct{}

// NewChainBuilder creates a new chain-based flow builder.
func NewChainBuilder() *ChainBuilder {
	return &ChainBuilder{}
}

// Build constructs a linear graph from step definitions.
func (b *ChainBuilder) Build(steps []*pipeline.StepDefinition, opts pipeline.FlowOptions) (*graph.Graph, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps to build chain")
	}

	sg := graph.NewStateGraph(graph.MessagesStateSchema())

	// Phase 1: Create all nodes
	for _, step := range steps {
		stepID := strings.TrimSpace(step.Frontmatter.Step)
		if stepID == "" {
			return nil, fmt.Errorf("step %s missing step ID", step.Path)
		}

		instruction := step.Body

		var nodeOpts []graph.Option
		if step.Frontmatter.Title != "" {
			nodeOpts = append(nodeOpts,
				graph.WithName(step.Frontmatter.Title),
				graph.WithDescription(step.Frontmatter.Title),
			)
		}

		maxTok := opts.MaxOutputTokens
		if step.Frontmatter.MaxOutputTokens > 0 {
			maxTok = step.Frontmatter.MaxOutputTokens
		}
		if maxTok > 0 {
			mt := maxTok
			nodeOpts = append(nodeOpts, graph.WithGenerationConfig(model.GenerationConfig{
				Stream:    true,
				MaxTokens: &mt,
			}))
		}

		// Middleware pre-node callbacks
		chain := NewMiddlewareChain(opts.Middlewares...)
		preCb := chain.WrapPreNode(stepID, step)
		if preCb != nil {
			nodeOpts = append(nodeOpts, graph.WithPreNodeCallback(preCb))
		}

		toolSets, err := resolveToolSets(step.Frontmatter.EffectiveTools(), opts.ToolSets, opts.AllowMissing)
		if err != nil {
			return nil, err
		}
		if len(toolSets) > 0 {
			nodeOpts = append(nodeOpts, graph.WithToolSets(toolSets))
		}

		sg.AddLLMNode(stepID, opts.Model, instruction, nil, nodeOpts...)

		// Confirm node
		cid := confirmNodeID(stepID)
		confirmNode := makeConfirmNode(stepID, step.Frontmatter.Advance)
		confirmOpts := []graph.Option{graph.WithName(cid)}

		postCb := chain.WrapPostNode(stepID, step)
		if postCb != nil {
			confirmOpts = append(confirmOpts, graph.WithPostNodeCallback(postCb))
		}

		sg.AddNode(cid, confirmNode, confirmOpts...)

		// Tools node (if needed)
		if len(toolSets) > 0 {
			tid := toolsNodeID(stepID)
			toolsNode := wrapToolsNode(graph.NewToolsNodeFunc(nil, graph.WithToolSets(toolSets)))
			sg.AddNode(tid, toolsNode, graph.WithName(tid), graph.WithNodeType(graph.NodeTypeTool))
		}
	}

	// Phase 2: Linear edges — ignore next/fallback, connect sequentially
	for i, step := range steps {
		stepID := step.Frontmatter.Step
		cid := confirmNodeID(stepID)

		toolSets, _ := resolveToolSets(step.Frontmatter.EffectiveTools(), opts.ToolSets, opts.AllowMissing)
		if len(toolSets) > 0 {
			tid := toolsNodeID(stepID)
			sg.AddToolsConditionalEdges(stepID, tid, cid)
			// In chain mode, tools errors just retry the same step
			sg.AddConditionalEdges(tid, makeFallbackRouter(nil), map[string]string{"success": stepID})
		} else {
			sg.AddEdge(stepID, cid)
		}

		// Connect confirm → next step (or END)
		if i < len(steps)-1 {
			nextStepID := steps[i+1].Frontmatter.Step
			sg.AddEdge(cid, nextStepID)
		}
	}

	// Phase 3: Entry and finish
	sg.SetEntryPoint(steps[0].Frontmatter.Step)
	sg.SetFinishPoint(confirmNodeID(steps[len(steps)-1].Frontmatter.Step))

	return sg.Compile()
}

// Verify interface compliance at compile time.
var _ pipeline.FlowBuilder = (*ChainBuilder)(nil)
