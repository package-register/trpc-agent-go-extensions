package telemetry

import (
	"context"
)

// noopTracer 空实现追踪器（默认）
// 当未启用可观测性时使用，零开销
type noopTracer struct{}

// noopSpan 空实现 span
type noopSpan struct{}

// Noop 返回空实现追踪器
func Noop() Tracer {
	return &noopTracer{}
}

// StartSpan 实现 Tracer 接口
func (t *noopTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	return ctx, &noopSpan{}
}

// Shutdown 实现 Tracer 接口
func (t *noopTracer) Shutdown(ctx context.Context) error {
	return nil
}

// IsEnabled 实现 Tracer 接口
func (t *noopTracer) IsEnabled() bool {
	return false
}

// SetAttributes 实现 Span 接口
func (s *noopSpan) SetAttributes(attrs ...Attribute) {}

// SetStatus 实现 Span 接口
func (s *noopSpan) SetStatus(status Status, description string) {}

// RecordError 实现 Span 接口
func (s *noopSpan) RecordError(err error) {}

// End 实现 Span 接口
func (s *noopSpan) End() {}
