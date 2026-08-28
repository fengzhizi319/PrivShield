package dp

import (
	"math"
	"testing"
)

func TestAddLaplaceNoise(t *testing.T) {
	value := 100.0
	epsilon := 1.0
	sensitivity := 1.0

	// 多次采样验证噪声存在
	var results []float64
	for i := 0; i < 100; i++ {
		noisy := AddLaplaceNoise(value, epsilon, sensitivity)
		results = append(results, noisy)
	}

	// 验证噪声非零
	var sum float64
	for _, v := range results {
		sum += v
	}
	mean := sum / float64(len(results))
	if math.Abs(mean-value) > 5.0 {
		t.Errorf("Laplace noise mean deviation too large: %f", mean-value)
	}
}

func TestAddGaussianNoise(t *testing.T) {
	value := 100.0
	epsilon := 1.0
	delta := 1e-5
	sensitivity := 1.0

	// 多次采样验证噪声存在
	var results []float64
	for i := 0; i < 100; i++ {
		noisy := AddGaussianNoise(value, epsilon, delta, sensitivity)
		results = append(results, noisy)
	}

	// 验证噪声非零
	var sum float64
	for _, v := range results {
		sum += v
	}
	mean := sum / float64(len(results))
	if math.Abs(mean-value) > 5.0 {
		t.Errorf("Gaussian noise mean deviation too large: %f", mean-value)
	}
}

func TestClipValue(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		bound    float64
		expected float64
	}{
		{"within bound", 5.0, 10.0, 5.0},
		{"exceeds upper", 15.0, 10.0, 10.0},
		{"exceeds lower", -15.0, 10.0, -10.0},
		{"at boundary", 10.0, 10.0, 10.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClipValue(tt.value, tt.bound)
			if result != tt.expected {
				t.Errorf("ClipValue(%f, %f) = %f, want %f", tt.value, tt.bound, result, tt.expected)
			}
		})
	}
}

func TestClipL2Norm(t *testing.T) {
	vec := []float64{3.0, 4.0} // L2 norm = 5.0
	maxNorm := 3.0

	result := ClipL2Norm(vec, maxNorm)
	var sumSq float64
	for _, v := range result {
		sumSq += v * v
	}
	norm := math.Sqrt(sumSq)

	if math.Abs(norm-maxNorm) > 0.01 {
		t.Errorf("ClipL2Norm norm = %f, want %f", norm, maxNorm)
	}
}

func TestNoisyCount(t *testing.T) {
	count := 100
	epsilon := 1.0

	var results []float64
	for i := 0; i < 100; i++ {
		noisy := NoisyCount(count, epsilon)
		results = append(results, noisy)
	}

	// 验证均值接近真实值
	var sum float64
	for _, v := range results {
		sum += v
	}
	mean := sum / float64(len(results))
	if math.Abs(mean-float64(count)) > 5.0 {
		t.Errorf("NoisyCount mean = %f, want ~%d", mean, count)
	}
}

func TestNoisySum(t *testing.T) {
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	epsilon := 1.0
	sensitivity := 5.0

	result := NoisySum(values, epsilon, sensitivity)
	expectedSum := 15.0

	// 验证结果接近真实总和
	if math.Abs(result-expectedSum) > 10.0 {
		t.Errorf("NoisySum = %f, want ~%f", result, expectedSum)
	}
}

func TestNoisyMean(t *testing.T) {
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	epsilon := 1.0
	delta := 1e-5
	clipBound := 10.0

	result := NoisyMean(values, epsilon, delta, clipBound)
	expectedMean := 3.0

	// 验证结果接近真实均值
	if math.Abs(result-expectedMean) > 2.0 {
		t.Errorf("NoisyMean = %f, want ~%f", result, expectedMean)
	}
}
