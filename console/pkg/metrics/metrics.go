// Package metrics provides shared Prometheus metrics for console Go modules.
// Package metrics 为控制台各 Go 模块提供共享的 Prometheus 指标定义与 /metrics 端点。
//
// 每个模块在启动时调用 NewCollector(module) 创建带模块标签的指标收集器，
// 再通过 gin handler 暴露 GET /metrics 供 Prometheus 抓取。
//
// 每个 Collector 使用独立的 prometheus.Registry，避免全局注册冲突（测试/多实例场景）。
package metrics

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector holds module-scoped Prometheus metrics.
// Collector 持有模块级别的 Prometheus 指标。
type Collector struct {
	module   string
	registry *prometheus.Registry

	// HTTPRequestsTotal counts HTTP requests by method/path/status.
	// HTTPRequestsTotal 按 method/path/status 统计 HTTP 请求数。
	HTTPRequestsTotal *prometheus.CounterVec

	// HTTPRequestDuration records HTTP request latency histogram.
	// HTTPRequestDuration 记录 HTTP 请求延迟直方图。
	HTTPRequestDuration *prometheus.HistogramVec

	// AgentRequestsTotal counts upstream agent calls by endpoint/status.
	// AgentRequestsTotal 按 endpoint/status 统计上游 agent 调用数。
	AgentRequestsTotal *prometheus.CounterVec

	// AgentRequestDuration records upstream agent call latency.
	// AgentRequestDuration 记录上游 agent 调用延迟。
	AgentRequestDuration *prometheus.HistogramVec
}

// NewCollector creates and registers a new metrics collector for the given module.
// Each collector uses its own prometheus.Registry to avoid global registration conflicts.
// NewCollector 为指定模块创建并注册新的指标收集器。
// 每个收集器使用独立的 prometheus.Registry，避免全局注册冲突。
func NewCollector(module string) *Collector {
	reg := prometheus.NewRegistry()

	c := &Collector{
		module:   module,
		registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "http_requests_total",
				Help:        "Total HTTP requests processed.",
				ConstLabels: prometheus.Labels{"module": module},
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "http_request_duration_seconds",
				Help:        "HTTP request latency in seconds.",
				ConstLabels: prometheus.Labels{"module": module},
				Buckets:     prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		AgentRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "agent_requests_total",
				Help:        "Total upstream agent requests.",
				ConstLabels: prometheus.Labels{"module": module},
			},
			[]string{"endpoint", "status"},
		),
		AgentRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "agent_request_duration_seconds",
				Help:        "Upstream agent request latency in seconds.",
				ConstLabels: prometheus.Labels{"module": module},
				Buckets:     prometheus.DefBuckets,
			},
			[]string{"endpoint"},
		),
	}

	reg.MustRegister(
		c.HTTPRequestsTotal,
		c.HTTPRequestDuration,
		c.AgentRequestsTotal,
		c.AgentRequestDuration,
	)

	return c
}

// RecordHTTP records an HTTP request metric.
// RecordHTTP 记录一次 HTTP 请求指标。
func (c *Collector) RecordHTTP(method, path string, status int, durationSec float64) {
	statusStr := strconv.Itoa(status)
	c.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	c.HTTPRequestDuration.WithLabelValues(method, path).Observe(durationSec)
}

// RecordAgentCall records an upstream agent call metric.
// RecordAgentCall 记录一次上游 agent 调用指标。
func (c *Collector) RecordAgentCall(endpoint string, status string, durationSec float64) {
	c.AgentRequestsTotal.WithLabelValues(endpoint, status).Inc()
	c.AgentRequestDuration.WithLabelValues(endpoint).Observe(durationSec)
}

// Handler returns a Gin handler that serves Prometheus /metrics endpoint
// using this collector's custom registry.
// Handler 返回暴露 Prometheus /metrics 端点的 Gin handler，使用本收集器的自定义注册表。
func (c *Collector) Handler() gin.HandlerFunc {
	h := promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
	return func(ctx *gin.Context) {
		h.ServeHTTP(ctx.Writer, ctx.Request)
	}
}
