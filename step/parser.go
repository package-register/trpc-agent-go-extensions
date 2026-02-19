package step

import (
	"fmt"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// LoadStep reads and parses a step definition from the filesystem.
func LoadStep(fs pipeline.FileSystem, path string) (*pipeline.StepDefinition, error) {
	content, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read step: %w", err)
	}

	frontmatter, body, err := pipeline.ParsePrompt(string(content))
	if err != nil {
		return nil, err
	}

	return &pipeline.StepDefinition{
		Path:        path,
		Frontmatter: frontmatter,
		Body:        body,
	}, nil
}
