package security

import (
	"os"
	"strings"
	"sync"
)

// KeyConfig 表示单个 API Key 配置。
type KeyConfig struct {
	Name   string
	Scopes []string
}

// Settings 安全配置，从环境变量加载。
type Settings struct {
	AuthEnabled              bool
	TLSEnabled               bool
	RateLimitEnabled         bool
	HealthNoAuth             bool
	HealthNoRateLimit        bool
	InternalKeys             map[string]*KeyConfig // token -> config
	ExternalKeys             map[string]*KeyConfig // token -> config
	RateLimitDefaultRPS      float64
	RateLimitDefaultBurst    int
	RateLimitPerEndpoint     map[string]*EndpointRateLimit
	RateLimitRedisURL        string
	MTLSAllowedCNs           []string
	MTLSWhitelistFile        string
	MTLSEnabled              bool
}

// EndpointRateLimit 单端点限流配置。
type EndpointRateLimit struct {
	RPS   float64
	Burst int
}

var (
	settingsOnce     sync.Once
	cachedSettings   *Settings
)

// GetSettings 返回缓存的安全配置单例。
func GetSettings() *Settings {
	settingsOnce.Do(func() {
		cachedSettings = loadSettings()
	})
	return cachedSettings
}

// ResetSettings 重置缓存（仅测试用）。
func ResetSettings() {
	settingsOnce = sync.Once{}
	cachedSettings = nil
}

func loadSettings() *Settings {
	internalKeys := parseAPIKeys("PRIVACY_AUTH_INTERNAL_API_KEYS")
	if internalKeys == nil {
		internalKeys = make(map[string]*KeyConfig)
	}
	for _, envK := range []string{"PRIVACY_AUTH_API_KEY", "PRIVACY_API_KEY"} {
		if k := os.Getenv(envK); k != "" {
			internalKeys[k] = &KeyConfig{Name: "default-internal", Scopes: []string{"*"}}
		}
	}

	externalKeys := parseAPIKeys("PRIVACY_AUTH_EXTERNAL_API_KEYS")
	if externalKeys == nil {
		externalKeys = make(map[string]*KeyConfig)
	}
	if ext := parseAPIKeys("PRIVACY_AUTH_STATIC_API_KEYS"); ext != nil {
		for k, v := range ext {
			externalKeys[k] = v
		}
	}

	s := &Settings{
		AuthEnabled:           envBool("PRIVACY_AUTH_ENABLED", false),
		TLSEnabled:            envBool("PRIVACY_TLS_ENABLED", false),
		RateLimitEnabled:      envBool("PRIVACY_RATE_LIMIT_ENABLED", false),
		HealthNoAuth:          envBool("PRIVACY_HEALTH_NO_AUTH", true),
		HealthNoRateLimit:     envBool("PRIVACY_HEALTH_NO_RATE_LIMIT", true),
		MTLSEnabled:           envBool("PRIVACY_AUTH_INTERNAL_MTLS_ENABLED", false),
		MTLSWhitelistFile:     os.Getenv("PRIVACY_AUTH_MTLS_WHITELIST_FILE"),
		RateLimitDefaultRPS:   envFloat("PRIVACY_RATE_LIMIT_DEFAULT_RPS", 100),
		RateLimitDefaultBurst: envInt("PRIVACY_RATE_LIMIT_DEFAULT_BURST", 200),
		RateLimitRedisURL:     os.Getenv("PRIVACY_RATE_LIMIT_REDIS_URL"),
		InternalKeys:          internalKeys,
		ExternalKeys:          externalKeys,
		MTLSAllowedCNs:        parseStringList(os.Getenv("PRIVACY_AUTH_MTLS_ALLOWED_CNS")),
	}
	return s
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return strings.EqualFold(v, "true") || v == "1"
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f := 0.0
	_, _ = strings.CutPrefix(v, "")
	n, err := parseFloat(v)
	if err == nil {
		f = n
	} else {
		f = def
	}
	return f
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := parseInt(v)
	if err != nil {
		return def
	}
	return n
}

func parseFloat(s string) (float64, error) {
	// Simple float parser
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	intPart := 0
	i := 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		intPart = intPart*10 + int(s[i]-'0')
	}
	fracPart := 0.0
	if i < len(s) && s[i] == '.' {
		i++
		div := 10.0
		for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
			fracPart += float64(s[i]-'0') / div
			div *= 10
		}
	}
	result := float64(intPart) + fracPart
	if neg {
		result = -result
	}
	return result, nil
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, &parseError{s: s}
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

type parseError struct{ s string }

func (e *parseError) Error() string { return "invalid number: " + e.s }

// parseAPIKeys 解析 "key1:name1:scope1,scope2;key2:name2:scope3" 格式。
func parseAPIKeys(envKey string) map[string]*KeyConfig {
	raw := os.Getenv(envKey)
	if raw == "" {
		return nil
	}
	keys := make(map[string]*KeyConfig)
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) < 2 {
			continue
		}
		token := parts[0]
		name := parts[1]
		var scopes []string
		if len(parts) == 3 && parts[2] != "" {
			scopes = strings.Split(parts[2], ",")
		} else {
			scopes = []string{"*"}
		}
		keys[token] = &KeyConfig{Name: name, Scopes: scopes}
	}
	return keys
}

func parseStringList(s string) []string {
	if s == "" {
		return nil
	}
	// Try JSON array first
	if strings.HasPrefix(s, "[") {
		s = strings.Trim(s, "[]")
	}
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "\"")
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
