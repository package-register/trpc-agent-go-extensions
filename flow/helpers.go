package flow

import (
	"context"
	"fmt"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	// StateKeyPipelineErrorCode stores the last tool error classification.
	StateKeyPipelineErrorCode = "pipeline_error_code"
)

func resolveToolSets(names []string, available map[string]tool.ToolSet, allowMissing bool) ([]tool.ToolSet, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var result []tool.ToolSet
	for _, name := range names {
		toolSet := available[name]
		if toolSet == nil {
			if allowMissing {
				continue
			}
			return nil, fmt.Errorf("toolset not found: %s", name)
		}
		result = append(result, toolSet)
	}
	return result, nil
}

func makeFallbackRouter(fallback map[string]string) graph.ConditionalFunc {
	return func(_ context.Context, state graph.State) (string, error) {
		if code, ok := state[StateKeyPipelineErrorCode].(string); ok && code != "" {
			if _, exists := fallback[code]; exists {
				return code, nil
			}
			if _, exists := fallback["default"]; exists {
				return "default", nil
			}
			return "success", nil
		}
		return "success", nil
	}
}

func nextStepID(next string) string {
	if next == "" {
		return graph.End
	}
	return next
}

func confirmNodeID(stepID string) string {
	return stepID + ":confirm"
}

func toolsNodeID(stepID string) string {
	return stepID + ":tools"
}

func wrapToolsNode(base graph.NodeFunc) graph.NodeFunc {
	return func(ctx context.Context, state graph.State) (any, error) {
		result, err := base(ctx, state)
		if err != nil {
			code := pipeline.ClassifyToolError(err)
			return graph.State{StateKeyPipelineErrorCode: string(code)}, nil
		}
		if st, ok := result.(graph.State); ok {
			st[StateKeyPipelineErrorCode] = ""
			return st, nil
		}
		return result, nil
	}
}

func clearPipelineErrorCode(_ context.Context, _ *graph.NodeCallbackContext, _ graph.State, result any, nodeErr error) (any, error) {
	if nodeErr != nil {
		return nil, nodeErr
	}
	if st, ok := result.(graph.State); ok {
		st[StateKeyPipelineErrorCode] = ""
		return st, nil
	}
	return nil, nil
}

func makeConfirmNode(stepID string, mode pipeline.AdvanceMode) graph.NodeFunc {
	var prompt string
	switch mode {
	case pipeline.AdvanceBlock:
		prompt = fmt.Sprintf("阶段 %s 已完成，等待手动继续", stepID)
	case pipeline.AdvanceConfirm:
		prompt = fmt.Sprintf("确认进入下一阶段? (%s)", stepID)
	default:
		prompt = fmt.Sprintf("阶段 %s 已完成，等待用户输入", stepID)
	}
	return func(ctx context.Context, state graph.State) (any, error) {
		if mode == pipeline.AdvanceAuto {
			return graph.State{StateKeyPipelineErrorCode: ""}, nil
		}
		_, err := graph.Interrupt(ctx, state, stepID, map[string]any{
			"message": prompt,
			"stage":   stepID,
			"advance": string(mode),
		})
		return nil, err
	}
}
