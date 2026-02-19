package pipeline

import (
	"io/fs"
	"os"
	"path/filepath"
)

// OSFS implements FileSystem using the real operating system.
// It resolves all paths relative to a base directory.
type OSFS struct {
	baseDir string
}

// NewOSFS creates a FileSystem rooted at the given base directory.
func NewOSFS(baseDir string) *OSFS {
	return &OSFS{baseDir: baseDir}
}

// Open implements fs.FS.
func (f *OSFS) Open(name string) (fs.File, error) {
	return os.Open(f.resolve(name))
}

// ReadFile implements fs.ReadFileFS.
func (f *OSFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(f.resolve(name))
}

// Stat implements FileSystem.
func (f *OSFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(f.resolve(name))
}

// ReadDir implements FileSystem.
func (f *OSFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(f.resolve(name))
}

func (f *OSFS) resolve(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(f.baseDir, name)
}

// Verify interface compliance at compile time.
var _ FileSystem = (*OSFS)(nil)
