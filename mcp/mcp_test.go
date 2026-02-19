// Package mcp provides tests for MCP tool integration.
package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigFromMcpJson 测试从 mcp.json 加载配置
func TestLoadConfigFromMcpJson(t *testing.T) {
	// 获取当前目录
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}

	// mcp.json 路径
	configPath := filepath.Join(cwd, "mcp.json")

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("mcp.json 不存在，跳过测试")
	}

	// 加载配置
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config == nil {
		t.Fatal("配置不应为空")
	}

	t.Logf("加载了 %d 个 MCP 服务器", len(config.MCPServers))

	// 验证配置
	for name, serverCfg := range config.MCPServers {
		t.Logf("服务器: %s", name)
		t.Logf("  - Disabled: %v", serverCfg.Disabled)
		t.Logf("  - Command: %s", serverCfg.Command)
		t.Logf("  - ServerUrl: %s", serverCfg.ServerUrl)
		t.Logf("  - Tools: %v", serverCfg.Tools)
	}
}

// TestMCPToolSetsFromRealConfig 测试使用真实配置创建工具集
func TestMCPToolSetsFromRealConfig(t *testing.T) {
	ctx := context.Background()

	// 获取当前目录
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}

	// mcp.json 路径
	configPath := filepath.Join(cwd, "mcp.json")

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("mcp.json 不存在，跳过测试")
	}

	// 加载配置
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 创建工具集
	sets, err := NewMCPToolSetsFromStruct(ctx, config)
	if err != nil {
		t.Fatalf("创建工具集失败: %v", err)
	}

	if sets == nil {
		t.Fatal("工具集不应为空")
	}

	t.Logf("成功初始化 %d 个 MCP 服务器", sets.Count())

	// 获取活跃工具集
	activeSets := sets.GetActiveToolSets()
	t.Logf("活跃工具集数量: %d", len(activeSets))

	// 获取每个工具集的工具列表
	for _, ts := range activeSets {
		tools := ts.Tools(ctx)
		t.Logf("工具集 %s 有 %d 个工具", ts.Name(), len(tools))
		for i, tool := range tools {
			if decl := tool.Declaration(); decl != nil {
				t.Logf("  工具 %d: %s - %s", i+1, decl.Name, decl.Description)
			} else {
				t.Logf("  工具 %d: %T", i+1, tool)
			}
		}
	}

	// 关闭工具集
	sets.Close()
}

// TestConfigFieldValidation 测试配置字段验证
func TestConfigFieldValidation(t *testing.T) {
	// 获取当前目录
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}

	// mcp.json 路径
	configPath := filepath.Join(cwd, "mcp.json")

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("mcp.json 不存在，跳过测试")
	}

	// 加载配置
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证每个服务器配置
	for name, serverCfg := range config.MCPServers {
		t.Run(name, func(t *testing.T) {
			// 验证必填字段
			if serverCfg.Command == "" && serverCfg.ServerUrl == "" {
				t.Error("必须提供 command 或 serverUrl")
			}

			// 验证传输类型
			if serverCfg.ServerUrl != "" {
				t.Logf("使用 HTTP 传输: %s", serverCfg.ServerUrl)
			} else {
				t.Logf("使用 stdio 传输: %s", serverCfg.Command)
			}

			// 验证工具列表
			if len(serverCfg.Tools) > 0 {
				t.Logf("工具列表: %v", serverCfg.Tools)
			}
		})
	}
}

// TestEnvironmentVariableInterpolation 测试环境变量插值
func TestEnvironmentVariableInterpolation(t *testing.T) {
	// 设置测试环境变量
	if err := os.Setenv("MCP_TEST_VAR", "test-value"); err != nil {
		t.Fatalf("设置环境变量失败: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("MCP_TEST_VAR"); err != nil {
			t.Logf("清理环境变量失败: %v", err)
		}
	}()

	// 创建临时配置文件
	tmpFile, err := os.CreateTemp("", "mcp-interp-*.json")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Logf("删除临时文件失败: %v", err)
		}
	}()

	configContent := `{
  "mcpServers": {
    "test-server": {
      "command": "echo",
      "args": ["${env:MCP_TEST_VAR}"],
      "disabled": false
    }
  },
  "defaults": {
    "timeout": 30,
    "retries": 2
  }
}`

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("关闭临时文件失败: %v", err)
	}

	// 加载配置
	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证环境变量插值
	serverCfg := config.MCPServers["test-server"]
	if len(serverCfg.Args) == 0 {
		t.Fatal("Args 不应为空")
	}

	expected := "test-value"
	if serverCfg.Args[0] != expected {
		t.Errorf("期望 %s，实际 %s", expected, serverCfg.Args[0])
	}

	t.Logf("环境变量插值成功: %s", serverCfg.Args[0])
}
