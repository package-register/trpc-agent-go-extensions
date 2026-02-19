// Deprecated: This file is superseded by pkg/memory/tracker.go.
// Kept for backward compatibility; will be removed in a future release.
package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/logger"

	"trpc.group/trpc-go/trpc-agent-go/graph"
)

// ArtifactInfo records metadata about a step's output file.
type ArtifactInfo struct {
	StepID    string
	Title     string
	FilePath  string // relative path, e.g. "docs/设计大纲.md"
	Status    string // "completed" | "in_progress" | "pending"
	Summary   string // LLM-generated summary (populated lazily by EnvironmentBuilder)
	LineCount int
	CreatedAt time.Time
}

// FileArtifactTracker tracks produced documents across pipeline steps.
type FileArtifactTracker struct {
	mu        sync.RWMutex
	baseDir   string
	artifacts map[string]*ArtifactInfo // key = stepID
}

// NewFileArtifactTracker creates a tracker rooted at baseDir.
func NewFileArtifactTracker(baseDir string) *FileArtifactTracker {
	return &FileArtifactTracker{
		baseDir:   baseDir,
		artifacts: make(map[string]*ArtifactInfo),
	}
}

// RecordIfCompleted checks whether the output file exists and records it.
// Returns true if the file was found and recorded.
func (t *FileArtifactTracker) RecordIfCompleted(stepID, title, outputPath string) bool {
	absPath := filepath.Join(t.baseDir, outputPath)
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return false
	}

	lineCount := countLines(absPath)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.artifacts[stepID] = &ArtifactInfo{
		StepID:    stepID,
		Title:     title,
		FilePath:  outputPath,
		Status:    "completed",
		LineCount: lineCount,
		CreatedAt: info.ModTime(),
	}
	logger.L().Info("Artifact recorded", "step", stepID, "output", outputPath, "lines", lineCount)
	return true
}

// GetArtifacts returns a snapshot of all recorded artifacts.
func (t *FileArtifactTracker) GetArtifacts() map[string]*ArtifactInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]*ArtifactInfo, len(t.artifacts))
	for k, v := range t.artifacts {
		cp := *v
		out[k] = &cp
	}
	return out
}

// GetArtifact returns a single artifact by stepID.
func (t *FileArtifactTracker) GetArtifact(stepID string) *ArtifactInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if a, ok := t.artifacts[stepID]; ok {
		cp := *a
		return &cp
	}
	return nil
}

// MakePostNodeCallback returns an AfterNodeCallback that records the artifact
// when the confirm node completes successfully.
func (t *FileArtifactTracker) MakePostNodeCallback(stepID, title, outputPath string) graph.AfterNodeCallback {
	return func(_ context.Context, _ *graph.NodeCallbackContext, _ graph.State, _ any, nodeErr error) (any, error) {
		if nodeErr != nil {
			return nil, nodeErr
		}
		t.RecordIfCompleted(stepID, title, outputPath)
		return nil, nil
	}
}

// countLines counts newline characters in a file (best-effort).
func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}
	return count
}
