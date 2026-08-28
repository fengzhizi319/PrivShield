// Package dp 提供纯标量浮点差分隐私原语。
//
// 实现 Laplace / Gaussian 机制、自适应梯度截断与向量加噪，
// 所有函数均为零状态纯计算，适合高并发场景。
package dp

import (
	"math"
	"math/rand/v2"
)

// ──────────────────────────────────────────────
// Laplace 机制（ε-DP，适用于 Count / Sum）
// ──────────────────────────────────────────────

// AddLaplaceNoise 为数值添加 Laplace 噪声。
// scale = sensitivity / epsilon，满足 ε-差分隐私。
// epsilon <= 0 时直接返回原值（无隐私保护）。
func AddLaplaceNoise(value, epsilon, sensitivity float64) float64 {
	if epsilon <= 0 || sensitivity <= 0 {
		return value
	}
	scale := sensitivity / epsilon
	u := rand.Float64() - 0.5
	sgn := 1.0
	if u < 0 {
		sgn = -1.0
	}
	noise := -scale * sgn * math.Log(1.0-2.0*math.Abs(u))
	return value + noise
}

// ──────────────────────────────────────────────
// Gaussian 机制（(ε,δ)-DP，适用于 Mean / Sum）
// ──────────────────────────────────────────────

// AddGaussianNoise 为数值添加 Gaussian 噪声。
// sigma = sqrt(2 * ln(1.25/delta)) * sensitivity / epsilon，
// 满足 (ε,δ)-差分隐私。
func AddGaussianNoise(value, epsilon, delta, sensitivity float64) float64 {
	if epsilon <= 0 || delta <= 0 || sensitivity <= 0 {
		return value
	}
	sigma := math.Sqrt(2.0*math.Log(1.25/delta)) * sensitivity / epsilon
	return value + boxMullerNormal()*sigma
}

// boxMullerNormal 使用 Box-Muller 变换生成标准正态分布随机数。
func boxMullerNormal() float64 {
	u1 := rand.Float64()
	u2 := rand.Float64()
	// 避免 log(0)
	for u1 == 0 {
		u1 = rand.Float64()
	}
	return math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
}

// ──────────────────────────────────────────────
// 自适应梯度截断（Clipping）
// ──────────────────────────────────────────────

// ClipValue 将数值截断至 [-bound, +bound] 区间。
func ClipValue(value, bound float64) float64 {
	if value > bound {
		return bound
	}
	if value < -bound {
		return -bound
	}
	return value
}

// ClipL2Norm 将向量截断至 L2 范数不超过 maxNorm。
// 返回截断后的新切片（不修改原切片）。
func ClipL2Norm(vec []float64, maxNorm float64) []float64 {
	if maxNorm <= 0 || len(vec) == 0 {
		return vec
	}
	var sumSq float64
	for _, v := range vec {
		sumSq += v * v
	}
	norm := math.Sqrt(sumSq)
	if norm <= maxNorm {
		result := make([]float64, len(vec))
		copy(result, vec)
		return result
	}
	scale := maxNorm / norm
	result := make([]float64, len(vec))
	for i, v := range vec {
		result[i] = v * scale
	}
	return result
}

// ──────────────────────────────────────────────
// 向量加噪
// ──────────────────────────────────────────────

// AddLaplaceVector 为向量每个分量独立添加 Laplace 噪声。
func AddLaplaceVector(vec []float64, epsilon, sensitivity float64) []float64 {
	result := make([]float64, len(vec))
	for i, v := range vec {
		result[i] = AddLaplaceNoise(v, epsilon, sensitivity)
	}
	return result
}

// AddGaussianVector 为向量每个分量独立添加 Gaussian 噪声。
func AddGaussianVector(vec []float64, epsilon, delta, sensitivity float64) []float64 {
	result := make([]float64, len(vec))
	for i, v := range vec {
		result[i] = AddGaussianNoise(v, epsilon, delta, sensitivity)
	}
	return result
}

// ──────────────────────────────────────────────
// 统计聚合 + 加噪
// ──────────────────────────────────────────────

// NoisyCount 计算计数并添加 Laplace 噪声（敏感度 = 1）。
func NoisyCount(count int, epsilon float64) float64 {
	return AddLaplaceNoise(float64(count), epsilon, 1.0)
}

// NoisySum 计算总和并添加 Laplace 噪声。
func NoisySum(values []float64, epsilon, sensitivity float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return AddLaplaceNoise(sum, epsilon, sensitivity)
}

// NoisyMean 计算均值并添加 Gaussian 噪声。
// 先对每个值截断至 [-clipBound, +clipBound]，再计算均值并加噪。
func NoisyMean(values []float64, epsilon, delta, clipBound float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += ClipValue(v, clipBound)
	}
	mean := sum / float64(len(values))
	// 均值的敏感度 = clipBound / n
	sensitivity := clipBound / float64(len(values))
	return AddGaussianNoise(mean, epsilon, delta, sensitivity)
}

// ──────────────────────────────────────────────
// 直方图与向量均值（对齐 Python DPApi）
// ──────────────────────────────────────────────

// NoisyHistogram 计算带噪声的分类直方图。
// trueCounts 为各分类的真实计数（map[分类]计数），
// epsilon 为隐私预算。返回各分类的带噪声计数。
// 每个分类独立添加 Laplace 噪声（敏感度 = 1）。
func NoisyHistogram(trueCounts map[string]int, epsilon float64) map[string]float64 {
	result := make(map[string]float64, len(trueCounts))
	for k, v := range trueCounts {
		result[k] = NoisyCount(v, epsilon)
	}
	return result
}

// VectorMean 计算向量均值并添加 Laplace 向量噪声。
// 先对每个向量截断 L2 范数至 maxNorm，再计算均值，
// 最后为均值向量每个分量添加 Laplace 噪声。
// 返回带噪声的均值向量。
func VectorMean(vectors [][]float64, maxNorm float64, epsilon float64) []float64 {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	if dim == 0 {
		return nil
	}

	// 1. 截断每个向量的 L2 范数
	clipped := make([][]float64, len(vectors))
	for i, v := range vectors {
		clipped[i] = ClipL2Norm(v, maxNorm)
	}

	// 2. 计算截断后的均值
	sum := make([]float64, dim)
	for _, v := range clipped {
		for j := 0; j < dim && j < len(v); j++ {
			sum[j] += v[j]
		}
	}
	n := float64(len(vectors))
	mean := make([]float64, dim)
	for j := 0; j < dim; j++ {
		mean[j] = sum[j] / n
	}

	// 3. 为均值向量添加 Laplace 噪声
	// 敏感度 = maxNorm / n（每个分量的敏感度）
	sensitivity := maxNorm / n
	return AddLaplaceVector(mean, epsilon, sensitivity)
}
