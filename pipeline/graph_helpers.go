// Deprecated: This file is superseded by pkg/flow/helpers.go.
// Kept for backward compatibility; will be removed in a future release.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

func buildInstruction(prompt *PromptFile, promptRoot string, baseVars map[string]string, systemInstruction string) (string, error) {
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

	if prompt.Frontmatter.OutputTemplate != "" {
		templatePath := prompt.Frontmatter.OutputTemplate
		if !filepath.IsAbs(templatePath) {
			templatePath = filepath.Join(promptRoot, templatePath)
		}
		content, err := os.ReadFile(templatePath)
		if err != nil {
			return "", fmt.Errorf("read template %s: %w", templatePath, err)
		}
		vars["output_template"] = RenderTemplate(string(content), vars)
	}

	body := RenderTemplate(prompt.Body, vars)
	if systemInstruction != "" {
		return systemInstruction + "\n\n" + body, nil
	}
	return body, nil
}

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
	return func(ctx context.Context, state graph.State) (string, error) {
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
			code := ClassifyToolError(err)
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

func makeConfirmNode(stepID string, mode AdvanceMode) graph.NodeFunc {
	var prompt string
	switch mode {
	case AdvanceBlock:
		prompt = fmt.Sprintf("阶段 %s 已完成，等待手动继续", stepID)
	case AdvanceConfirm:
		prompt = fmt.Sprintf("确认进入下一阶段? (%s)", stepID)
	default:
		prompt = fmt.Sprintf("阶段 %s 已完成，等待用户输入", stepID)
	}
	return func(ctx context.Context, state graph.State) (any, error) {
		if mode == AdvanceAuto {
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
