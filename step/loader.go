package step

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// FileStepLoader implements pipeline.StepLoader.
// It loads step definitions from a filesystem directory, skipping
// "templates/" and "system/" subdirectories and "_" prefixed files.
type FileStepLoader struct {
	fs  pipeline.FileSystem
	dir string
}

// NewFileStepLoader creates a loader that reads .md files from the given directory.
func NewFileStepLoader(fs pipeline.FileSystem, dir string) *FileStepLoader {
	return &FileStepLoader{fs: fs, dir: dir}
}

// Load reads all prompt files from the directory, parses them, and returns
// them sorted by step ID.
func (l *FileStepLoader) Load() ([]*pipeline.StepDefinition, error) {
	var steps []*pipeline.StepDefinition

	err := l.walkDir(l.dir, func(filePath string) error {
		step, err := LoadStep(l.fs, filePath)
		if err != nil {
			return err
		}
		steps = append(steps, step)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps found in %s", l.dir)
	}

	sort.Slice(steps, func(i, j int) bool {
		return steps[i].Frontmatter.Step < steps[j].Frontmatter.Step
	})

	return steps, nil
}

// walkDir recursively walks the directory, calling fn for each .md file.
func (l *FileStepLoader) walkDir(dir string, fn func(string) error) error {
	entries, err := l.fs.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		fullPath := path.Join(dir, name)

		if entry.IsDir() {
			if name == "templates" || name == "system" {
				continue
			}
			if err := l.walkDir(fullPath, fn); err != nil {
				return err
			}
			continue
		}

		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if strings.HasPrefix(name, "_") {
			continue
		}

		if err := fn(fullPath); err != nil {
			return err
		}
	}
	return nil
}

// FilteredStepLoader wraps another StepLoader and filters by step ID prefix.
type FilteredStepLoader struct {
	inner  pipeline.StepLoader
	prefix string
}

// NewFilteredStepLoader creates a loader that only returns steps whose ID starts with prefix.
func NewFilteredStepLoader(inner pipeline.StepLoader, prefix string) *FilteredStepLoader {
	return &FilteredStepLoader{inner: inner, prefix: prefix}
}

// Load returns only steps whose step ID starts with the configured prefix.
func (l *FilteredStepLoader) Load() ([]*pipeline.StepDefinition, error) {
	all, err := l.inner.Load()
	if err != nil {
		return nil, err
	}

	var filtered []*pipeline.StepDefinition
	for _, s := range all {
		if strings.HasPrefix(s.Frontmatter.Step, l.prefix) {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no steps matching prefix %q", l.prefix)
	}
	return filtered, nil
}

// CompositeStepLoader merges results from multiple StepLoaders.
type CompositeStepLoader struct {
	loaders []pipeline.StepLoader
}

// NewCompositeStepLoader creates a loader that merges multiple sources.
func NewCompositeStepLoader(loaders ...pipeline.StepLoader) *CompositeStepLoader {
	return &CompositeStepLoader{loaders: loaders}
}

// Load calls each inner loader, merges results, checks for duplicate step IDs,
// and returns them sorted by step ID.
func (l *CompositeStepLoader) Load() ([]*pipeline.StepDefinition, error) {
	seen := make(map[string]bool)
	var all []*pipeline.StepDefinition

	for _, loader := range l.loaders {
		steps, err := loader.Load()
		if err != nil {
			return nil, err
		}
		for _, s := range steps {
			if seen[s.Frontmatter.Step] {
				return nil, fmt.Errorf("duplicate step ID %q", s.Frontmatter.Step)
			}
			seen[s.Frontmatter.Step] = true
			all = append(all, s)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Frontmatter.Step < all[j].Frontmatter.Step
	})

	return all, nil
}

// InMemoryStepLoader provides step definitions from memory, useful for testing.
type InMemoryStepLoader struct {
	steps []*pipeline.StepDefinition
}

// NewInMemoryStepLoader creates a loader from pre-built step definitions.
func NewInMemoryStepLoader(steps ...*pipeline.StepDefinition) *InMemoryStepLoader {
	return &InMemoryStepLoader{steps: steps}
}

// Load returns the pre-configured steps.
func (l *InMemoryStepLoader) Load() ([]*pipeline.StepDefinition, error) {
	if len(l.steps) == 0 {
		return nil, fmt.Errorf("no steps configured")
	}
	out := make([]*pipeline.StepDefinition, len(l.steps))
	copy(out, l.steps)
	return out, nil
}

// Verify interface compliance at compile time.
var (
	_ pipeline.StepLoader = (*FileStepLoader)(nil)
	_ pipeline.StepLoader = (*FilteredStepLoader)(nil)
	_ pipeline.StepLoader = (*CompositeStepLoader)(nil)
	_ pipeline.StepLoader = (*InMemoryStepLoader)(nil)
)
