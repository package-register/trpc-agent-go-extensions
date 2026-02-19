package memory

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// testFS wraps fstest.MapFS to implement pipeline.FileSystem.
type testFS struct {
	fstest.MapFS
}

func (t testFS) Stat(name string) (fs.FileInfo, error) {
	f, err := t.MapFS.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (t testFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(t.MapFS, name)
}

func TestFileTracker_RecordCompleted(t *testing.T) {
	tfs := testFS{fstest.MapFS{
		"docs/design.md": &fstest.MapFile{Data: []byte("line1\nline2\nline3\n")},
	}}

	tracker := NewFileTracker(tfs)
	ok := tracker.RecordCompleted("1.1", "设计大纲", "docs/design.md")
	if !ok {
		t.Fatal("expected RecordCompleted to return true for existing file")
	}

	a := tracker.GetArtifact("1.1")
	if a == nil {
		t.Fatal("expected artifact to exist")
	}
	if a.StepID != "1.1" {
		t.Fatalf("expected stepID=1.1, got %s", a.StepID)
	}
	if a.Status != "completed" {
		t.Fatalf("expected status=completed, got %s", a.Status)
	}
	if a.LineCount != 3 {
		t.Fatalf("expected 3 lines, got %d", a.LineCount)
	}
}

func TestFileTracker_FileNotFound(t *testing.T) {
	tfs := testFS{fstest.MapFS{}}
	tracker := NewFileTracker(tfs)

	ok := tracker.RecordCompleted("1.1", "设计大纲", "docs/missing.md")
	if ok {
		t.Fatal("expected RecordCompleted to return false for missing file")
	}

	a := tracker.GetArtifact("1.1")
	if a != nil {
		t.Fatal("expected no artifact for missing file")
	}
}

func TestFileTracker_GetAll(t *testing.T) {
	tfs := testFS{fstest.MapFS{
		"docs/a.md": &fstest.MapFile{Data: []byte("a\n")},
		"docs/b.md": &fstest.MapFile{Data: []byte("b\n")},
	}}

	tracker := NewFileTracker(tfs)
	tracker.RecordCompleted("1.1", "Step A", "docs/a.md")
	tracker.RecordCompleted("1.2", "Step B", "docs/b.md")

	all := tracker.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(all))
	}

	// Verify it's a snapshot (not a reference)
	all["1.1"].Title = "modified"
	original := tracker.GetArtifact("1.1")
	if original.Title == "modified" {
		t.Fatal("GetAll should return a snapshot, not references")
	}
}

func TestFileTracker_GetArtifact_Snapshot(t *testing.T) {
	tfs := testFS{fstest.MapFS{
		"docs/a.md": &fstest.MapFile{Data: []byte("a\n")},
	}}

	tracker := NewFileTracker(tfs)
	tracker.RecordCompleted("1.1", "Step A", "docs/a.md")

	a1 := tracker.GetArtifact("1.1")
	a1.Title = "modified"

	a2 := tracker.GetArtifact("1.1")
	if a2.Title == "modified" {
		t.Fatal("GetArtifact should return a copy, not a reference")
	}
}

func TestFileTracker_ImplementsInterface(t *testing.T) {
	tfs := testFS{fstest.MapFS{}}
	var _ pipeline.ArtifactTracker = NewFileTracker(tfs)
}
