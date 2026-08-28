// GatewayMetrics 网关专属 Prometheus 指标。
//
// 对齐设计文档 §11.1 网关指标规约：
//   - privshield_gateway_backend_in_flight{node_id,backend_addr}
//   - privshield_gateway_backend_ewma_latency_seconds{node_id}
//   - privshield_gateway_circuit_breaker_state{node_id,state}
//   - privshield_gateway_requests_total{node_id,status}
package observability

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// GatewayMetrics 持有网关专属 Prometheus 指标。
type GatewayMetrics struct {
	registry *prometheus.Registry

	// BackendInFlight 各后端节点实时在途并发数。
	BackendInFlight *prometheus.GaugeVec

	// BackendEWMALatency 节点指数移动加权平均延迟（秒）。
	BackendEWMALatency *prometheus.GaugeVec

	// CircuitBreakerState 节点熔断器状态（0=Closed, 1=HalfOpen, 2=Open）。
	CircuitBreakerState *prometheus.GaugeVec

	// RequestsTotal 按 node_id/status 统计网关转发请求数。
	RequestsTotal *prometheus.CounterVec
}

// NewGatewayMetrics 创建并注册网关指标集合。
func NewGatewayMetrics() *GatewayMetrics {
	reg := prometheus.NewRegistry()

	m := &GatewayMetrics{
		registry: reg,

		BackendInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "privshield_gateway_backend_in_flight",
				Help: "Current in-flight requests per backend node.",
			},
			[]string{"node_id", "backend_addr"},
		),

		BackendEWMALatency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "privshield_gateway_backend_ewma_latency_seconds",
				Help: "Exponentially weighted moving average latency per backend node.",
			},
			[]string{"node_id"},
		),

		CircuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "privshield_gateway_circuit_breaker_state",
				Help: "Circuit breaker state per node (0=closed, 1=half_open, 2=open).",
			},
			[]string{"node_id", "state"},
		),

		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "privshield_gateway_requests_total",
				Help: "Total requests forwarded by the gateway.",
			},
			[]string{"node_id", "status"},
		),
	}

	reg.MustRegister(
		m.BackendInFlight,
		m.BackendEWMALatency,
		m.CircuitBreakerState,
		m.RequestsTotal,
	)

	return m
}

// SetBackendInFlight 更新后端在途请求数。
func (m *GatewayMetrics) SetBackendInFlight(nodeID, addr string, count float64) {
	m.BackendInFlight.WithLabelValues(nodeID, addr).Set(count)
}

// SetBackendEWMALatency 更新后端 EWMA 延迟。
func (m *GatewayMetrics) SetBackendEWMALatency(nodeID string, latencySec float64) {
	m.BackendEWMALatency.WithLabelValues(nodeID).Set(latencySec)
}

// SetCircuitBreakerState 更新熔断器状态。
// state: "closed"=0, "half_open"=1, "open"=2
func (m *GatewayMetrics) SetCircuitBreakerState(nodeID, state string) {
	var val float64
	switch state {
	case "closed":
		val = 0
	case "half_open":
		val = 1
	case "open":
		val = 2
	}
	m.CircuitBreakerState.WithLabelValues(nodeID, state).Set(val)
}

// RecordForwarded 记录一次转发。
func (m *GatewayMetrics) RecordForwarded(nodeID string, status int) {
	m.RequestsTotal.WithLabelValues(nodeID, strconv.Itoa(status)).Inc()
}

// PrometheusMiddleware 返回网关 HTTP 请求指标中间件。
func (m *GatewayMetrics) PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅记录经过网关的请求（不记录 /health、/metrics 等本地端点）
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" || path == "/gateway/backends" {
			c.Next()
			return
		}
		c.Next()
		// 记录到默认节点（实际 node_id 由代理层 RecordForwarded 精确上报）
		m.RequestsTotal.WithLabelValues("aggregate", strconv.Itoa(c.Writer.Status())).Inc()
	}
}

// Handler 返回暴露 /metrics 端点的 Gin handler。
func (m *GatewayMetrics) Handler() gin.HandlerFunc {
	h := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
