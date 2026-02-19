package prompt

import (
	"io/fs"
	"testing/fstest"
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

// newTestFS creates a testFS from a map of path → content.
func newTestFS(files map[string]string) testFS {
	m := make(fstest.MapFS)
	for path, content := range files {
		m[path] = &fstest.MapFile{Data: []byte(content)}
	}
	return testFS{m}
}
