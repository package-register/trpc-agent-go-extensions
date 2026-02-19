package prompt

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/logger"
	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// LLMSummarizer implements pipeline.InputSummarizer using an LLM to generate
// concise 2-3 line summaries of input files. Results are cached.
type LLMSummarizer struct {
	llmModel model.Model
	fs       pipeline.FileSystem
	mu       sync.Mutex
	cache    map[string]string
}

// NewLLMSummarizer creates a summarizer backed by an LLM model.
func NewLLMSummarizer(llmModel model.Model, fs pipeline.FileSystem) *LLMSummarizer {
	return &LLMSummarizer{
		llmModel: llmModel,
		fs:       fs,
		cache:    make(map[string]string),
	}
}

// Summarize returns a cached or freshly-generated LLM summary for the given path.
func (s *LLMSummarizer) Summarize(ctx context.Context, path string) (string, error) {
	s.mu.Lock()
	if cached, ok := s.cache[path]; ok {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	content, err := s.fs.ReadFile(path)
	if err != nil {
		return "(读取失败)", nil
	}

	text := string(content)
	const maxChars = 4000
	if len(text) > maxChars {
		text = text[:maxChars] + "\n...(已截断)"
	}

	summary := s.llmSummarize(ctx, path, text)

	s.mu.Lock()
	s.cache[path] = summary
	s.mu.Unlock()
	return summary, nil
}

// llmSummarize calls the model to produce a 2-3 line summary.
func (s *LLMSummarizer) llmSummarize(ctx context.Context, filename, content string) string {
	if s.llmModel == nil {
		return fallbackSummary(content)
	}

	prompt := fmt.Sprintf(
		"请用2-3行中文概括以下文件(%s)的核心内容，保留关键数据点和技术指标。只输出摘要，不要任何前缀。\n\n%s",
		filename, content,
	)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := &model.Request{
		Messages: []model.Message{
			model.NewUserMessage(prompt),
		},
		GenerationConfig: model.GenerationConfig{
			Stream: false,
		},
	}

	ch, err := s.llmModel.GenerateContent(ctx, req)
	if err != nil {
		logger.L().Warn("LLM summary failed", "file", filename, "error", err)
		return fallbackSummary(content)
	}

	var result string
	for resp := range ch {
		if resp.Error != nil {
			logger.L().Warn("LLM summary error", "file", filename, "error", resp.Error.Message)
			return fallbackSummary(content)
		}
		if len(resp.Choices) > 0 {
			result += resp.Choices[0].Message.Content
		}
	}

	if trimmed := strings.TrimSpace(result); trimmed != "" {
		return trimmed
	}
	return fallbackSummary(content)
}

// FallbackSummarizer implements pipeline.InputSummarizer without an LLM.
// It returns the first 5 lines of the file content.
type FallbackSummarizer struct {
	fs pipeline.FileSystem
}

// NewFallbackSummarizer creates a summarizer that returns the first 5 lines.
func NewFallbackSummarizer(fs pipeline.FileSystem) *FallbackSummarizer {
	return &FallbackSummarizer{fs: fs}
}

// Summarize reads the file and returns the first 5 lines.
func (s *FallbackSummarizer) Summarize(_ context.Context, path string) (string, error) {
	content, err := s.fs.ReadFile(path)
	if err != nil {
		return "(读取失败)", nil
	}
	return fallbackSummary(string(content)), nil
}

// fallbackSummary returns the first few lines when LLM is unavailable.
func fallbackSummary(content string) string {
	lines := strings.SplitN(content, "\n", 6)
	if len(lines) > 5 {
		lines = lines[:5]
	}
	return strings.Join(lines, "\n")
}
