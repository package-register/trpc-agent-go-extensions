// Package mcp provides MCP (Model Context Protocol) tool integration for EDA workflows.
package mcp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/logger"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/mcp"
	tmcp "trpc.group/trpc-go/trpc-mcp-go"
)

// NewMCPToolSetsFromConfig creates MCP tool sets from configuration file.
// Optional ExportOption arguments enable automatic schema export after init.
func NewMCPToolSetsFromConfig(ctx context.Context, configPath string, exportOpts ...ExportOption) (*MCPToolSets, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	return NewMCPToolSetsFromStruct(ctx, config, exportOpts...)
}

// NewMCPToolSetsFromStruct creates MCP tool sets from configuration struct.
// Optional ExportOption arguments enable automatic schema export after init.
func NewMCPToolSetsFromStruct(ctx context.Context, config *MCPConfig, exportOpts ...ExportOption) (*MCPToolSets, error) {
	var eopts exportOptions
	for _, o := range exportOpts {
		o(&eopts)
	}
	sets := &MCPToolSets{
		toolSets: make(map[string]*mcp.ToolSet),
	}

	defaultTimeout := time.Duration(config.Defaults.Timeout) * time.Second
	if defaultTimeout == 0 {
		defaultTimeout = 30 * time.Second
	}
	defaultRetries := config.Defaults.Retries
	if defaultRetries == 0 {
		defaultRetries = 2
	}

	var initErrors []error

	for name, serverCfg := range config.MCPServers {
		if serverCfg.Disabled {
			logger.L().Info("MCP server disabled", "name", name)
			continue
		}

		timeout := defaultTimeout
		if serverCfg.Timeout > 0 {
			timeout = time.Duration(serverCfg.Timeout) * time.Second
		}

		// Determine transport type
		var transport string
		var command string
		var args []string
		var serverURL string
		var headers map[string]string

		// Determine transport based on configuration
		if serverCfg.Transport != "" {
			// Use explicitly configured transport
			transport = serverCfg.Transport
		} else if serverCfg.ServerUrl != "" {
			// Default to streamable for HTTP URLs
			transport = "streamable"
		} else {
			// Default to stdio
			transport = "stdio"
		}

		// Set transport-specific configuration
		switch transport {
		case "stdio":
			command = serverCfg.Command
			args = serverCfg.Args
		case "sse", "streamable":
			serverURL = serverCfg.ServerUrl
			headers = serverCfg.Headers
		default:
			return nil, fmt.Errorf("unsupported transport: %s, supported: stdio, sse, streamable", transport)
		}

		// Create tool set
		var opts []mcp.ToolSetOption
		opts = append(opts, mcp.WithMCPOptions(tmcp.WithSimpleRetry(defaultRetries)))

		// Add tool filter only if tools are configured
		if len(serverCfg.Tools) > 0 {
			opts = append(opts, mcp.WithToolFilterFunc(tool.NewIncludeToolNamesFilter(serverCfg.Tools...)))
		}

		toolSet := mcp.NewMCPToolSet(
			mcp.ConnectionConfig{
				Transport: transport,
				Command:   command,
				Args:      args,
				ServerURL: serverURL,
				Headers:   headers,
				Timeout:   timeout,
			},
			opts...,
		)

		// Set environment variables before init, restore after
		savedEnv := make(map[string]string, len(serverCfg.Env))
		for k, v := range serverCfg.Env {
			savedEnv[k] = os.Getenv(k)
			if err := os.Setenv(k, v); err != nil {
				logger.L().Warn("MCP failed to set env", "key", k, "error", err)
			}
		}

		if err := toolSet.Init(ctx); err != nil {
			initErrors = append(initErrors, fmt.Errorf("%s: %w", name, err))
			logger.L().Error("MCP server init failed", "name", name, "error", err)
		} else {
			sets.toolSets[name] = toolSet
			tools := toolSet.Tools(ctx)
			logger.L().Info("MCP server initialized", "name", name, "tools", len(tools))
		}

		// Restore environment variables
		for k, v := range savedEnv {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}

	if len(initErrors) > 0 {
		logger.L().Warn("MCP init errors", "failed", len(initErrors))
		for _, err := range initErrors {
			logger.L().Warn("MCP init error detail", "error", err)
		}
	}

	logger.L().Info("MCP servers ready", "active", len(sets.toolSets), "total", len(config.MCPServers))

	// Run exports if any enabled
	runExports(ctx, sets, config, eopts)

	return sets, nil
}

// GetActiveToolSets returns all successfully initialized tool sets
func (m *MCPToolSets) GetActiveToolSets() []tool.ToolSet {
	var result []tool.ToolSet
	for _, ts := range m.toolSets {
		result = append(result, ts)
	}
	return result
}

// GetToolSet returns a specific tool set by name
func (m *MCPToolSets) GetToolSet(name string) *mcp.ToolSet {
	return m.toolSets[name]
}

// Close closes all MCP tool sets
func (m *MCPToolSets) Close() {
	for name, ts := range m.toolSets {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.L().Warn("MCP panic in close", "name", name, "recover", r)
				}
			}()
			if err := ts.Close(); err != nil {
				logger.L().Warn("MCP error closing", "name", name, "error", err)
			}
		}()
		logger.L().Info("MCP server closed", "name", name)
	}
}

// Count returns the number of active tool sets
func (m *MCPToolSets) Count() int {
	return len(m.toolSets)
}
