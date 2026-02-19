package telemetry

import (
	"context"
	"sync"
)

var (
	globalTracer Tracer = Noop()
	tracerMutex  sync.RWMutex
)

// Init 初始化全局追踪器
// 这是一个可选操作，如果未调用，默认使用 Noop 追踪器（零开销）
func Init(tracer Tracer) {
	tracerMutex.Lock()
	defer tracerMutex.Unlock()
	globalTracer = tracer
}

// Get 获取当前全局追踪器
func Get() Tracer {
	tracerMutex.RLock()
	defer tracerMutex.RUnlock()
	return globalTracer
}

// StartSpan 便捷方法：开始一个新的 span
func StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	return Get().StartSpan(ctx, name, opts...)
}

// Shutdown 关闭全局追踪器
func Shutdown(ctx context.Context) error {
	return Get().Shutdown(ctx)
}

// IsEnabled 检查追踪是否启用
func IsEnabled() bool {
	return Get().IsEnabled()
}

// LangfuseAttributes 提供 Langfuse 特定的属性常量
type LangfuseAttributes struct{}

// Trace 属性
func (LangfuseAttributes) TraceName() string          { return "langfuse.trace.name" }
func (LangfuseAttributes) UserID() string             { return "langfuse.user.id" }
func (LangfuseAttributes) SessionID() string          { return "langfuse.session.id" }
func (LangfuseAttributes) TraceInput() string         { return "langfuse.trace.input" }
func (LangfuseAttributes) TraceOutput() string        { return "langfuse.trace.output" }
func (LangfuseAttributes) TraceTags() string          { return "langfuse.trace.tags" }
func (LangfuseAttributes) ObservationType() string    { return "langfuse.observation.type" }
func (LangfuseAttributes) ObservationModel() string   { return "langfuse.observation.model.name" }
func (LangfuseAttributes) Environment() string        { return "langfuse.environment" }

// LF 预定义的 Langfuse 属性辅助实例
var LF = LangfuseAttributes{}

// BuildAttributes 构建属性 map 的辅助函数
func BuildAttributes(pairs ...string) map[string]any {
	if len(pairs)%2 != 0 {
		panic("BuildAttributes: pairs must be even number of arguments")
	}

	result := make(map[string]any)
	for i := 0; i < len(pairs); i += 2 {
		result[pairs[i]] = pairs[i+1]
	}
	return result
}
