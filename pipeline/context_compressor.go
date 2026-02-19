// Deprecated: This file is superseded by pkg/memory/compressor.go and pkg/memory/summary.go.
// Kept for backward compatibility; will be removed in a future release.
package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/logger"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

const summarizePrompt = `请将以下对话历史压缩为一段简洁的摘要，保留所有关键信息、决策、结论和产出物路径。
摘要应该让后续对话能够无缝继续，不丢失重要上下文。
使用中文输出。不要添加任何前缀或标题，直接输出摘要内容。

对话历史：
`

// CompressionObserver is notified when context compression occurs.
// Implementations can update token monitors, emit events, etc.
type CompressionObserver interface {
	OnCompression(beforeTokens, afterTokens int)
}

// ContextCompressor compresses message history when prompt tokens
// approach the context window limit. It replaces old messages with a summary
// while keeping system messages (Layer 1+2) and recent turns intact.
type ContextCompressor struct {
	llmModel        model.Model
	tokenCounter    model.TokenCounter
	contextWindow   int
	threshold       float64 // e.g. 0.7
	keepRecentTurns int     // number of user/assistant turn pairs to preserve
	observer        CompressionObserver
}

// NewContextCompressor creates a compressor with accurate token counting.
func NewContextCompressor(
	llmModel model.Model,
	contextWindow int,
	threshold float64,
	keepRecentTurns int,
) *ContextCompressor {
	if keepRecentTurns <= 0 {
		keepRecentTurns = 3
	}
	if threshold <= 0 || threshold >= 1 {
		threshold = 0.7
	}
	return &ContextCompressor{
		llmModel:        llmModel,
		tokenCounter:    model.NewSimpleTokenCounter(),
		contextWindow:   contextWindow,
		threshold:       threshold,
		keepRecentTurns: keepRecentTurns,
	}
}

// SetObserver registers an observer that is notified on compression events.
func (c *ContextCompressor) SetObserver(obs CompressionObserver) {
	c.observer = obs
}

// CompressIfNeeded checks whether the current prompt token count exceeds the
// threshold ratio and, if so, compresses the message history.
// Returns the (possibly compressed) messages, whether compression occurred, and any error.
func (c *ContextCompressor) CompressIfNeeded(
	ctx context.Context,
	messages []model.Message,
	currentPromptTokens int,
) ([]model.Message, bool, error) {
	if c.contextWindow <= 0 || currentPromptTokens <= 0 {
		return messages, false, nil
	}

	ratio := float64(currentPromptTokens) / float64(c.contextWindow)
	if ratio < c.threshold {
		return messages, false, nil
	}

	logger.L().Info("Context compression triggered", "ratio", fmt.Sprintf("%.1f%%", ratio*100), "threshold", fmt.Sprintf("%.0f%%", c.threshold*100))

	compressed, err := c.compress(ctx, messages)
	if err != nil {
		logger.L().Warn("Compression failed, using original messages", "error", err)
		return messages, false, nil
	}

	return compressed, true, nil
}

// compress performs the actual message compression.
// Layer-aware: preserves all system messages (Layer 1+2 and previous summaries),
// only compresses Layer 3 conversation messages (user/assistant).
func (c *ContextCompressor) compress(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	if len(messages) <= 1 {
		return messages, nil
	}

	// Separate system messages (Layer 1+2, summaries) from conversation (Layer 3)
	var systemMsgs []model.Message
	var conversationMsgs []model.Message
	for _, m := range messages {
		if m.Role == model.RoleSystem {
			systemMsgs = append(systemMsgs, m)
		} else {
			conversationMsgs = append(conversationMsgs, m)
		}
	}

	// Calculate how many conversation messages to keep (keepRecentTurns * 2 for user+assistant pairs)
	keepCount := c.keepRecentTurns * 2
	if keepCount >= len(conversationMsgs) {
		// Not enough conversation messages to compress — keep everything.
		return messages, nil
	}

	toCompress := conversationMsgs[:len(conversationMsgs)-keepCount]
	toKeep := conversationMsgs[len(conversationMsgs)-keepCount:]

	// Build the conversation text to summarise.
	var convText strings.Builder
	for _, msg := range toCompress {
		role := string(msg.Role)
		content := msg.Content
		if len(content) > 2000 {
			content = content[:2000] + "...(截断)"
		}
		convText.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
	}

	summary, err := c.callSummarize(ctx, convText.String())
	if err != nil {
		return nil, err
	}

	// Rebuild: [all system msgs] + [summary] + [recent conversation]
	// Filter out old summaries to avoid accumulation
	var result []model.Message
	for _, sm := range systemMsgs {
		if !IsSummaryMessage(sm.Content) {
			result = append(result, sm)
		}
	}
	result = append(result, FormatSummaryMessage(summary))
	result = append(result, toKeep...)

	logger.L().Info("Context compressed",
		"systemMsgs", len(systemMsgs), "compressed", len(toCompress), "kept", len(toKeep))

	return result, nil
}

// callSummarize invokes the LLM to summarise the conversation.
func (c *ContextCompressor) callSummarize(ctx context.Context, conversationText string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req := &model.Request{
		Messages: []model.Message{
			model.NewUserMessage(summarizePrompt + conversationText),
		},
		GenerationConfig: model.GenerationConfig{
			Stream: false,
		},
	}

	ch, err := c.llmModel.GenerateContent(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarize call failed: %w", err)
	}

	var result strings.Builder
	for resp := range ch {
		if resp.Error != nil {
			return "", fmt.Errorf("summarize API error: %s", resp.Error.Message)
		}
		if len(resp.Choices) > 0 {
			result.WriteString(resp.Choices[0].Message.Content)
		}
	}

	summary := strings.TrimSpace(result.String())
	if summary == "" {
		return "", fmt.Errorf("summarize returned empty result")
	}
	return summary, nil
}

// countTokens estimates the total token count for a message list using the framework's counter.
func (c *ContextCompressor) countTokens(ctx context.Context, msgs []model.Message) int {
	total, err := c.tokenCounter.CountTokensRange(ctx, msgs, 0, len(msgs))
	if err != nil {
		// Fallback: rough estimate
		for _, m := range msgs {
			total += len(m.Content) / 4
		}
	}
	return total
}

// MakePreNodeCallback returns a BeforeNodeCallback that checks token usage
// and triggers compression when needed. Uses the framework's SimpleTokenCounter
// for accurate token estimation.
func (c *ContextCompressor) MakePreNodeCallback() graph.BeforeNodeCallback {
	return func(ctx context.Context, _ *graph.NodeCallbackContext, state graph.State) (any, error) {
		msgs, ok := state[graph.StateKeyMessages].([]model.Message)
		if !ok || len(msgs) <= 1 {
			return nil, nil
		}

		estimatedTokens := c.countTokens(ctx, msgs)

		compressed, didCompress, err := c.CompressIfNeeded(ctx, msgs, estimatedTokens)
		if err != nil || !didCompress {
			return nil, nil
		}

		// Notify observer with before/after token counts
		if c.observer != nil {
			afterTokens := c.countTokens(ctx, compressed)
			c.observer.OnCompression(estimatedTokens, afterTokens)
		}

		// Replace the entire message history with the compressed version.
		return graph.State{
			graph.StateKeyMessages: []graph.MessageOp{
				graph.RemoveAllMessages{},
				graph.AppendMessages{Items: compressed},
			},
		}, nil
	}
}
