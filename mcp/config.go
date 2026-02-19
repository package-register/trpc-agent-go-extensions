// Package mcp provides MCP (Model Context Protocol) tool integration for EDA workflows.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MCPConfig is the root configuration structure matching mcp.json
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
	Defaults   MCPDefaults                `json:"defaults"`
}

// MCPServerConfig defines a single MCP server configuration
type MCPServerConfig struct {
	// Transport specifies the transport method: "stdio", "sse", "streamable"
	Transport string `json:"transport,omitempty"`

	// STDIO configuration
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`

	// Streamable/SSE configuration
	ServerUrl string            `json:"serverUrl,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`

	// Common configuration
	Disabled bool              `json:"disabled"`
	Env      map[string]string `json:"env,omitempty"`
	Timeout  int               `json:"timeout,omitempty"`
	Tools    []string          `json:"tools,omitempty"`
}

// MCPDefaults defines default settings
type MCPDefaults struct {
	Timeout int `json:"timeout"` // seconds
	Retries int `json:"retries"`
}

// LoadConfig loads MCP configuration from a JSON file
func LoadConfig(configPath string) (*MCPConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read mcp.json: %w", err)
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse mcp.json: %w", err)
	}

	// Interpolate environment variables
	interpolateConfig(&config)

	return &config, nil
}

// interpolateConfig replaces ${env:VAR} with actual environment variable values
func interpolateConfig(config *MCPConfig) {
	for _, serverCfg := range config.MCPServers {
		// Interpolate command
		serverCfg.Command = interpolateEnv(serverCfg.Command)

		// Interpolate args
		for i, arg := range serverCfg.Args {
			serverCfg.Args[i] = interpolateEnv(arg)
		}

		// Interpolate serverUrl
		serverCfg.ServerUrl = interpolateEnv(serverCfg.ServerUrl)

		// Interpolate headers
		for k, v := range serverCfg.Headers {
			serverCfg.Headers[k] = interpolateEnv(v)
		}

		// Interpolate env values
		for k, v := range serverCfg.Env {
			serverCfg.Env[k] = interpolateEnv(v)
		}
	}
}

// interpolateEnv replaces ${env:VAR} with environment variable value
func interpolateEnv(s string) string {
	if strings.HasPrefix(s, "${env:") && strings.HasSuffix(s, "}") {
		varName := strings.TrimPrefix(s, "${env:")
		varName = strings.TrimSuffix(varName, "}")
		return os.Getenv(varName)
	}
	return s
}
