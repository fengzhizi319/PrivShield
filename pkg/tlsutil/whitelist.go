// Package tlsutil — dynamic mTLS CN whitelist with hot-reload support.
// Package tlsutil — 动态 mTLS CN 白名单与热重载支持。
//
// DynamicWhitelist 从 YAML 配置文件加载客户端 CN 白名单，支持不停机的
// 文件监听热重载（基于文件修改时间轮询），为 gRPC 双向认证提供
// 细粒度的 per-CN scope 授权校验。
//
// 配置文件格式（config/mtls-whitelist.yaml）：
//
//	version: "1.0"
//	clients:
//	  - cn: "bff-go.privshield.internal"
//	    allowed_scopes: ["*"]
//	  - cn: "service-hub.privshield.internal"
//	    allowed_scopes: ["/PrivacyService/Process"]
package tlsutil

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// WhitelistClient represents a single CN entry in the whitelist config.
// WhitelistClient 表示白名单配置中的单个 CN 条目。
type WhitelistClient struct {
	CN            string   `yaml:"cn"`
	AllowedScopes []string `yaml:"allowed_scopes"`
	Role          string   `yaml:"role,omitempty"`
	Description   string   `yaml:"description,omitempty"`
	Enabled       *bool    `yaml:"enabled,omitempty"` // nil = true (default)
}

// WhitelistConfig represents the top-level whitelist YAML configuration.
// WhitelistConfig 表示白名单 YAML 配置的顶层结构。
//
// Supports two YAML key formats for backward compatibility:
//   - "clients" (design doc standard): uses "allowed_scopes" field
//   - "entries" (legacy format): uses "scopes" field
type WhitelistConfig struct {
	Version string            `yaml:"version"`
	Clients []WhitelistClient `yaml:"clients"`
	Entries []struct {
		CN          string   `yaml:"cn"`
		Scopes      []string `yaml:"scopes"`
		Description string   `yaml:"description,omitempty"`
		Enabled     *bool    `yaml:"enabled,omitempty"`
	} `yaml:"entries"`
}

// DynamicWhitelist manages a hot-reloadable CN → scopes mapping.
// DynamicWhitelist 管理可热重载的 CN → scopes 映射。
//
// Thread-safe: uses RWMutex for concurrent read access during authorization
// checks, and exclusive write lock during config reload.
// 线程安全：使用 RWMutex 实现并发读（授权校验）与独占写（配置重载）。
type DynamicWhitelist struct {
	mu      sync.RWMutex
	clients map[string][]string // CN → allowed scopes
	path    string

	// Polling state / 轮询状态
	stopCh  chan struct{}
	stopped bool
	stopMu  sync.Mutex
}

// NewDynamicWhitelist creates a whitelist from a YAML file and starts background polling.
// NewDynamicWhitelist 从 YAML 文件创建白名单并启动后台文件变更轮询。
//
// The file is loaded immediately. A background goroutine polls the file's
// modification time every 5 seconds and triggers a reload when changes are detected.
// 文件立即加载。后台 goroutine 每 5 秒轮询文件修改时间，检测到变更时触发重载。
//
// Call Close() to stop the background polling goroutine.
// 调用 Close() 停止后台轮询 goroutine。
func NewDynamicWhitelist(path string) (*DynamicWhitelist, error) {
	cleanPath := filepath.Clean(path)
	dw := &DynamicWhitelist{
		clients: make(map[string][]string),
		path:    cleanPath,
		stopCh:  make(chan struct{}),
	}
	if err := dw.reload(); err != nil {
		return nil, err
	}
	go dw.poll()
	return dw, nil
}

// reload reads and parses the YAML whitelist configuration file.
// reload 读取并解析 YAML 白名单配置文件。
func (dw *DynamicWhitelist) reload() error {
	data, err := os.ReadFile(dw.path)
	if err != nil {
		return err
	}
	var conf WhitelistConfig
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return err
	}

	newClients := make(map[string][]string)

	// Priority 1: "clients" key (design doc standard) / 优先使用 "clients" 键
	for _, c := range conf.Clients {
		if c.Enabled != nil && !*c.Enabled {
			continue
		}
		newClients[c.CN] = c.AllowedScopes
	}

	// Priority 2: "entries" key (legacy format) / 回退到 "entries" 键
	if len(newClients) == 0 {
		for _, e := range conf.Entries {
			if e.Enabled != nil && !*e.Enabled {
				continue
			}
			newClients[e.CN] = e.Scopes
		}
	}

	dw.mu.Lock()
	defer dw.mu.Unlock()
	dw.clients = newClients
	log.Printf("[mTLS Whitelist] Reloaded %d authorized CN entries from %s", len(dw.clients), dw.path)
	return nil
}

// poll watches for file changes by polling modification time.
// poll 通过轮询文件修改时间监听文件变更。
//
// Poll interval: 5 seconds. This provides near-instant hot-reload without
// requiring external dependencies like fsnotify.
// 轮询间隔：5 秒。无需 fsnotify 等外部依赖即可实现近即时热重载。
func (dw *DynamicWhitelist) poll() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastModTime time.Time
	if info, err := os.Stat(dw.path); err == nil {
		lastModTime = info.ModTime()
	}

	for {
		select {
		case <-ticker.C:
			info, err := os.Stat(dw.path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastModTime) {
				lastModTime = info.ModTime()
				if err := dw.reload(); err != nil {
					log.Printf("[mTLS Whitelist] Reload error: %v", err)
				}
			}
		case <-dw.stopCh:
			return
		}
	}
}

// Close stops the background file polling goroutine.
// Close 停止后台文件轮询 goroutine。
func (dw *DynamicWhitelist) Close() {
	dw.stopMu.Lock()
	defer dw.stopMu.Unlock()
	if !dw.stopped {
		close(dw.stopCh)
		dw.stopped = true
	}
}

// IsAuthorized checks whether a client CN is present in the whitelist.
// IsAuthorized 检查客户端 CN 是否存在于白名单中。
func (dw *DynamicWhitelist) IsAuthorized(clientCN string) bool {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	_, exists := dw.clients[clientCN]
	return exists
}

// CheckScope checks whether a client CN is authorized for a specific method/scope.
// CheckScope 检查客户端 CN 是否被授权访问特定方法/范围。
//
// Scope matching rules / 范围匹配规则：
//   - "*" grants access to all methods / "*" 授予所有方法的访问权限
//   - Exact match against the method string / 精确匹配方法字符串
//   - Supports fnmatch-style wildcards via matchScopePattern / 支持通配符模式匹配
func (dw *DynamicWhitelist) CheckScope(clientCN string, method string) (bool, []string) {
	dw.mu.RLock()
	defer dw.mu.RUnlock()

	scopes, exists := dw.clients[clientCN]
	if !exists {
		return false, nil
	}

	for _, s := range scopes {
		if s == "*" || s == method || matchScopePattern(s, method) {
			return true, scopes
		}
	}
	return false, scopes
}

// GetScopes returns the allowed scopes for a given CN.
// GetScopes 返回指定 CN 的允许范围列表。
func (dw *DynamicWhitelist) GetScopes(clientCN string) ([]string, bool) {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	scopes, exists := dw.clients[clientCN]
	return scopes, exists
}

// matchScopePattern performs simple wildcard pattern matching.
// matchScopePattern 执行简单的通配符模式匹配。
//
// Supports:
//   - "*" matches everything / 匹配所有
//   - "/ServiceHub/*" matches "/ServiceHub/DispatchTask" etc.
//   - "*" within a path segment matches any characters in that segment
func matchScopePattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	// Simple prefix matching for patterns like "/ServiceHub/*"
	// 对 "/ServiceHub/*" 等模式执行简单前缀匹配
	if len(pattern) > 2 && pattern[len(pattern)-1] == '*' && pattern[len(pattern)-2] == '/' {
		prefix := pattern[:len(pattern)-1] // "/ServiceHub/"
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}
	return pattern == value
}
