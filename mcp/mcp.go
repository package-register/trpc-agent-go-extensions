// Package mcp provides MCP (Model Context Protocol) tool integration for EDA workflows.
package mcp

import (
	"trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

// MCPToolSets contains all initialized MCP tool sets
type MCPToolSets struct {
	toolSets map[string]*mcp.ToolSet
}
