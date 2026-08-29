// Package profile — 隐私参数解析与校验模块。
//
// 对齐 Python engine/privacy/profile.py：
// 从 YAML 配置文件加载各隐私原语的默认参数，支持请求级参数覆盖。
// 提供参数校验能力，确保 DP epsilon 为正、K-Anonymity k 不小于 2 等。
package profile

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// PrimitiveParams 隐私原语默认参数。
type PrimitiveParams map[string]interface{}

// PrivacyProfile 隐私参数配置。
type PrivacyProfile struct {
	Name       string                      `yaml:"name"`
	Version    string                      `yaml:"version"`
	Defaults   map[string]PrimitiveParams   `yaml:"defaults"`
	Namespaces map[string]PrimitiveParams   `yaml:"namespaces"`
}

// Resolver 隐私参数解析器。
type Resolver struct {
	mu      sync.RWMutex
	profile *PrivacyProfile
}

// NewResolver 创建参数解析器。
func NewResolver() *Resolver {
	return &Resolver{
		profile: defaultProfile(),
	}
}

// LoadFromYAML 从 YAML 文件加载配置。
func (r *Resolver) LoadFromYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read profile: %w", err)
	}
	var p PrivacyProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("parse profile YAML: %w", err)
	}
	r.mu.Lock()
	r.profile = &p
	r.mu.Unlock()
	return nil
}

// Resolve 解析指定原语的参数，支持请求级覆盖。
// 优先级：请求参数 > 命名空间参数 > 全局默认 > 内置默认。
func (r *Resolver) Resolve(primitive string, namespace string, overrides map[string]interface{}) map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. 内置默认
	result := builtinDefaults(primitive)

	// 2. 全局默认覆盖
	if r.profile != nil && r.profile.Defaults != nil {
		if defaults, ok := r.profile.Defaults[primitive]; ok {
			for k, v := range defaults {
				result[k] = v
			}
		}
	}

	// 3. 命名空间参数覆盖
	if namespace != "" && r.profile != nil && r.profile.Namespaces != nil {
		if nsParams, ok := r.profile.Namespaces[namespace]; ok {
			if v, exists := nsParams[primitive]; exists {
				if m, ok := v.(map[string]interface{}); ok {
					for k, val := range m {
						result[k] = val
					}
				}
			}
		}
	}

	// 4. 请求级覆盖
	for k, v := range overrides {
		result[k] = v
	}

	return result
}

// Recommend 返回推荐的隐私参数配置。
func (r *Resolver) Recommend() map[string]interface{} {
	return map[string]interface{}{
		"recommended_profile": r.profileName(),
		"epsilon":             1.0,
		"delta":               1e-5,
		"k":                   5,
		"mechanism":           "laplace",
		"note":                "Go 引擎参数推荐",
	}
}

func (r *Resolver) profileName() string {
	if r.profile != nil && r.profile.Name != "" {
		return r.profile.Name
	}
	return "standard"
}

// Validate 校验参数合法性。
func Validate(primitive string, params map[string]interface{}) error {
	switch primitive {
	case "dp":
		if eps, ok := params["epsilon"].(float64); ok && eps <= 0 {
			return fmt.Errorf("dp epsilon must be positive, got %f", eps)
		}
		if delta, ok := params["delta"].(float64); ok && delta < 0 {
			return fmt.Errorf("dp delta must be non-negative, got %f", delta)
		}
	case "k_anonymity":
		if k, ok := params["k"].(int); ok && k < 2 {
			return fmt.Errorf("k_anonymity k must be >= 2, got %d", k)
		}
		if kf, ok := params["k"].(float64); ok && kf < 2 {
			return fmt.Errorf("k_anonymity k must be >= 2, got %f", kf)
		}
	}
	return nil
}

// ──────────────────────────────────────────────
// 内置默认参数
// ──────────────────────────────────────────────

func defaultProfile() *PrivacyProfile {
	return &PrivacyProfile{
		Name:    "standard",
		Version: "1.0",
		Defaults: map[string]PrimitiveParams{
			"dp":          {"epsilon": 1.0, "delta": 0.0, "mechanism": "laplace"},
			"k_anonymity": {"k": 5, "l": 2, "t": 0.2, "max_depth": 10},
			"sanitization": {"engine": "mask"},
			"qol": {
				"num_dummies": 3,
			},
			"classification": {
				"confidence_threshold": 0.75,
			},
		},
	}
}

func builtinDefaults(primitive string) map[string]interface{} {
	defaults := map[string]map[string]interface{}{
		"dp":          {"epsilon": 1.0, "delta": 0.0, "mechanism": "laplace"},
		"k_anonymity": {"k": 5, "l": 2, "t": 0.2, "max_depth": 10},
		"sanitization": {"engine": "mask"},
		"qol":         {"num_dummies": 3},
		"classification": {"confidence_threshold": 0.75},
	}
	if d, ok := defaults[primitive]; ok {
		result := make(map[string]interface{}, len(d))
		for k, v := range d {
			result[k] = v
		}
		return result
	}
	return make(map[string]interface{})
}
