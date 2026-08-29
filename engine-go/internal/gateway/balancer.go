// Package gateway 提供 L7 自适应负载均衡网关。
//
// 实现 P2C-EWMA 调度、三态熔断器、HTTP 反向代理与 gRPC 透明转发。
package gateway

import (
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// ──────────────────────────────────────────────
// 后端节点
// ──────────────────────────────────────────────

// BackendNode 后端节点
type BackendNode struct {
	Address       string
	Weight        int
	currentWeight atomic.Int32   // Nginx SWRR 当前权重（原子操作）
	InFlight      atomic.Int64   // 当前在途请求数（原子操作，与 EWMA 锁分离）
	EWMA          float64        // 指数移动加权平均延迟
	LastUsed      time.Time      // 最后使用时间
	CB            CircuitBreaker // 熔断器
	eWMAMu        sync.Mutex     // 仅保护 EWMA 字段
}

// ──────────────────────────────────────────────
// 三态熔断器
// ──────────────────────────────────────────────

// CBState 熔断器状态
type CBState int

const (
	CBClosed   CBState = iota // 正常
	CBHalfOpen                // 半开（探测）
	CBOpen                    // 熔断
)

// CircuitBreaker 三态熔断器
type CircuitBreaker struct {
	state        CBState
	failureCount int
	successCount int
	threshold    int           // 触发熔断的失败次数
	halfOpenMax  int           // 半开状态最大探测次数
	lastFailure  time.Time     // 最近失败时间
	cooldown     time.Duration // 冷却时间
	mu           sync.Mutex
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(threshold int, cooldown time.Duration) CircuitBreaker {
	return CircuitBreaker{
		state:       CBClosed,
		threshold:   threshold,
		halfOpenMax: 3,
		cooldown:    cooldown,
	}
}

// Allow 检查是否允许请求通过
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CBClosed:
		return true
	case CBOpen:
		// 检查冷却期是否已过
		if time.Since(cb.lastFailure) > cb.cooldown {
			cb.state = CBHalfOpen
			cb.successCount = 0
			return true
		}
		return false
	case CBHalfOpen:
		return cb.successCount < cb.halfOpenMax
	}
	return true
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CBHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.halfOpenMax {
			cb.state = CBClosed
			cb.failureCount = 0
		}
	case CBClosed:
		cb.failureCount = 0
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailure = time.Now()

	switch cb.state {
	case CBClosed:
		if cb.failureCount >= cb.threshold {
			cb.state = CBOpen
		}
	case CBHalfOpen:
		cb.state = CBOpen
	}
}

// State 返回当前状态
func (cb *CircuitBreaker) State() CBState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// ──────────────────────────────────────────────
// 负载均衡器
// ──────────────────────────────────────────────

// LoadBalancer 自适应负载均衡器
type LoadBalancer struct {
	nodes    []*BackendNode
	strategy string       // "p2c" | "round_robin" | "least_conn" | "weighted_rr" | "weighted_random"
	rrIndex  atomic.Int32 // round-robin 原子计数器（无锁化）
}

// NewLoadBalancer 创建负载均衡器
func NewLoadBalancer(addresses []string, strategy string) *LoadBalancer {
	nodes := make([]*BackendNode, len(addresses))
	for i, addr := range addresses {
		nodes[i] = &BackendNode{
			Address: addr,
			Weight:  1,
			CB:      NewCircuitBreaker(5, 30*time.Second),
		}
	}
	return &LoadBalancer{
		nodes:    nodes,
		strategy: strategy,
	}
}

// NewWeightedLoadBalancer 创建支持权重的负载均衡器。
// weights 与 addresses 一一对应，值越大分配流量越多。
func NewWeightedLoadBalancer(addresses []string, weights []int, strategy string) *LoadBalancer {
	nodes := make([]*BackendNode, len(addresses))
	for i, addr := range addresses {
		w := 1
		if i < len(weights) && weights[i] > 0 {
			w = weights[i]
		}
		nodes[i] = &BackendNode{
			Address: addr,
			Weight:  w,
			CB:      NewCircuitBreaker(5, 30*time.Second),
		}
	}
	return &LoadBalancer{
		nodes:    nodes,
		strategy: strategy,
	}
}

// SelectNode 选择一个后端节点（无全局锁，各策略独立无锁化）
func (lb *LoadBalancer) SelectNode() *BackendNode {
	switch lb.strategy {
	case "p2c":
		return lb.selectP2C()
	case "round_robin":
		return lb.selectRoundRobin()
	case "least_conn":
		return lb.selectLeastConn()
	case "weighted_rr":
		return lb.selectWeightedRoundRobin()
	case "weighted_random":
		return lb.selectWeightedRandom()
	default:
		return lb.selectP2C()
	}
}

// selectP2C 幂律双选 (Power of Two Choices) + EWMA 延迟
func (lb *LoadBalancer) selectP2C() *BackendNode {
	if len(lb.nodes) == 0 {
		return nil
	}

	// 收集可用节点
	available := make([]*BackendNode, 0, len(lb.nodes))
	for _, n := range lb.nodes {
		if n.CB.Allow() {
			available = append(available, n)
		}
	}
	if len(available) == 0 {
		// 全部熔断，返回第一个节点供调用方执行熔断降级与指标上报
		return lb.nodes[0]
	}
	if len(available) == 1 {
		return available[0]
	}

	// 随机选两个
	i := rand.IntN(len(available))
	j := rand.IntN(len(available))
	for j == i {
		j = rand.IntN(len(available))
	}

	a, b := available[i], available[j]
	// 选择负载较低的（在途请求 * EWMA 延迟）
	scoreA := float64(a.InFlight.Load()+1) * math.Max(a.EWMA, 0.001)
	scoreB := float64(b.InFlight.Load()+1) * math.Max(b.EWMA, 0.001)

	if scoreA <= scoreB {
		return a
	}
	return b
}

// selectRoundRobin 无锁轮询（atomic fetch-and-add）
func (lb *LoadBalancer) selectRoundRobin() *BackendNode {
	available := make([]*BackendNode, 0, len(lb.nodes))
	for _, n := range lb.nodes {
		if n.CB.Allow() {
			available = append(available, n)
		}
	}
	if len(available) == 0 {
		return lb.nodes[0]
	}
	idx := int(lb.rrIndex.Add(1)-1) % len(available)
	return available[idx]
}

// selectLeastConn 最少连接（原子读取 InFlight）
func (lb *LoadBalancer) selectLeastConn() *BackendNode {
	var best *BackendNode
	bestInFlight := int64(math.MaxInt64)
	for _, n := range lb.nodes {
		if !n.CB.Allow() {
			continue
		}
		inFlight := n.InFlight.Load()
		if inFlight < bestInFlight {
			bestInFlight = inFlight
			best = n
		}
	}
	if best == nil {
		return lb.nodes[0]
	}
	return best
}

// selectWeightedRoundRobin Nginx 平滑加权轮询 (SWRR)。
//
// 算法：每轮所有节点 currentWeight += weight；
// 选取 currentWeight 最大的节点；
// 被选中节点 currentWeight -= totalWeight。
// 保证分配比例精确且分布均匀（不会出现连续集中分配到同一节点）。
func (lb *LoadBalancer) selectWeightedRoundRobin() *BackendNode {
	available := make([]*BackendNode, 0, len(lb.nodes))
	for _, n := range lb.nodes {
		if n.CB.Allow() {
			available = append(available, n)
		}
	}
	if len(available) == 0 {
		return lb.nodes[0]
	}

	totalWeight := int32(0)
	var best *BackendNode
	bestCW := int32(-1 << 31) // min int32
	for _, n := range available {
		cw := n.currentWeight.Add(int32(n.Weight))
		totalWeight += int32(n.Weight)
		if cw > bestCW {
			bestCW = cw
			best = n
		}
	}
	best.currentWeight.Add(-totalWeight)
	return best
}

// selectWeightedRandom 加权随机选择。
//
// 每个节点的选中概率与其 Weight 成正比。
func (lb *LoadBalancer) selectWeightedRandom() *BackendNode {
	available := make([]*BackendNode, 0, len(lb.nodes))
	for _, n := range lb.nodes {
		if n.CB.Allow() {
			available = append(available, n)
		}
	}
	if len(available) == 0 {
		return lb.nodes[0]
	}
	if len(available) == 1 {
		return available[0]
	}

	totalWeight := 0
	for _, n := range available {
		totalWeight += n.Weight
	}
	r := rand.IntN(totalWeight)
	cumulative := 0
	for _, n := range available {
		cumulative += n.Weight
		if r < cumulative {
			return n
		}
	}
	return available[len(available)-1]
}

// UpdateEWMA 更新节点 EWMA 延迟（独立 eWMAMu，不与 InFlight 竞争）
func (n *BackendNode) UpdateEWMA(latency time.Duration, alpha float64) {
	n.eWMAMu.Lock()
	defer n.eWMAMu.Unlock()
	n.EWMA = alpha*float64(latency) + (1-alpha)*n.EWMA
	n.LastUsed = time.Now()
}

// IncrementInFlight 增加在途请求数（原子操作）
func (n *BackendNode) IncrementInFlight() {
	n.InFlight.Add(1)
}

// DecrementInFlight 减少在途请求数（原子操作）
func (n *BackendNode) DecrementInFlight() {
	for {
		old := n.InFlight.Load()
		if old <= 0 {
			return
		}
		if n.InFlight.CompareAndSwap(old, old-1) {
			return
		}
	}
}

// Nodes 返回所有节点
func (lb *LoadBalancer) Nodes() []*BackendNode {
	return lb.nodes
}
