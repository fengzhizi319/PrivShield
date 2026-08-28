// Package observability — engine-go Prometheus 指标定义与 /metrics 端点。
//
// 对齐设计文档 §11.1 指标规约与 pkg/metrics 命名规范：
//   - 自定义指标使用 privshield_ 前缀
//   - Counter 以 _total 结尾
//   - Histogram 以 _duration_seconds 结尾
//
// 指标清单（对齐 Python engine/observability/metrics.py）：
//   - privshield_requests_total{protocol,endpoint,status}
//   - privshield_request_duration_seconds{protocol,endpoint}
//   - privshield_classification_total{engine,level,domain}
//   - privshield_budget_consumed_total{namespace,mechanism}
//   - privshield_ner_inference_seconds{device,batch_size}
package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EngineMetrics 持有 engine-go 全部 Prometheus 指标。
type EngineMetrics struct {
	registry *prometheus.Registry

	// RequestsTotal 按 protocol/endpoint/status 统计请求总数。
	// 对齐 §11.1 privacy_requests_total。
	RequestsTotal *prometheus.CounterVec

	// RequestDuration 按 protocol/endpoint 记录请求延迟直方图。
	// 对齐 §11.1 privacy_request_duration_seconds。
	RequestDuration *prometheus.HistogramVec

	// ClassificationTotal 按 engine/level/domain 统计分类命中数。
	// 对齐 §11.1 privacy_classification_total。
	ClassificationTotal *prometheus.CounterVec

	// BudgetConsumedTotal 按 namespace/mechanism 统计 DP 预算消耗。
	// 对齐 §11.1 privacy_budget_consumed_total。
	BudgetConsumedTotal *prometheus.CounterVec

	// NerInferenceSeconds GPU/CPU NER 推理耗时直方图。
	// 对齐 §11.1 privacy_ner_gpu_inference_seconds（扩展支持 CPU 设备）。
	NerInferenceSeconds *prometheus.HistogramVec
}

// NewEngineMetrics 创建并注册 engine-go 指标集合。
// 使用独立 Registry 避免全局注册冲突。
func NewEngineMetrics() *EngineMetrics {
	reg := prometheus.NewRegistry()

	m := &EngineMetrics{
		registry: reg,

		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "privshield_requests_total",
				Help: "Total requests processed by the privacy engine.",
			},
			[]string{"protocol", "endpoint", "status"},
		),

		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "privshield_request_duration_seconds",
				Help:    "Request latency histogram for privacy engine endpoints.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"protocol", "endpoint"},
		),

		ClassificationTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "privshield_classification_total",
				Help: "Classification funnel hits by engine/level/domain.",
			},
			[]string{"engine", "level", "domain"},
		),

		BudgetConsumedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "privshield_budget_consumed_total",
				Help: "Cumulative differential privacy budget consumed.",
			},
			[]string{"namespace", "mechanism"},
		),

		NerInferenceSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "privshield_ner_inference_seconds",
				Help:    "NER inference latency by device and batch size.",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"device", "batch_size"},
		),
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.ClassificationTotal,
		m.BudgetConsumedTotal,
		m.NerInferenceSeconds,
	)

	return m
}

// RecordRequest 记录一次请求指标。
func (m *EngineMetrics) RecordRequest(protocol, endpoint string, status int, durationSec float64) {
	statusStr := strconv.Itoa(status)
	m.RequestsTotal.WithLabelValues(protocol, endpoint, statusStr).Inc()
	m.RequestDuration.WithLabelValues(protocol, endpoint).Observe(durationSec)
}

// RecordClassification 记录一次分类命中。
func (m *EngineMetrics) RecordClassification(engine, level, domain string) {
	m.ClassificationTotal.WithLabelValues(engine, level, domain).Inc()
}

// RecordBudgetConsumed 记录一次 DP 预算消耗。
func (m *EngineMetrics) RecordBudgetConsumed(namespace, mechanism string) {
	m.BudgetConsumedTotal.WithLabelValues(namespace, mechanism).Inc()
}

// RecordNerInference 记录一次 NER 推理耗时。
func (m *EngineMetrics) RecordNerInference(device string, batchSize int, durationSec float64) {
	m.NerInferenceSeconds.WithLabelValues(device, strconv.Itoa(batchSize)).Observe(durationSec)
}

// PrometheusMiddleware 返回自动记录 HTTP 请求指标的 Gin 中间件。
// 替代原有 TODO 桩，实际写入 Prometheus Counter + Histogram。
func (m *EngineMetrics) PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		// 跳过 /metrics 自身避免递归
		if path == "/metrics" {
			return
		}

		duration := time.Since(start).Seconds()
		m.RecordRequest("http", path, c.Writer.Status(), duration)
	}
}

// Handler 返回暴露 /metrics 端点的 Gin handler。
func (m *EngineMetrics) Handler() gin.HandlerFunc {
	h := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
