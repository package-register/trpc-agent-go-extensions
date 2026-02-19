package flow

import (
	"fmt"
	"strings"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// GraphBuilder implements pipeline.FlowBuilder.
// It constructs a state-machine graph with next/fallback conditional routing,
// matching the behavior of the original BuildGraphFromPrompts.
type GraphBuilder struct{}

// NewGraphBuilder creates a new graph-based flow builder.
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{}
}

// Build constructs a StateGraph from step definitions with full routing support.
func (b *GraphBuilder) Build(steps []*pipeline.StepDefinition, opts pipeline.FlowOptions) (*graph.Graph, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps to build graph")
	}

	sg := graph.NewStateGraph(graph.MessagesStateSchema())

	stepIDs := make(map[string]struct{})
	stepTools := make(map[string][]tool.ToolSet)

	// Phase 1: Create all nodes
	for _, step := range steps {
		stepID := strings.TrimSpace(step.Frontmatter.Step)
		if stepID == "" {
			return nil, fmt.Errorf("step %s missing step ID", step.Path)
		}
		if _, exists := stepIDs[stepID]; exists {
			return nil, fmt.Errorf("duplicate step %s", stepID)
		}
		stepIDs[stepID] = struct{}{}

		instruction, nodeOpts, err := b.buildLLMNodeOptions(step, stepID, opts)
		if err != nil {
			return nil, err
		}

		toolSets, err := resolveToolSets(step.Frontmatter.EffectiveTools(), opts.ToolSets, opts.AllowMissing)
		if err != nil {
			return nil, err
		}
		stepTools[stepID] = toolSets
		if len(toolSets) > 0 {
			nodeOpts = append(nodeOpts, graph.WithToolSets(toolSets))
		}

		sg.AddLLMNode(stepID, opts.Model, instruction, nil, nodeOpts...)

		b.addConfirmNode(sg, step, stepID, opts)

		if len(toolSets) > 0 {
			tid := toolsNodeID(stepID)
			toolsNode := wrapToolsNode(graph.NewToolsNodeFunc(nil, graph.WithToolSets(toolSets)))
			sg.AddNode(tid, toolsNode, graph.WithName(tid), graph.WithNodeType(graph.NodeTypeTool))
		}
	}

	// Phase 2: Connect all edges
	for _, step := range steps {
		stepID := step.Frontmatter.Step
		b.addEdges(sg, step, stepID, stepTools[stepID])
	}

	// Phase 3: Set entry and finish points
	b.setEntryAndFinish(sg, steps)

	return sg.Compile()
}

// buildLLMNodeOptions constructs the instruction and graph options for an LLM node.
func (b *GraphBuilder) buildLLMNodeOptions(step *pipeline.StepDefinition, stepID string, opts pipeline.FlowOptions) (string, []graph.Option, error) {
	var instruction string
	if opts.Assembler != nil {
		built, err := opts.Assembler.BuildStatic(step, opts.BaseVars)
		if err != nil {
			return "", nil, fmt.Errorf("build instruction for %s: %w", stepID, err)
		}
		instruction = built
	} else {
		instruction = step.Body
	}

	var nodeOpts []graph.Option
	if step.Frontmatter.Title != "" {
		nodeOpts = append(nodeOpts,
			graph.WithName(step.Frontmatter.Title),
			graph.WithDescription(step.Frontmatter.Title),
		)
	}

	// Per-step MaxOutputTokens from frontmatter, fallback to global default
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

	nodeOpts = append(nodeOpts, graph.WithPostNodeCallback(clearPipelineErrorCode))

	return instruction, nodeOpts, nil
}

// addConfirmNode adds a confirm node for the step.
func (b *GraphBuilder) addConfirmNode(sg *graph.StateGraph, step *pipeline.StepDefinition, stepID string, opts pipeline.FlowOptions) {
	cid := confirmNodeID(stepID)
	confirmNode := makeConfirmNode(stepID, step.Frontmatter.Advance)
	confirmOpts := []graph.Option{graph.WithName(cid)}

	// Middleware post-node callbacks for artifact recording
	chain := NewMiddlewareChain(opts.Middlewares...)
	postCb := chain.WrapPostNode(stepID, step)
	if postCb != nil {
		confirmOpts = append(confirmOpts, graph.WithPostNodeCallback(postCb))
	}

	sg.AddNode(cid, confirmNode, confirmOpts...)
}

// addEdges connects a step to its next/fallback targets.
func (b *GraphBuilder) addEdges(sg *graph.StateGraph, step *pipeline.StepDefinition, stepID string, toolSets []tool.ToolSet) {
	nextID := nextStepID(step.Frontmatter.Next)
	advanceTarget := confirmNodeID(stepID)
	sg.AddEdge(advanceTarget, nextID)

	if len(toolSets) > 0 {
		tid := toolsNodeID(stepID)
		sg.AddToolsConditionalEdges(stepID, tid, advanceTarget)

		pathMap := map[string]string{"success": stepID}
		for code, target := range step.Frontmatter.Fallback {
			if target == "" {
				continue
			}
			pathMap[code] = target
		}
		cond := makeFallbackRouter(step.Frontmatter.Fallback)
		sg.AddConditionalEdges(tid, cond, pathMap)
		return
	}

	if len(step.Frontmatter.Fallback) > 0 {
		pathMap := map[string]string{"success": advanceTarget}
		for code, target := range step.Frontmatter.Fallback {
			if target == "" {
				continue
			}
			pathMap[code] = target
		}
		cond := makeFallbackRouter(step.Frontmatter.Fallback)
		sg.AddConditionalEdges(stepID, cond, pathMap)
	} else {
		sg.AddEdge(stepID, advanceTarget)
	}
}

// setEntryAndFinish sets the graph entry and finish points.
func (b *GraphBuilder) setEntryAndFinish(sg *graph.StateGraph, steps []*pipeline.StepDefinition) {
	entryID := steps[0].Frontmatter.Step
	for _, s := range steps {
		if s.Frontmatter.Step != "" {
			entryID = s.Frontmatter.Step
			break
		}
	}

	sg.SetEntryPoint(entryID)
	lastStep := steps[len(steps)-1]
	finishID := confirmNodeID(lastStep.Frontmatter.Step)
	sg.SetFinishPoint(finishID)
}

// Verify interface compliance at compile time.
var _ pipeline.FlowBuilder = (*GraphBuilder)(nil)
