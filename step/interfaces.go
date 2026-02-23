package step

import (
	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// Loader loads step definitions from any source (filesystem, memory, etc.).
type Loader interface {
	Load() ([]*pipeline.StepDefinition, error)
}
