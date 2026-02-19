// Deprecated: This file is superseded by pkg/flow/graph.go, pkg/flow/chain.go, and pkg/flow/agent.go.
// Kept for backward compatibility; will be removed in a future release.
package pipeline

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"github.com/package-register/trpc-agent-go-extensions/logger"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	// StateKeyPipelineErrorCode stores the last tool error classification.
	StateKeyPipelineErrorCode = "pipeline_error_code"
)

// BuildOptions configures graph construction from prompt files.
type BuildOptions struct {
	PromptsDir           string
	TemplatesDir         string
	Model                model.Model
	ToolSets             map[string]tool.ToolSet
	BaseVars             map[string]string
	SystemInstruction    string // Deprecated: use PromptBuilder instead
	AllowMissingToolSets bool
	MaxOutputTokens      int // fixed max output tokens per LLM call (0 = framework default)
	ArtifactTracker      *FileArtifactTracker
	PromptBuilder        *PromptBuilder      // 3-layer prompt constructor (preferred)
	EnvironmentBuilder   *EnvironmentBuilder // Deprecated: use PromptBuilder instead
	ContextCompressor    *ContextCompressor
}

// BuildGraphFromDir loads prompts and builds a graph from the directory.
func BuildGraphFromDir(opts BuildOptions) (*graph.Graph, error) {
	if opts.PromptsDir == "" {
		return nil, fmt.Errorf("prompts dir is required")
	}
	if opts.Model == nil {
		return nil, fmt.Errorf("model is required")
	}

	promptFiles, err := LoadPrompts(opts.PromptsDir)
	if err != nil {
		return nil, err
	}

	return BuildGraphFromPrompts(promptFiles, opts)
}

// LoadPrompts scans a directory and parses prompt files.
func LoadPrompts(dir string) ([]*PromptFile, error) {
	var prompts []*PromptFile

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "templates" || entry.Name() == "system" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		if strings.HasPrefix(entry.Name(), "_") {
			return nil
		}

		prompt, err := LoadPrompt(path)
		if err != nil {
			return err
		}
		prompts = append(prompts, prompt)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(prompts) == 0 {
		return nil, fmt.Errorf("no prompts found in %s", dir)
	}

	sort.Slice(prompts, func(i, j int) bool {
		return prompts[i].Frontmatter.Step < prompts[j].Frontmatter.Step
	})

	return prompts, nil
}

// BuildGraphFromPrompts constructs a StateGraph from prompt definitions.
func BuildGraphFromPrompts(prompts []*PromptFile, opts BuildOptions) (*graph.Graph, error) {
	if len(prompts) == 0 {
		return nil, fmt.Errorf("no prompts to build graph")
	}

	sg := graph.NewStateGraph(graph.MessagesStateSchema())
	promptRoot := opts.TemplatesDir
	if promptRoot == "" {
		promptRoot = filepath.Dir(opts.PromptsDir)
	}

	stepIDs := make(map[string]struct{})
	stepTools := make(map[string][]tool.ToolSet)
	for _, prompt := range prompts {
		stepID := strings.TrimSpace(prompt.Frontmatter.Step)
		if stepID == "" {
			return nil, fmt.Errorf("prompt %s missing step", prompt.Path)
		}
		if _, exists := stepIDs[stepID]; exists {
			return nil, fmt.Errorf("duplicate step %s", stepID)
		}
		stepIDs[stepID] = struct{}{}

		var instruction string
		var err error
		if opts.PromptBuilder != nil {
			instruction, err = opts.PromptBuilder.BuildStaticInstruction(prompt, promptRoot, opts.BaseVars)
		} else {
			instruction, err = buildInstruction(prompt, promptRoot, opts.BaseVars, opts.SystemInstruction)
		}
		if err != nil {
			return nil, err
		}

		nodeOpts := []graph.Option{
			graph.WithName(prompt.Frontmatter.Title),
		}
		if prompt.Frontmatter.Title != "" {
			nodeOpts = append(nodeOpts, graph.WithDescription(prompt.Frontmatter.Title))
		}
		{
			// Per-step MaxOutputTokens from frontmatter, fallback to global default
			maxTok := opts.MaxOutputTokens
			if prompt.Frontmatter.MaxOutputTokens > 0 {
				maxTok = prompt.Frontmatter.MaxOutputTokens
			}
			if maxTok > 0 {
				mt := maxTok
				nodeOpts = append(nodeOpts, graph.WithGenerationConfig(model.GenerationConfig{
					Stream:    true,
					MaxTokens: &mt,
				}))
			}
		}
		{
			compressor := opts.ContextCompressor
			promptBuilder := opts.PromptBuilder
			envBuilder := opts.EnvironmentBuilder
			cbStepID := stepID
			cbPrompt := prompt
			cbBaseVars := opts.BaseVars

			needCallback := compressor != nil || promptBuilder != nil || envBuilder != nil
			if needCallback {
				diagStepID := stepID // capture for closure
				nodeOpts = append(nodeOpts, graph.WithPreNodeCallback(
					func(ctx context.Context, cbCtx *graph.NodeCallbackContext, state graph.State) (any, error) {
						// [DIAG] Log state contents for debugging empty LLM responses
						log := logger.L()
						userInput, _ := state[graph.StateKeyUserInput].(string)
						var msgCount int
						if msgs, ok := state[graph.StateKeyMessages].([]model.Message); ok {
							msgCount = len(msgs)
						}
						log.Debug("[DIAG] PreNodeCallback",
							"step", diagStepID,
							"userInput", userInput,
							"msgCount", msgCount,
						)
						var merged graph.State
						// Step 1: context compression (Layer 3 only)
						if compressor != nil {
							result, err := compressor.MakePreNodeCallback()(ctx, cbCtx, state)
							if err != nil {
								return nil, err
							}
							if st, ok := result.(graph.State); ok {
								merged = st
								if msgs, ok := st[graph.StateKeyMessages]; ok {
									state[graph.StateKeyMessages] = msgs
								}
							}
						}
						// Step 2: rebuild Layer 1+2 system message
						if promptBuilder != nil && promptBuilder.HasDynamicContent() {
							result, err := promptBuilder.MakePreNodeCallback(cbStepID, cbPrompt, cbBaseVars)(ctx, cbCtx, state)
							if err != nil {
								return nil, err
							}
							if st, ok := result.(graph.State); ok {
								if merged == nil {
									merged = st
								} else {
									for k, v := range st {
										merged[k] = v
									}
								}
							}
						} else if envBuilder != nil {
							// Fallback: use legacy EnvironmentBuilder
							result, err := envBuilder.MakePreNodeCallback(cbStepID, cbPrompt)(ctx, cbCtx, state)
							if err != nil {
								return nil, err
							}
							if st, ok := result.(graph.State); ok {
								if merged == nil {
									merged = st
								} else {
									for k, v := range st {
										merged[k] = v
									}
								}
							}
						}
						return merged, nil
					},
				))
			}
		}
		nodeOpts = append(nodeOpts, graph.WithPostNodeCallback(clearPipelineErrorCode))

		toolSets, err := resolveToolSets(prompt.Frontmatter.EffectiveTools(), opts.ToolSets, opts.AllowMissingToolSets)
		if err != nil {
			return nil, err
		}
		stepTools[stepID] = toolSets
		if len(toolSets) > 0 {
			nodeOpts = append(nodeOpts, graph.WithToolSets(toolSets))
		}

		sg.AddLLMNode(stepID, opts.Model, instruction, nil, nodeOpts...)

		{
			confirmID := confirmNodeID(stepID)
			confirmNode := makeConfirmNode(stepID, prompt.Frontmatter.Advance)
			confirmOpts := []graph.Option{graph.WithName(confirmID)}
			if opts.ArtifactTracker != nil && len(prompt.Frontmatter.Output) > 0 {
				for _, output := range prompt.Frontmatter.Output {
					confirmOpts = append(confirmOpts, graph.WithPostNodeCallback(
						opts.ArtifactTracker.MakePostNodeCallback(
							stepID,
							prompt.Frontmatter.Title,
							output,
						),
					))
				}
			}
			sg.AddNode(confirmID, confirmNode, confirmOpts...)
		}

		if len(toolSets) > 0 {
			toolsNodeID := toolsNodeID(stepID)
			toolsNode := wrapToolsNode(graph.NewToolsNodeFunc(nil, graph.WithToolSets(toolSets)))
			sg.AddNode(toolsNodeID, toolsNode, graph.WithName(toolsNodeID), graph.WithNodeType(graph.NodeTypeTool))
		}
	}

	for _, prompt := range prompts {
		stepID := prompt.Frontmatter.Step
		nextID := nextStepID(prompt.Frontmatter.Next)
		advanceTarget := confirmNodeID(stepID)
		sg.AddEdge(advanceTarget, nextID)

		toolSets := stepTools[stepID]
		if len(toolSets) > 0 {
			toolsNodeID := toolsNodeID(stepID)
			sg.AddToolsConditionalEdges(stepID, toolsNodeID, advanceTarget)

			pathMap := map[string]string{"success": stepID}
			for code, target := range prompt.Frontmatter.Fallback {
				if target == "" {
					continue
				}
				pathMap[code] = target
			}
			cond := makeFallbackRouter(prompt.Frontmatter.Fallback)
			sg.AddConditionalEdges(toolsNodeID, cond, pathMap)
			continue
		}

		if len(prompt.Frontmatter.Fallback) > 0 {
			pathMap := map[string]string{"success": advanceTarget}
			for code, target := range prompt.Frontmatter.Fallback {
				if target == "" {
					continue
				}
				pathMap[code] = target
			}
			cond := makeFallbackRouter(prompt.Frontmatter.Fallback)
			sg.AddConditionalEdges(stepID, cond, pathMap)
		} else {
			sg.AddEdge(stepID, advanceTarget)
		}
	}

	entryID := prompts[0].Frontmatter.Step
	for _, prompt := range prompts {
		if prompt.Frontmatter.Step != "" {
			entryID = prompt.Frontmatter.Step
			break
		}
	}

	sg.SetEntryPoint(entryID)
	lastPrompt := prompts[len(prompts)-1]
	finishID := confirmNodeID(lastPrompt.Frontmatter.Step)
	sg.SetFinishPoint(finishID)

	return sg.Compile()
}
