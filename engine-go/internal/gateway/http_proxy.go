// Package gateway 提供 HTTP 反向代理。
package gateway

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// NewHTTPProxyHandler 创建 HTTP 反向代理处理器
func NewHTTPProxyHandler(lb *LoadBalancer) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := lb.SelectNode()
		if node == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    "SERVICE_UNAVAILABLE",
				"message": "no available backend",
			})
			return
		}

		// 检查熔断器
		if !node.CB.Allow() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    "CIRCUIT_OPEN",
				"message": fmt.Sprintf("backend %s circuit breaker open", node.Address),
			})
			return
		}

		node.IncrementInFlight()
		defer node.DecrementInFlight()

		// 创建反向代理
		target, err := url.Parse(fmt.Sprintf("http://%s", node.Address))
		if err != nil {
			node.CB.RecordFailure()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "PROXY_ERROR",
				"message": err.Error(),
			})
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			node.CB.RecordFailure()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"code":"BAD_GATEWAY","message":"%s","detail":"backend %s unreachable"}`, err.Error(), node.Address)
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
	}
}

// NewHealthCheckHandler 创建健康检查代理
func NewHealthCheckHandler(lb *LoadBalancer) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodes := lb.Nodes()
		results := make([]gin.H, 0, len(nodes))
		for _, n := range nodes {
			state := "closed"
			switch n.CB.State() {
			case CBHalfOpen:
				state = "half_open"
			case CBOpen:
				state = "open"
			}
			results = append(results, gin.H{
				"address":   n.Address,
				"in_flight": n.InFlight,
				"ewma_ms":   n.EWMA / 1e6,
				"cb_state":  state,
			})
		}
		c.JSON(http.StatusOK, gin.H{"backends": results})
	}
}
