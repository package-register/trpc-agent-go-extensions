package telemetry

import (
	"maps"
	"context"
)

// Tracer 定义可观测性追踪接口
// 采用接口抽象，支持多种追踪后端（Langfuse、其他、空实现）
type Tracer interface {
	// StartSpan 开始一个新的追踪 span
	StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)

	// Shutdown 优雅关闭追踪器
	Shutdown(ctx context.Context) error

	// IsEnabled 检查追踪器是否启用
	IsEnabled() bool
}

// Span 表示一个追踪区间
type Span interface {
	// SetAttributes 设置 span 属性
	SetAttributes(attrs ...Attribute)

	// SetStatus 设置 span 状态
	SetStatus(status Status, description string)

	// RecordError 记录错误
	RecordError(err error)

	// End 结束 span
	End()
}

// SpanAttribute span 属性选项
type SpanOption func(*SpanConfig)

type SpanConfig struct {
	Attributes map[string]any
}

// Attribute 追踪属性
type Attribute struct {
	Key   string
	Value any
}

// Status span 状态
type Status struct {
	Code int
}

// 预定义状态
var (
	StatusOK = Status{Code: 1}
	StatusError = Status{Code: 2}
)

// WithAttributes 创建属性选项
func WithAttributes(attrs map[string]any) SpanOption {
	return func(cfg *SpanConfig) {
		if cfg.Attributes == nil {
			cfg.Attributes = make(map[string]any)
		}
		maps.Copy(cfg.Attributes, attrs)
	}
}
