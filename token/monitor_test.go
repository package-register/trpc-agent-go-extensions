package token

import (
	"testing"
)

func TestMonitor_RecordUsage(t *testing.T) {
	m := NewMonitor(100000)
	m.RecordUsage(TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})

	stats := m.GetStats()
	if stats["totalTokens"] != 150 {
		t.Fatalf("expected totalTokens=150, got %v", stats["totalTokens"])
	}
	if stats["turnCount"] != 1 {
		t.Fatalf("expected turnCount=1, got %v", stats["turnCount"])
	}
}

func TestMonitor_OnCompression(t *testing.T) {
	m := NewMonitor(100000)
	m.RecordUsage(TokenUsage{PromptTokens: 5000, CompletionTokens: 1000, TotalTokens: 6000})

	m.OnCompression(5000, 2000) // saved 3000

	stats := m.GetStats()
	totalTokens, _ := stats["totalTokens"].(int)
	if totalTokens != 3000 {
		t.Fatalf("expected totalTokens=3000 after compression, got %d", totalTokens)
	}
	promptTokens, _ := stats["totalPromptTokens"].(int)
	if promptTokens != 2000 {
		t.Fatalf("expected totalPromptTokens=2000 after compression, got %d", promptTokens)
	}
}

func TestMonitor_DrainPendingUpdate(t *testing.T) {
	m := NewMonitor(100000)

	if m.DrainPendingUpdate() {
		t.Fatal("expected no pending update initially")
	}

	m.OnCompression(5000, 2000)
	if !m.DrainPendingUpdate() {
		t.Fatal("expected pending update after compression")
	}
	if m.DrainPendingUpdate() {
		t.Fatal("expected no pending update after drain")
	}
}

func TestMonitor_IsWarning(t *testing.T) {
	m := NewMonitor(1000)
	m.RecordUsage(TokenUsage{PromptTokens: 700, CompletionTokens: 100, TotalTokens: 800})

	if !m.IsWarning() {
		t.Fatal("expected warning at 80% usage")
	}

	m2 := NewMonitor(1000)
	m2.RecordUsage(TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	if m2.IsWarning() {
		t.Fatal("expected no warning at 15% usage")
	}
}

func TestMonitor_IsCritical(t *testing.T) {
	m := NewMonitor(1000)
	m.RecordUsage(TokenUsage{PromptTokens: 900, CompletionTokens: 60, TotalTokens: 960})

	if !m.IsCritical() {
		t.Fatal("expected critical at 96% usage")
	}
}

func TestMonitor_HistoryLimit(t *testing.T) {
	m := NewMonitor(100000)
	for i := 0; i < 1100; i++ {
		m.RecordUsage(TokenUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2})
	}

	m.mu.RLock()
	histLen := len(m.usageHistory)
	m.mu.RUnlock()

	if histLen > maxUsageHistory {
		t.Fatalf("expected history capped at %d, got %d", maxUsageHistory, histLen)
	}
}

func TestMonitor_Reset(t *testing.T) {
	m := NewMonitor(100000)
	m.RecordUsage(TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	m.Reset()

	stats := m.GetStats()
	if stats["totalTokens"] != 0 {
		t.Fatalf("expected totalTokens=0 after reset, got %v", stats["totalTokens"])
	}
	if stats["turnCount"] != 0 {
		t.Fatalf("expected turnCount=0 after reset, got %v", stats["turnCount"])
	}
}

func TestMonitor_ZeroMaxTokens(t *testing.T) {
	m := NewMonitor(0)
	m.RecordUsage(TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})

	if m.IsWarning() {
		t.Fatal("expected no warning with zero maxTokens")
	}
	if m.IsCritical() {
		t.Fatal("expected no critical with zero maxTokens")
	}
}
