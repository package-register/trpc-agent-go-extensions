package step

import (
	"fmt"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"
)

// ValidationError describes a reference integrity issue in step definitions.
type ValidationError struct {
	StepID    string // the step containing the bad reference
	Field     string // "next" or "fallback.{code}"
	Reference string // the target stepID that was referenced
	Message   string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("step %s: %s references %q — %s", e.StepID, e.Field, e.Reference, e.Message)
}

// ValidateReferences checks that all next/fallback references point to existing step IDs.
// Returns nil if all references are valid.
func ValidateReferences(steps []*pipeline.StepDefinition) []ValidationError {
	known := make(map[string]bool, len(steps))
	for _, s := range steps {
		known[s.Frontmatter.Step] = true
	}

	var errs []ValidationError

	for _, s := range steps {
		sid := s.Frontmatter.Step

		// Check next
		if next := s.Frontmatter.Next; next != "" {
			if !known[next] {
				errs = append(errs, ValidationError{
					StepID:    sid,
					Field:     "next",
					Reference: next,
					Message:   "target step does not exist",
				})
			}
			if next == sid {
				errs = append(errs, ValidationError{
					StepID:    sid,
					Field:     "next",
					Reference: next,
					Message:   "self-loop detected",
				})
			}
		}

		// Check fallback
		for code, target := range s.Frontmatter.Fallback {
			if target == "" {
				continue
			}
			if !known[target] {
				errs = append(errs, ValidationError{
					StepID:    sid,
					Field:     fmt.Sprintf("fallback.%s", code),
					Reference: target,
					Message:   "target step does not exist",
				})
			}
		}
	}

	return errs
}
