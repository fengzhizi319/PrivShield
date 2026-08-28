// Package ldp 提供本地差分隐私 (Local Differential Privacy) 原语。
//
// 实现二值 Randomized Response、多类别 O-RR (Optimized Randomized Response)
// 与无偏频数估计，所有函数均为零状态纯计算。
package ldp

import (
	"math"
	"math/rand/v2"
)

// ──────────────────────────────────────────────
// 二值 Randomized Response
// ──────────────────────────────────────────────

// RandomizedResponse 对布尔值执行 Randomized Response。
// 以概率 p = e^ε / (1 + e^ε) 返回真实值，以概率 1-p 返回翻转值。
// 满足 ε-本地差分隐私。
func RandomizedResponse(value bool, epsilon float64) bool {
	if epsilon <= 0 {
		return value
	}
	p := math.Exp(epsilon) / (1 + math.Exp(epsilon))
	if rand.Float64() < p {
		return value
	}
	return !value
}

// EstimateTrueCount 从 Randomized Response 结果中估计真实 true 计数。
// 公式：count_true = (n*p - (n - sum)) / (2*p - 1)
func EstimateTrueCount(responses []bool, epsilon float64) int {
	n := len(responses)
	if n == 0 || epsilon <= 0 {
		return 0
	}
	p := math.Exp(epsilon) / (1 + math.Exp(epsilon))
	var sum int
	for _, r := range responses {
		if r {
			sum++
		}
	}
	// 无偏估计
	estimated := (float64(sum) - float64(n)*(1-p)) / (2*p - 1)
	return int(math.Round(estimated))
}

// ──────────────────────────────────────────────
// 多类别 O-RR (Optimized Randomized Response)
// ──────────────────────────────────────────────

// ORRResponse 对离散类别执行 Optimized Randomized Response。
// 以概率 p = e^ε / (e^ε + k - 1) 返回真实值，
// 以概率 1-p 均匀随机返回其他 k-1 个值之一。
// domainSize 为类别总数 k。
func ORRResponse(value int, epsilon float64, domainSize int) int {
	if domainSize <= 1 || epsilon <= 0 {
		return value
	}
	p := math.Exp(epsilon) / (math.Exp(epsilon) + float64(domainSize) - 1)
	if rand.Float64() < p {
		return value
	}
	// 均匀选择其他值
	other := rand.IntN(domainSize - 1)
	if other >= value {
		other++
	}
	return other
}

// EstimateFrequency 从 O-RR 响应中估计各类别频数。
// 返回长度为 domainSize 的切片，第 i 个元素为类别 i 的估计频数。
func EstimateFrequency(responses []int, epsilon float64, domainSize int) []int {
	n := len(responses)
	if n == 0 || domainSize <= 0 {
		return make([]int, domainSize)
	}
	if epsilon <= 0 {
		counts := make([]int, domainSize)
		for _, r := range responses {
			if r >= 0 && r < domainSize {
				counts[r]++
			}
		}
		return counts
	}

	p := math.Exp(epsilon) / (math.Exp(epsilon) + float64(domainSize) - 1)
	q := (1 - p) / float64(domainSize-1)

	// 统计各响应计数
	counts := make([]int, domainSize)
	for _, r := range responses {
		if r >= 0 && r < domainSize {
			counts[r]++
		}
	}

	// 无偏估计：count_i = (n_i - n*q) / (p - q)
	estimated := make([]int, domainSize)
	for i := 0; i < domainSize; i++ {
		est := (float64(counts[i]) - float64(n)*q) / (p - q)
		estimated[i] = int(math.Round(math.Max(0, est)))
	}
	return estimated
}

// ──────────────────────────────────────────────
// 数值型 LDP（基于分段机制）
// ──────────────────────────────────────────────

// NumericLDP 对 [lower, upper] 区间内的数值添加本地差分隐私噪声。
// 使用简化的分段机制：将值归一化至 [0, 1]，添加 Laplace 噪声后截断回区间。
func NumericLDP(value, lower, upper, epsilon float64) float64 {
	if upper <= lower || epsilon <= 0 {
		return value
	}
	// 归一化至 [0, 1]
	normalized := (value - lower) / (upper - lower)
	// 添加 Laplace 噪声（敏感度 = 1）
	noisy := AddLaplaceSimple(normalized, epsilon)
	// 截断回 [0, 1] 并反归一化
	noisy = math.Max(0, math.Min(1, noisy))
	return lower + noisy*(upper-lower)
}

// AddLaplaceSimple 简化版 Laplace 噪声（敏感度 = 1）。
func AddLaplaceSimple(value, epsilon float64) float64 {
	if epsilon <= 0 {
		return value
	}
	scale := 1.0 / epsilon
	u := rand.Float64() - 0.5
	sgn := 1.0
	if u < 0 {
		sgn = -1.0
	}
	noise := -scale * sgn * math.Log(1.0-2.0*math.Abs(u))
	return value + noise
}
