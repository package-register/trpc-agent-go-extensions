package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/logger"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// ── Export option types ──

// ExportOption configures optional schema export behavior.
type ExportOption func(*exportOptions)

type exportOptions struct {
	toolSchemaPath     string // A: raw tool declarations
	openAISchemaPath   string // C: OpenAI function-calling format
	configTemplatePath string // B: mcp.json template with discovered tools
}

// WithExportToolSchema enables exporting raw tool schema JSON after init.
func WithExportToolSchema(path string) ExportOption {
	return func(o *exportOptions) { o.toolSchemaPath = path }
}

// WithExportOpenAISchema enables exporting OpenAI function-calling JSON after init.
func WithExportOpenAISchema(path string) ExportOption {
	return func(o *exportOptions) { o.openAISchemaPath = path }
}

// WithExportConfigTemplate enables generating an mcp.json template with discovered tool names.
func WithExportConfigTemplate(path string) ExportOption {
	return func(o *exportOptions) { o.configTemplatePath = path }
}

// ── Export data structures ──

// ToolSchemaExport is the top-level structure for raw tool schema export (A).
type ToolSchemaExport struct {
	ExportedAt string                       `json:"exported_at"`
	Servers    map[string]ServerToolsExport `json:"servers"`
}

// ServerToolsExport describes one MCP server and its tools.
type ServerToolsExport struct {
	URL       string             `json:"url,omitempty"`
	Transport string             `json:"transport,omitempty"`
	Tools     []tool.Declaration `json:"tools"`
}

// OpenAISchemaExport is the top-level structure for OpenAI function schema export (C).
type OpenAISchemaExport struct {
	ExportedAt string           `json:"exported_at"`
	Tools      []OpenAIFunction `json:"tools"`
}

// OpenAIFunction represents one tool in OpenAI function-calling format.
type OpenAIFunction struct {
	Type     string             `json:"type"`
	Function OpenAIFunctionDecl `json:"function"`
}

// OpenAIFunctionDecl is the function declaration inside OpenAIFunction.
type OpenAIFunctionDecl struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Parameters  *tool.Schema `json:"parameters,omitempty"`
}

// ConfigTemplateExport is the structure for mcp.json template export (B).
type ConfigTemplateExport struct {
	MCPServers map[string]ConfigTemplateServer `json:"mcpServers"`
	Defaults   MCPDefaults                     `json:"defaults"`
}

// ConfigTemplateServer is a single server entry in the config template.
type ConfigTemplateServer struct {
	Transport string   `json:"transport,omitempty"`
	ServerURL string   `json:"serverUrl,omitempty"`
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	Disabled  bool     `json:"disabled"`
	Timeout   int      `json:"timeout,omitempty"`
	Tools     []string `json:"tools"`
}

// ── Export functions ──

// ExportToolSchema exports all discovered tool declarations to a JSON file (A).
func ExportToolSchema(ctx context.Context, sets *MCPToolSets, config *MCPConfig, path string) error {
	export := ToolSchemaExport{
		ExportedAt: time.Now().Format(time.RFC3339),
		Servers:    make(map[string]ServerToolsExport),
	}

	for name, ts := range sets.toolSets {
		serverCfg := config.MCPServers[name]
		serverExport := ServerToolsExport{
			URL:       serverCfg.ServerUrl,
			Transport: serverCfg.Transport,
		}

		tools := ts.Tools(ctx)
		for _, t := range tools {
			if decl := t.Declaration(); decl != nil {
				serverExport.Tools = append(serverExport.Tools, *decl)
			}
		}
		export.Servers[name] = serverExport
	}

	return writeJSON(path, export)
}

// ExportOpenAISchema exports tool declarations in OpenAI function-calling format (C).
func ExportOpenAISchema(ctx context.Context, sets *MCPToolSets, path string) error {
	export := OpenAISchemaExport{
		ExportedAt: time.Now().Format(time.RFC3339),
	}

	for _, ts := range sets.toolSets {
		tools := ts.Tools(ctx)
		for _, t := range tools {
			decl := t.Declaration()
			if decl == nil {
				continue
			}
			export.Tools = append(export.Tools, OpenAIFunction{
				Type: "function",
				Function: OpenAIFunctionDecl{
					Name:        decl.Name,
					Description: decl.Description,
					Parameters:  decl.InputSchema,
				},
			})
		}
	}

	return writeJSON(path, export)
}

// GenerateConfigTemplate generates an mcp.json template with discovered tool names (B).
func GenerateConfigTemplate(ctx context.Context, sets *MCPToolSets, config *MCPConfig, path string) error {
	export := ConfigTemplateExport{
		MCPServers: make(map[string]ConfigTemplateServer),
		Defaults:   config.Defaults,
	}
	if export.Defaults.Timeout == 0 {
		export.Defaults.Timeout = 30
	}
	if export.Defaults.Retries == 0 {
		export.Defaults.Retries = 2
	}

	for name, ts := range sets.toolSets {
		serverCfg := config.MCPServers[name]

		entry := ConfigTemplateServer{
			Transport: serverCfg.Transport,
			ServerURL: serverCfg.ServerUrl,
			Command:   serverCfg.Command,
			Args:      serverCfg.Args,
			Disabled:  false,
			Timeout:   serverCfg.Timeout,
		}

		tools := ts.Tools(ctx)
		for _, t := range tools {
			if decl := t.Declaration(); decl != nil {
				entry.Tools = append(entry.Tools, decl.Name)
			}
		}
		export.MCPServers[name] = entry
	}

	return writeJSON(path, export)
}

// runExports executes all enabled exports. Called internally after init.
func runExports(ctx context.Context, sets *MCPToolSets, config *MCPConfig, opts exportOptions) {
	log := logger.L()

	if opts.toolSchemaPath != "" {
		if err := ExportToolSchema(ctx, sets, config, opts.toolSchemaPath); err != nil {
			log.Error("Failed to export tool schema", "path", opts.toolSchemaPath, "error", err)
		} else {
			log.Info("Tool schema exported", "path", opts.toolSchemaPath)
		}
	}

	if opts.openAISchemaPath != "" {
		if err := ExportOpenAISchema(ctx, sets, opts.openAISchemaPath); err != nil {
			log.Error("Failed to export OpenAI schema", "path", opts.openAISchemaPath, "error", err)
		} else {
			log.Info("OpenAI schema exported", "path", opts.openAISchemaPath)
		}
	}

	if opts.configTemplatePath != "" {
		if err := GenerateConfigTemplate(ctx, sets, config, opts.configTemplatePath); err != nil {
			log.Error("Failed to generate config template", "path", opts.configTemplatePath, "error", err)
		} else {
			log.Info("Config template generated", "path", opts.configTemplatePath)
		}
	}
}

// writeJSON marshals v to JSON and writes to path.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
