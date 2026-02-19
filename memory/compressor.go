package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/logger"
	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

const summarizePrompt = `请将以下对话历史压缩为一段简洁的摘要，保留所有关键信息、决策、结论和产出物路径。
摘要应该让后续对话能够无缝继续，不丢失重要上下文。
使用中文输出。不要添加任何前缀或标题，直接输出摘要内容。

对话历史：
`

// LLMCompressor implements pipeline.Compressor.
// It compresses message history when token usage approaches the context window limit,
// replacing old messages with an LLM-generated summary while keeping recent turns intact.
type LLMCompressor struct {
	llmModel        model.Model
	counter         pipeline.TokenCounter
	contextWindow   int
	threshold       float64
	keepRecentTurns int
}

// NewLLMCompressor creates a compressor that delegates token counting to the provided counter.
func NewLLMCompressor(
	llmModel model.Model,
	counter pipeline.TokenCounter,
	contextWindow int,
	threshold float64,
	keepRecentTurns int,
) *LLMCompressor {
	if keepRecentTurns <= 0 {
		keepRecentTurns = 3
	}
	if threshold <= 0 || threshold >= 1 {
		threshold = 0.7
	}
	return &LLMCompressor{
		llmModel:        llmModel,
		counter:         counter,
		contextWindow:   contextWindow,
		threshold:       threshold,
		keepRecentTurns: keepRecentTurns,
	}
}

// CompressIfNeeded checks whether the current prompt token count exceeds the
// threshold ratio and, if so, compresses the message history.
func (c *LLMCompressor) CompressIfNeeded(
	ctx context.Context,
	msgs []model.Message,
	currentTokens int,
) ([]model.Message, bool, error) {
	if c.contextWindow <= 0 || currentTokens <= 0 {
		return msgs, false, nil
	}

	ratio := float64(currentTokens) / float64(c.contextWindow)
	if ratio < c.threshold {
		return msgs, false, nil
	}

	logger.L().Info("Context compression triggered",
		"ratio", fmt.Sprintf("%.1f%%", ratio*100),
		"threshold", fmt.Sprintf("%.0f%%", c.threshold*100))

	compressed, err := c.compress(ctx, msgs)
	if err != nil {
		logger.L().Warn("Compression failed, using original messages", "error", err)
		return msgs, false, nil
	}

	// compress() returns the original slice when there aren't enough messages to compress.
	didCompress := len(compressed) != len(msgs) || (len(compressed) > 0 && &compressed[0] != &msgs[0])
	return compressed, didCompress, nil
}

// compress performs the actual message compression.
// Layer-aware: preserves all system messages (Layer 1+2 and previous summaries),
// only compresses Layer 3 conversation messages (user/assistant).
func (c *LLMCompressor) compress(ctx context.Context, msgs []model.Message) ([]model.Message, error) {
	if len(msgs) <= 1 {
		return msgs, nil
	}

	var systemMsgs []model.Message
	var conversationMsgs []model.Message
	for _, m := range msgs {
		if m.Role == model.RoleSystem {
			systemMsgs = append(systemMsgs, m)
		} else {
			conversationMsgs = append(conversationMsgs, m)
		}
	}

	keepCount := c.keepRecentTurns * 2
	if keepCount >= len(conversationMsgs) {
		return msgs, nil
	}

	toCompress := conversationMsgs[:len(conversationMsgs)-keepCount]
	toKeep := conversationMsgs[len(conversationMsgs)-keepCount:]

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
func (c *LLMCompressor) callSummarize(ctx context.Context, conversationText string) (string, error) {
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
