package tools

import (
	"context"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	filetool "trpc.group/trpc-go/trpc-agent-go/tool/file"
)

// CombinedFileToolSet wraps the official filetool ToolSet and custom extra tools
// under a single "file" name. This allows frontmatter to use `tools: [file]`
// to get both read/write/search and delete/rename/tree/clean/stat capabilities.
type CombinedFileToolSet struct {
	official tool.ToolSet
	extra    []tool.Tool
}

// NewCombinedFileToolSet creates a unified file ToolSet combining official and custom tools.
func NewCombinedFileToolSet(baseDir string) (*CombinedFileToolSet, error) {
	official, err := filetool.NewToolSet(filetool.WithBaseDir(baseDir))
	if err != nil {
		return nil, err
	}
	return &CombinedFileToolSet{
		official: official,
		extra:    NewExtraTools(baseDir),
	}, nil
}

// Tools implements the tool.ToolSet interface.
func (c *CombinedFileToolSet) Tools(ctx context.Context) []tool.Tool {
	all := c.official.Tools(ctx)
	all = append(all, c.extra...)
	return all
}

// Close implements the tool.ToolSet interface.
func (c *CombinedFileToolSet) Close() error {
	return c.official.Close()
}

// Name implements the tool.ToolSet interface.
func (c *CombinedFileToolSet) Name() string {
	return "file"
}
