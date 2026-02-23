package memory

import (
	"context"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// Compressor compresses message history when token usage approaches the context window limit.
type Compressor interface {
	CompressIfNeeded(ctx context.Context, msgs []model.Message, currentTokens int) (compressed []model.Message, didCompress bool, err error)
}

// ArtifactTracker tracks produced documents across pipeline steps.
type ArtifactTracker interface {
	RecordCompleted(stepID, title, outputPath string) bool
	GetArtifact(stepID string) *ArtifactInfo
	GetAll() map[string]*ArtifactInfo
}

// ArtifactInfo holds metadata about a produced document.
type ArtifactInfo struct {
	StepID     string
	Title      string
	FilePath   string
	Status     string
	LineCount  int
	CreatedAt  time.Time
}
