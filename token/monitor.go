package token

import (
	"sync"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/event"
)

// TokenUsage represents token usage for a single LLM invocation.
type TokenUsage struct {
	TurnNumber       int           `json:"turnNumber"`
	PromptTokens     int           `json:"promptTokens"`
	CompletionTokens int           `json:"completionTokens"`
	TotalTokens      int           `json:"totalTokens"`
	Model            string        `json:"model"`
	Timestamp        time.Time     `json:"timestamp"`
	Duration         time.Duration `json:"duration,omitempty"`
}

const maxUsageHistory = 1000

// Monitor tracks cumulative token usage across pipeline steps.
// It implements pipeline.TokenObserver via the OnCompression method.
type Monitor struct {
	mu                    sync.RWMutex
	maxTokens             int
	totalPromptTokens     int
	totalCompletionTokens int
	totalTokens           int
	turnCount             int
	usageHistory          []TokenUsage
	warningThreshold      float64
	pendingUpdate         bool // set by OnCompression, cleared by translator after push
}

// NewMonitor creates a new token monitor with the given context-window size.
func NewMonitor(maxTokens int) *Monitor {
	return &Monitor{
		maxTokens:        maxTokens,
		usageHistory:     make([]TokenUsage, 0),
		warningThreshold: 0.8,
	}
}

// RecordUsage adds a single-turn usage record.
func (tm *Monitor) RecordUsage(usage TokenUsage) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.totalPromptTokens += usage.PromptTokens
	tm.totalCompletionTokens += usage.CompletionTokens
	tm.totalTokens += usage.TotalTokens
	tm.turnCount++
	usage.TurnNumber = tm.turnCount
	tm.usageHistory = append(tm.usageHistory, usage)

	if len(tm.usageHistory) > maxUsageHistory {
		tm.usageHistory = tm.usageHistory[len(tm.usageHistory)-maxUsageHistory:]
	}
}

// ProcessEvent extracts token usage from a trpc-agent-go event and records it.
// Returns the recorded TokenUsage (zero value if no usage found).
func (tm *Monitor) ProcessEvent(evt *event.Event) TokenUsage {
	if evt == nil || evt.Response == nil || evt.Response.Usage == nil {
		return TokenUsage{}
	}

	usage := TokenUsage{
		PromptTokens:     evt.Response.Usage.PromptTokens,
		CompletionTokens: evt.Response.Usage.CompletionTokens,
		TotalTokens:      evt.Response.Usage.TotalTokens,
		Model:            evt.Response.Model,
		Timestamp:        time.Now(),
	}

	tm.RecordUsage(usage)
	return usage
}

// GetStats returns a snapshot of cumulative statistics.
func (tm *Monitor) GetStats() map[string]any {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	remaining := tm.maxTokens - tm.totalTokens
	usagePercent := 0.0
	if tm.maxTokens > 0 {
		usagePercent = float64(tm.totalTokens) / float64(tm.maxTokens) * 100
	}

	stats := map[string]any{
		"maxTokens":             tm.maxTokens,
		"totalPromptTokens":     tm.totalPromptTokens,
		"totalCompletionTokens": tm.totalCompletionTokens,
		"totalTokens":           tm.totalTokens,
		"remainingTokens":       remaining,
		"usagePercent":          usagePercent,
		"turnCount":             tm.turnCount,
	}

	if tm.turnCount > 0 {
		stats["avgPromptTokens"] = tm.totalPromptTokens / tm.turnCount
		stats["avgCompletionTokens"] = tm.totalCompletionTokens / tm.turnCount
		stats["avgTotalTokens"] = tm.totalTokens / tm.turnCount
		avgTotal := tm.totalTokens / tm.turnCount
		if avgTotal > 0 {
			stats["estimatedRemainingTurns"] = remaining / avgTotal
		}
	}

	return stats
}

// IsWarning returns true when usage exceeds the warning threshold.
func (tm *Monitor) IsWarning() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.maxTokens <= 0 {
		return false
	}
	return float64(tm.totalTokens)/float64(tm.maxTokens) >= tm.warningThreshold
}

// IsCritical returns true when usage exceeds 95%.
func (tm *Monitor) IsCritical() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.maxTokens <= 0 {
		return false
	}
	return float64(tm.totalTokens)/float64(tm.maxTokens) >= 0.95
}

// OnCompression implements pipeline.TokenObserver.
// It adjusts the cumulative token counts to reflect the compressed context size
// and marks a pending update so the translator pushes refreshed stats to the frontend.
func (tm *Monitor) OnCompression(beforeTokens, afterTokens int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	saved := beforeTokens - afterTokens
	if saved <= 0 {
		return
	}

	tm.totalPromptTokens -= saved
	if tm.totalPromptTokens < 0 {
		tm.totalPromptTokens = 0
	}
	tm.totalTokens -= saved
	if tm.totalTokens < 0 {
		tm.totalTokens = 0
	}
	tm.pendingUpdate = true
}

// DrainPendingUpdate atomically checks and clears the pending flag.
// Returns true if a compression update was pending (caller should push refreshed stats).
func (tm *Monitor) DrainPendingUpdate() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if !tm.pendingUpdate {
		return false
	}
	tm.pendingUpdate = false
	return true
}

// Reset clears all tracked data.
func (tm *Monitor) Reset() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.totalPromptTokens = 0
	tm.totalCompletionTokens = 0
	tm.totalTokens = 0
	tm.turnCount = 0
	tm.usageHistory = make([]TokenUsage, 0)
}
