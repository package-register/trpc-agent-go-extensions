package memory

import (
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// SummaryPrefix is the prefix used for compression summary messages.
const SummaryPrefix = "[上下文摘要 — 以下是之前对话的压缩总结]\n"

// IsSummaryMessage checks if a system message is a compression summary.
func IsSummaryMessage(content string) bool {
	return strings.HasPrefix(content, "[上下文摘要")
}

// FormatSummaryMessage wraps a summary string into a system message.
func FormatSummaryMessage(summary string) model.Message {
	return model.Message{
		Role:    model.RoleSystem,
		Content: fmt.Sprintf("%s%s", SummaryPrefix, summary),
	}
}
