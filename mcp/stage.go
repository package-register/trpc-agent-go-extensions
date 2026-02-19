// Package mcp provides MCP (Model Context Protocol) tool integration for EDA workflows.
package mcp

import "trpc.group/trpc-go/trpc-agent-go/tool"

// GetToolSetsByStage returns tool sets for a specific IC design stage
func (m *MCPToolSets) GetToolSetsByStage(stage string) []tool.ToolSet {
	stageTools := map[string][]string{
		"planning":   {},
		"rtl":        {},
		"simulation": {"eda"},
		"waveform":   {"eda"},
		"formal":     {},
		"synthesis":  {"eda"},
		"physical":   {"eda"},
		"layout":     {"eda"},
		"report":     {"eda"},
	}

	var result []tool.ToolSet
	if names, ok := stageTools[stage]; ok {
		for _, name := range names {
			if ts := m.toolSets[name]; ts != nil {
				result = append(result, ts)
			}
		}
	}
	return result
}
