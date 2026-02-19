package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

// mockMCPToolSets creates a MCPToolSets with no real connections for unit testing export functions.
// We test the export logic with a real MCPToolSets but skip if no server is available.

func TestExportToolSchema(t *testing.T) {
	sets, config := setupTestSets(t)
	if sets == nil {
		return
	}
	defer sets.Close()

	outPath := filepath.Join(t.TempDir(), "tools_schema.json")
	err := ExportToolSchema(context.Background(), sets, config, outPath)
	if err != nil {
		t.Fatalf("ExportToolSchema failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var export ToolSchemaExport
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if export.ExportedAt == "" {
		t.Error("expected exported_at to be set")
	}
	if len(export.Servers) == 0 {
		t.Error("expected at least one server")
	}

	for name, srv := range export.Servers {
		t.Logf("Server %s: %d tools", name, len(srv.Tools))
		for _, tl := range srv.Tools {
			t.Logf("  - %s: %s", tl.Name, tl.Description)
		}
	}
}

func TestExportOpenAISchema(t *testing.T) {
	sets, _ := setupTestSets(t)
	if sets == nil {
		return
	}
	defer sets.Close()

	outPath := filepath.Join(t.TempDir(), "openai_functions.json")
	err := ExportOpenAISchema(context.Background(), sets, outPath)
	if err != nil {
		t.Fatalf("ExportOpenAISchema failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var export OpenAISchemaExport
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if export.ExportedAt == "" {
		t.Error("expected exported_at to be set")
	}

	for _, fn := range export.Tools {
		if fn.Type != "function" {
			t.Errorf("expected type=function, got %s", fn.Type)
		}
		if fn.Function.Name == "" {
			t.Error("expected function name to be set")
		}
		t.Logf("OpenAI function: %s", fn.Function.Name)
	}
}

func TestGenerateConfigTemplate(t *testing.T) {
	sets, config := setupTestSets(t)
	if sets == nil {
		return
	}
	defer sets.Close()

	outPath := filepath.Join(t.TempDir(), "mcp_template.json")
	err := GenerateConfigTemplate(context.Background(), sets, config, outPath)
	if err != nil {
		t.Fatalf("GenerateConfigTemplate failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var export ConfigTemplateExport
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(export.MCPServers) == 0 {
		t.Error("expected at least one server in template")
	}
	if export.Defaults.Timeout == 0 {
		t.Error("expected defaults.timeout to be set")
	}

	for name, srv := range export.MCPServers {
		t.Logf("Template server %s: %d tools discovered", name, len(srv.Tools))
	}
}

func TestWriteJSON_InvalidPath(t *testing.T) {
	err := writeJSON(filepath.Join(t.TempDir(), "nonexistent", "deep", "path.json"), map[string]string{"a": "b"})
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestExportWithMockData(t *testing.T) {
	// Test export functions with in-memory mock data (no real MCP connection)
	sets := &MCPToolSets{
		toolSets: make(map[string]*mcp.ToolSet),
	}
	config := &MCPConfig{
		MCPServers: map[string]MCPServerConfig{},
		Defaults:   MCPDefaults{Timeout: 30, Retries: 2},
	}

	// Test A: empty export should succeed
	outPath := filepath.Join(t.TempDir(), "empty_tools.json")
	if err := ExportToolSchema(context.Background(), sets, config, outPath); err != nil {
		t.Fatalf("ExportToolSchema with empty sets failed: %v", err)
	}
	data, _ := os.ReadFile(outPath)
	var schema ToolSchemaExport
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(schema.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(schema.Servers))
	}

	// Test C: empty export
	outPath2 := filepath.Join(t.TempDir(), "empty_openai.json")
	if err := ExportOpenAISchema(context.Background(), sets, outPath2); err != nil {
		t.Fatalf("ExportOpenAISchema with empty sets failed: %v", err)
	}

	// Test B: empty template with defaults
	outPath3 := filepath.Join(t.TempDir(), "empty_template.json")
	if err := GenerateConfigTemplate(context.Background(), sets, config, outPath3); err != nil {
		t.Fatalf("GenerateConfigTemplate with empty sets failed: %v", err)
	}
	data3, _ := os.ReadFile(outPath3)
	var tmpl ConfigTemplateExport
	if err := json.Unmarshal(data3, &tmpl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tmpl.Defaults.Timeout != 30 {
		t.Errorf("expected timeout=30, got %d", tmpl.Defaults.Timeout)
	}
}

// setupTestSets tries to connect to the real MCP server from mcp.json.
// Skips the test if the config or server is unavailable.
func setupTestSets(t *testing.T) (*MCPToolSets, *MCPConfig) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get cwd")
		return nil, nil
	}
	configPath := filepath.Join(cwd, "mcp.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("mcp.json not found, skipping integration test")
		return nil, nil
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Skip("cannot load mcp.json:", err)
		return nil, nil
	}

	sets, err := NewMCPToolSetsFromStruct(context.Background(), config)
	if err != nil {
		t.Skip("cannot init MCP sets:", err)
		return nil, nil
	}
	if sets.Count() == 0 {
		t.Skip("no active MCP servers")
		return nil, nil
	}

	return sets, config
}

// Verify tool.Declaration is properly serializable
func TestDeclarationJSON(t *testing.T) {
	decl := tool.Declaration{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: &tool.Schema{
			Type: "object",
			Properties: map[string]*tool.Schema{
				"code": {Type: "string", Description: "source code"},
			},
			Required: []string{"code"},
		},
	}

	data, err := json.MarshalIndent(decl, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	t.Logf("Declaration JSON:\n%s", string(data))

	var parsed tool.Declaration
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Name != "test_tool" {
		t.Errorf("expected name=test_tool, got %s", parsed.Name)
	}
	if parsed.InputSchema == nil || len(parsed.InputSchema.Properties) != 1 {
		t.Error("expected InputSchema.Properties to have 1 entry")
	}
}
