package pipeline

import (
	"context"
	"errors"
	"strings"
)

// ErrorCode represents a standardized tool error classification.
type ErrorCode string

const (
	ErrCodeCompileError    ErrorCode = "compile_error"
	ErrCodeLintError       ErrorCode = "lint_error"
	ErrCodeAssertionFail   ErrorCode = "assertion_fail"
	ErrCodeTimeout         ErrorCode = "timeout"
	ErrCodeToolUnavailable ErrorCode = "tool_unavailable"
	ErrCodeInputMissing    ErrorCode = "input_missing"
	ErrCodeRuntimeError    ErrorCode = "runtime_error"
	ErrCodeUnknown         ErrorCode = "unknown"
)

// ToolError allows callers to attach a specific error code.
type ToolError struct {
	Code ErrorCode
	Err  error
}

func (e ToolError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e ToolError) Unwrap() error {
	return e.Err
}

// ClassifyToolError maps an error to a standardized ErrorCode.
func ClassifyToolError(err error) ErrorCode {
	if err == nil {
		return ""
	}

	var toolErr ToolError
	if errors.As(err, &toolErr) && toolErr.Code != "" {
		return toolErr.Code
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrCodeTimeout
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "assert"):
		return ErrCodeAssertionFail
	case strings.Contains(msg, "lint"):
		return ErrCodeLintError
	case strings.Contains(msg, "compile") || strings.Contains(msg, "syntax"):
		return ErrCodeCompileError
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out"):
		return ErrCodeTimeout
	case strings.Contains(msg, "tool") && strings.Contains(msg, "not found"):
		return ErrCodeToolUnavailable
	case strings.Contains(msg, "unavailable") || strings.Contains(msg, "connection refused"):
		return ErrCodeToolUnavailable
	case strings.Contains(msg, "not found") || strings.Contains(msg, "missing"):
		return ErrCodeInputMissing
	default:
		return ErrCodeRuntimeError
	}
}
