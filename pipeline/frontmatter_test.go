package pipeline

import "testing"

func TestParsePrompt(t *testing.T) {
	content := `---
step: "3.1"
title: 功能仿真
output: docs/仿真报告.md
output_template: templates/仿真报告.md
input:
  - rtl/*.v
mcp: [eda]
next: "4.1"
advance: confirm
fallback:
  default: "2.1"
  timeout: "3.1"
---

# 功能仿真
内容...`

	frontmatter, body, err := ParsePrompt(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if frontmatter.Step != "3.1" {
		t.Fatalf("step mismatch: %s", frontmatter.Step)
	}
	if frontmatter.Advance != AdvanceConfirm {
		t.Fatalf("advance mismatch: %s", frontmatter.Advance)
	}
	if frontmatter.Fallback["timeout"] != "3.1" {
		t.Fatalf("fallback timeout mismatch: %s", frontmatter.Fallback["timeout"])
	}
	if body == "" {
		t.Fatalf("expected body content")
	}
}

func TestParsePromptMissingDelimiter(t *testing.T) {
	_, _, err := ParsePrompt("no frontmatter")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadRealPrompts(t *testing.T) {
	prompts, err := LoadPrompts("../../prompts")
	if err != nil {
		t.Fatalf("加载 prompts 目录失败: %v", err)
	}

	if len(prompts) != 14 {
		t.Fatalf("期望 14 个 prompt 文件，实际 %d 个", len(prompts))
	}

	// 检查每个 prompt 的基本字段
	for _, p := range prompts {
		if p.Frontmatter.Step == "" {
			t.Errorf("文件 %s 缺少 step 字段", p.Path)
		}
		if p.Frontmatter.Title == "" {
			t.Errorf("文件 %s 缺少 title 字段", p.Path)
		}
		if len(p.Frontmatter.Output) == 0 {
			t.Errorf("文件 %s 缺少 output 字段", p.Path)
		}
		if p.Body == "" {
			t.Errorf("文件 %s body 为空", p.Path)
		}
		t.Logf("✓ %s (%s) advance=%s fallback=%v next=%q",
			p.Frontmatter.Step, p.Frontmatter.Title,
			p.Frontmatter.Advance, p.Frontmatter.Fallback,
			p.Frontmatter.Next)
	}

	// 验证 fallback map 格式（非 null 的必须是 map）
	stepMap := make(map[string]*PromptFile)
	for _, p := range prompts {
		stepMap[p.Frontmatter.Step] = p
	}

	// 3.1 应有细化的 fallback 路由
	if p := stepMap["3.1"]; p != nil {
		if p.Frontmatter.Fallback["default"] != "2.1" {
			t.Errorf("3.1 fallback.default 期望 2.1，实际 %q", p.Frontmatter.Fallback["default"])
		}
		if p.Frontmatter.Fallback["compile_error"] != "2.1" {
			t.Errorf("3.1 fallback.compile_error 期望 2.1，实际 %q", p.Frontmatter.Fallback["compile_error"])
		}
		if p.Frontmatter.Fallback["timeout"] != "3.1" {
			t.Errorf("3.1 fallback.timeout 期望 3.1，实际 %q", p.Frontmatter.Fallback["timeout"])
		}
	}

	// 7.1 应为 confirm 模式
	if p := stepMap["7.1"]; p != nil {
		if p.Frontmatter.Advance != AdvanceConfirm {
			t.Errorf("7.1 advance 期望 confirm，实际 %q", p.Frontmatter.Advance)
		}
	}

	// 9.1 应为 block 模式
	if p := stepMap["9.1"]; p != nil {
		if p.Frontmatter.Advance != AdvanceBlock {
			t.Errorf("9.1 advance 期望 block，实际 %q", p.Frontmatter.Advance)
		}
	}
}
