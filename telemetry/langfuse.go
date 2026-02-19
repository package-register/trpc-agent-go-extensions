package telemetry

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	atrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
	"trpc.group/trpc-go/trpc-agent-go/telemetry/langfuse"
)

// langfuseTracer Langfuse 追踪器实现
type langfuseTracer struct {
	cleanup  func(ctx context.Context) error
	enabled  bool
	initOnce sync.Once
	initErr  error
}

// langfuseSpan Langfuse span 包装
type langfuseSpan struct {
	span trace.Span
}

// NewLangfuse 创建 Langfuse 追踪器
// 参数：
//   - secretKey: Langfuse 密钥
//   - publicKey: Langfuse 公钥
//   - host: Langfuse 服务器地址
//   - insecure: 是否使用 HTTP（本地开发）
func NewLangfuse(secretKey, publicKey, host string, insecure bool) Tracer {
	return &langfuseTracer{
		enabled: secretKey != "" && publicKey != "",
	}
}

// StartSpan 开始一个新的 span
func (t *langfuseTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	if !t.enabled {
		return ctx, &noopSpan{}
	}

	// 延迟初始化（首次调用时）
	if err := t.lazyInit(ctx); err != nil {
		// 初始化失败，返回 noop
		return ctx, &noopSpan{}
	}

	// 构建属性
	var attrs []attribute.KeyValue
	cfg := &SpanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	for k, v := range cfg.Attributes {
		attrs = append(attrs, attribute.String(k, fmt.Sprintf("%v", v)))
	}

	// 创建 span
	ctx, span := atrace.Tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, &langfuseSpan{span: span}
}

// Shutdown 关闭追踪器
func (t *langfuseTracer) Shutdown(ctx context.Context) error {
	if t.cleanup != nil {
		return t.cleanup(ctx)
	}
	return nil
}

// IsEnabled 检查是否启用
func (t *langfuseTracer) IsEnabled() bool {
	return t.enabled
}

// lazyInit 延迟初始化 Langfuse
func (t *langfuseTracer) lazyInit(ctx context.Context) error {
	t.initOnce.Do(func() {
		// 这里需要从配置读取实际参数
		// 为了解耦，我们使用环境变量或全局配置
		// 具体实现会在初始化时注入
	})
	return t.initErr
}

// SetAttributes 设置属性
func (s *langfuseSpan) SetAttributes(attrs ...Attribute) {
	if s.span == nil {
		return
	}

	var otelAttrs []attribute.KeyValue
	for _, attr := range attrs {
		otelAttrs = append(otelAttrs, attribute.String(attr.Key, fmt.Sprintf("%v", attr.Value)))
	}
	s.span.SetAttributes(otelAttrs...)
}

// SetStatus 设置状态
func (s *langfuseSpan) SetStatus(status Status, description string) {
	// OpenTelemetry 的状态设置较为复杂，这里简化处理
	if s.span != nil && status.Code == StatusError.Code {
		s.span.SetAttributes(attribute.String("error", description))
	}
}

// RecordError 记录错误
func (s *langfuseSpan) RecordError(err error) {
	if s.span == nil {
		return
	}
	s.span.SetAttributes(attribute.String("error.message", err.Error()))
}

// End 结束 span
func (s *langfuseSpan) End() {
	if s.span != nil {
		s.span.End()
	}
}

// InitLangfuse 初始化 Langfuse（由 main.go 调用）
// 这是启动 Langfuse 的辅助函数
func InitLangfuse(ctx context.Context, secretKey, publicKey, host string, insecure bool) (func(context.Context) error, error) {
	if secretKey == "" || publicKey == "" {
		return nil, fmt.Errorf("langfuse credentials not provided")
	}

	opts := []langfuse.Option{
		langfuse.WithSecretKey(secretKey),
		langfuse.WithPublicKey(publicKey),
		langfuse.WithHost(host),
	}

	if insecure {
		opts = append(opts, langfuse.WithInsecure())
	}

	clean, err := langfuse.Start(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to start langfuse: %w", err)
	}

	return clean, nil
}
