package memory

import (
	"sync"

	"github.com/package-register/trpc-agent-go-extensions/logger"
	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// FileTracker implements pipeline.ArtifactTracker.
// It tracks produced documents across pipeline steps using a FileSystem interface
// instead of direct os.Stat calls.
type FileTracker struct {
	fs   pipeline.FileSystem
	mu   sync.RWMutex
	data map[string]*pipeline.ArtifactInfo
}

// NewFileTracker creates a tracker that uses the provided FileSystem for file access.
func NewFileTracker(fs pipeline.FileSystem) *FileTracker {
	return &FileTracker{
		fs:   fs,
		data: make(map[string]*pipeline.ArtifactInfo),
	}
}

// RecordCompleted checks whether the output file exists and records it.
// Returns true if the file was found and recorded.
func (t *FileTracker) RecordCompleted(stepID, title, outputPath string) bool {
	info, err := t.fs.Stat(outputPath)
	if err != nil || info.IsDir() {
		return false
	}

	lineCount := t.countLines(outputPath)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.data[stepID] = &pipeline.ArtifactInfo{
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

// GetArtifact returns a single artifact by stepID.
func (t *FileTracker) GetArtifact(stepID string) *pipeline.ArtifactInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if a, ok := t.data[stepID]; ok {
		cp := *a
		return &cp
	}
	return nil
}

// GetAll returns a snapshot of all recorded artifacts.
func (t *FileTracker) GetAll() map[string]*pipeline.ArtifactInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]*pipeline.ArtifactInfo, len(t.data))
	for k, v := range t.data {
		cp := *v
		out[k] = &cp
	}
	return out
}

// countLines counts newline characters in a file (best-effort).
func (t *FileTracker) countLines(path string) int {
	data, err := t.fs.ReadFile(path)
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
