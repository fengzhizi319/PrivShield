// Package gateway 提供 HTTP 反向代理。
//
// 集成 GatewayMetrics 实时上报 InFlight/EWMA/熔断器状态到 Prometheus。
// 错误响应使用 pkg/middleware 统一信封格式。
package gateway

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/observability"
	"github.com/fengzhizi319/PrivShield/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// byteBufferPool 实现 httputil.BufferPool 接口，复用 32KB 读写缓冲区
type byteBufferPool struct {
	pool sync.Pool
}

func newByteBufferPool() *byteBufferPool {
	return &byteBufferPool{
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, 32*1024)
				return &b
			},
		},
	}
}

func (p *byteBufferPool) Get() []byte {
	return *p.pool.Get().(*[]byte)
}

func (p *byteBufferPool) Put(b []byte) {
	if cap(b) >= 32*1024 {
		p.pool.Put(&b)
	}
}

var (
	globalBufferPool = newByteBufferPool()
	sharedTransport  = &http.Transport{
		MaxIdleConns:        2048,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}
)

// ReverseProxy 返回绑定在本节点上的反向代理实例。
//
// 首次调用时惰性构建并经 sync.Once 固化，后续请求直接复用；实例生命周期与
// BackendNode 完全一致——节点释放即实例释放，因此不再需要全局 sync.Map 缓存、
// TTL 过期扫描与后台清理 goroutine（也无 Stop 钩子泄漏风险）。
// 连接复用不受影响：所有实例共享同一个 sharedTransport 连接池。
// metrics 仅用于 ErrorHandler 上报，取首次构建时传入的值；进程内只有一份 GatewayMetrics。
func (n *BackendNode) ReverseProxy(metrics *observability.GatewayMetrics) (*httputil.ReverseProxy, error) {
	n.proxyOnce.Do(func() {
		target, err := url.Parse(fmt.Sprintf("http://%s", n.Address))
		if err != nil {
			// 地址源于启动时静态配置，解析失败为永久错误，直接缓存错误避免重复构建
			n.proxyErr = err
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = sharedTransport
		proxy.BufferPool = globalBufferPool
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			n.CB.RecordFailure()
			if metrics != nil {
				metrics.SetCircuitBreakerState(n.Address, cbStateString(n.CB.State()))
				metrics.RecordForwarded(n.Address, http.StatusBadGateway)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"code":"BAD_GATEWAY","message":"后端 %s 不可达","detail":"%s","trace_id":"","timestamp":"%s"}`, n.Address, err.Error(), time.Now().UTC().Format(time.RFC3339Nano))
		}
		n.proxy = proxy
	})
	return n.proxy, n.proxyErr
}

// NewHTTPProxyHandler 创建 HTTP 反向代理处理器。
// metrics 可为 nil，为 nil 时不上报 Prometheus 指标。
func NewHTTPProxyHandler(lb *LoadBalancer, metrics *observability.GatewayMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := lb.SelectNode()
		if node == nil {
			middleware.AbortWithError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "无可用后端节点", "all backends exhausted")
			return
		}

		// 检查熔断器
		if !node.CB.Allow() {
			if metrics != nil {
				metrics.SetCircuitBreakerState(node.Address, cbStateString(node.CB.State()))
			}
			middleware.AbortWithError(c, http.StatusServiceUnavailable, "CIRCUIT_OPEN", fmt.Sprintf("后端 %s 熔断器开启", node.Address), "circuit breaker is open")
			return
		}

		node.IncrementInFlight()
		defer node.DecrementInFlight()

		// 上报 InFlight 指标
		if metrics != nil {
			metrics.SetBackendInFlight(node.Address, node.Address, float64(node.InFlight.Load()))
		}

		// 获取节点内聚的反向代理（内置 BufferPool 与共享长连接池）
		proxy, err := node.ReverseProxy(metrics)
		if err != nil {
			node.CB.RecordFailure()
			if metrics != nil {
				metrics.SetCircuitBreakerState(node.Address, cbStateString(node.CB.State()))
			}
			middleware.AbortWithError(c, http.StatusInternalServerError, "PROXY_ERROR", "后端代理创建失败", err.Error())
			return
		}

		// 记录延迟
		start := time.Now()
		proxy.ServeHTTP(c.Writer, c.Request)
		latency := time.Since(start)

		// 更新 EWMA（alpha=0.3）
		node.UpdateEWMA(latency, 0.3)

		// 根据响应状态更新熔断器
		if c.Writer.Status() < 500 {
			node.CB.RecordSuccess()
		} else {
			node.CB.RecordFailure()
		}

		// 上报 Prometheus 指标
		if metrics != nil {
			metrics.SetBackendEWMALatency(node.Address, float64(latency.Seconds()))
			metrics.SetCircuitBreakerState(node.Address, cbStateString(node.CB.State()))
			metrics.SetBackendInFlight(node.Address, node.Address, float64(node.InFlight.Load()))
			metrics.RecordForwarded(node.Address, c.Writer.Status())
		}
	}
}

// NewHealthCheckHandler 创建健康检查代理
func NewHealthCheckHandler(lb *LoadBalancer) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodes := lb.Nodes()
		results := make([]gin.H, 0, len(nodes))
		for _, n := range nodes {
			state := cbStateString(n.CB.State())
			results = append(results, gin.H{
				"address":   n.Address,
				"in_flight": n.InFlight.Load(),
				"ewma_ms":   n.GetEWMA() / 1e6,
				"cb_state":  state,
			})
		}
		c.JSON(http.StatusOK, gin.H{"backends": results})
	}
}

// cbStateString 将熔断器状态枚举转为可读字符串。
func cbStateString(s CBState) string {
	switch s {
	case CBClosed:
		return "closed"
	case CBHalfOpen:
		return "half_open"
	case CBOpen:
		return "open"
	}
	return "unknown"
}
