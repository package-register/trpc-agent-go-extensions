package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveSafePath validates and resolves a relative path within baseDir.
// It rejects any path that would escape baseDir (path traversal).
func ResolveSafePath(baseDir, relPath string) (string, error) {
	cleanBase := filepath.Clean(baseDir)
	full := filepath.Join(cleanBase, filepath.FromSlash(relPath))
	full = filepath.Clean(full)
	if full != cleanBase && !strings.HasPrefix(full, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal denied: %s", relPath)
	}
	return full, nil
}
