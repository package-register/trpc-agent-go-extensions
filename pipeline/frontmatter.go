package pipeline

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// AdvanceMode controls how a step advances after execution.
type AdvanceMode string

const (
	AdvanceAuto    AdvanceMode = "auto"
	AdvanceConfirm AdvanceMode = "confirm"
	AdvanceBlock   AdvanceMode = "block"
)

// OutputField supports both string and []string in YAML.
type OutputField []string

// UnmarshalYAML allows output to be either a single string or a list.
func (o *OutputField) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*o = []string{value.Value}
		return nil
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	*o = list
	return nil
}

// Frontmatter defines the prompt metadata consumed by the pipeline.
type Frontmatter struct {
	Step            string            `yaml:"step"`
	Name            string            `yaml:"name"`
	Title           string            `yaml:"title"`
	Description     string            `yaml:"description"`
	Output          OutputField       `yaml:"output"`
	OutputTemplate  string            `yaml:"output_template"`
	Input           []string          `yaml:"input"`
	Tools           []string          `yaml:"tools"`
	MCP             []string          `yaml:"mcp"`
	Next            string            `yaml:"next"`
	Fallback        map[string]string `yaml:"fallback"`
	Advance         AdvanceMode       `yaml:"advance"`
	Model           string            `yaml:"model"`
	MaxOutputTokens int               `yaml:"max_output_tokens"`
}

// EffectiveTools returns Tools if set, otherwise falls back to MCP.
func (f *Frontmatter) EffectiveTools() []string {
	if len(f.Tools) > 0 {
		return f.Tools
	}
	return f.MCP
}

// PrimaryOutput returns the first output path, or empty string.
func (f *Frontmatter) PrimaryOutput() string {
	if len(f.Output) > 0 {
		return f.Output[0]
	}
	return ""
}

// PromptFile represents a prompt file with parsed frontmatter and body.
type PromptFile struct {
	Path        string
	Frontmatter Frontmatter
	Body        string
}

// StepDefinition is the canonical name for a pipeline step.
// PromptFile is retained as an alias for backward compatibility.
type StepDefinition = PromptFile

// LoadPrompt reads and parses a prompt file from disk.
func LoadPrompt(path string) (*PromptFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prompt: %w", err)
	}

	frontmatter, body, err := ParsePrompt(string(content))
	if err != nil {
		return nil, err
	}

	return &PromptFile{
		Path:        path,
		Frontmatter: frontmatter,
		Body:        body,
	}, nil
}

// ParsePrompt splits YAML frontmatter and returns the body content.
func ParsePrompt(content string) (Frontmatter, string, error) {
	var frontmatter Frontmatter

	if !strings.HasPrefix(content, "---") {
		return frontmatter, "", fmt.Errorf("frontmatter delimiter not found")
	}

	parts := strings.SplitN(content[len("---"):], "---", 2)
	if len(parts) < 2 {
		return frontmatter, "", fmt.Errorf("frontmatter end delimiter not found")
	}

	if err := yaml.Unmarshal([]byte(parts[0]), &frontmatter); err != nil {
		return frontmatter, "", fmt.Errorf("parse frontmatter: %w", err)
	}

	if frontmatter.Advance == "" {
		frontmatter.Advance = AdvanceAuto
	}

	// Backward compat: mcp → tools
	if len(frontmatter.Tools) == 0 && len(frontmatter.MCP) > 0 {
		frontmatter.Tools = frontmatter.MCP
	}

	body := trimLeadingNewline(parts[1])
	return frontmatter, body, nil
}

func trimLeadingNewline(value string) string {
	if strings.HasPrefix(value, "\r\n") {
		return strings.TrimPrefix(value, "\r\n")
	}
	if strings.HasPrefix(value, "\n") {
		return strings.TrimPrefix(value, "\n")
	}
	return value
}
