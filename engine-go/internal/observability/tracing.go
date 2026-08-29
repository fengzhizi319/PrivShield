// Package observability — 可选 OpenTelemetry 分布式追踪。
//
// 对齐 Python engine/observability/tracing.py：
// OpenTelemetry 作为可选依赖；未配置时使用 NoOp tracer，零开销。
package observability

import (
	"context"
	"os"
	"sync"
)

// Tracer 抽象追踪器接口。
type Tracer interface {
	// StartSpan 开始一个新的 span 并返回 context 和结束函数。
	StartSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, func())
}

// NoOpTracer 不执行任何操作的追踪器（默认）。
type NoOpTracer struct{}

// StartSpan NoOp 实现。
func (t *NoOpTracer) StartSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, func()) {
	return ctx, func() {}
}

// OTelTracer 包装 OpenTelemetry tracer（当 OTEL 可用时）。
// 当前为预留结构；完整实现需要引入 go.opentelemetry.io/otel 依赖。
type OTelTracer struct {
	Endpoint    string
	ServiceName string
}

// StartSpan OTel 实现（当前降级为 NoOp，待引入 OTEL SDK 后激活）。
func (t *OTelTracer) StartSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, func()) {
	// TODO: 引入 go.opentelemetry.io/otel 后实现真实 span
	return ctx, func() {}
}

var (
	tracer     Tracer
	tracerOnce sync.Once
)

// InitTracing 初始化追踪器。
// endpoint 为空时从 OTEL_EXPORTER_OTLP_ENDPOINT 环境变量读取。
// 未配置时返回 NoOp tracer，零开销。
func InitTracing(endpoint, serviceName string) Tracer {
	tracerOnce.Do(func() {
		if endpoint == "" {
			endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if serviceName == "" {
			serviceName = os.Getenv("PRIVACY_SERVICE_NAME")
			if serviceName == "" {
				serviceName = "PrivShield"
			}
		}

		if endpoint != "" {
			tracer = &OTelTracer{
				Endpoint:    endpoint,
				ServiceName: serviceName,
			}
		} else {
			tracer = &NoOpTracer{}
		}
	})
	return tracer
}

// GetTracer 返回当前追踪器。未初始化时返回 NoOp。
func GetTracer() Tracer {
	if tracer == nil {
		return &NoOpTracer{}
	}
	return tracer
}

// ResetTracing 重置追踪器（仅测试用）。
func ResetTracing() {
	tracerOnce = sync.Once{}
	tracer = nil
}

// StartSpan 便捷函数：开始一个 span。
func StartSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, func()) {
	return GetTracer().StartSpan(ctx, name, attrs)
}

// TracingEnabled 返回是否配置了 OTLP endpoint。
func TracingEnabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
}
