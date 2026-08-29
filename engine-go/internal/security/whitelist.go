// Package security — mTLS CN 白名单管理（热重载）。
//
// 对齐 Python engine/security/whitelist.py：
//   - 基于 YAML 配置文件的 CN 白名单管理
//   - 每个 CN 独立 scope 控制（最小权限原则）
//   - 基于文件 mtime 的热重载（请求驱动，被动检查）
//   - 线程安全的两阶段提交
//   - 向后兼容环境变量静态列表
package security

import (
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// CNEntry 表示白名单中的单个 CN 条目。
type CNEntry struct {
	CN          string   `yaml:"cn"`
	Scopes      []string `yaml:"scopes"`
	Description string   `yaml:"description"`
	Enabled     bool     `yaml:"enabled"`
}

// WhitelistConfig 表示 YAML 白名单配置文件根结构。
type WhitelistConfig struct {
	Version       string    `yaml:"version"`
	Entries       []CNEntry `yaml:"entries"`
	DefaultScopes []string  `yaml:"default_scopes"`
}

// WhitelistManager 线程安全的 mTLS CN 白名单管理器，支持热重载。
type WhitelistManager struct {
	configPath    string
	staticCNs     []string
	mu            sync.RWMutex
	cache         map[string]*CNEntry
	defaultScopes []string
	lastMtime     time.Time
	lastLoadTime  time.Time
	loadError     string
}

// NewWhitelistManager 创建白名单管理器。
// configPath 非空时从 YAML 文件加载；否则使用 staticCNs 构建静态列表。
func NewWhitelistManager(configPath string, staticCNs []string) *WhitelistManager {
	m := &WhitelistManager{
		configPath: configPath,
		staticCNs:  staticCNs,
		cache:      make(map[string]*CNEntry),
	}
	m.load()
	return m
}

// load 加载或重载白名单配置。
// 使用两阶段提交：先解析到临时缓冲区，成功后原子交换。
func (m *WhitelistManager) load() {
	if m.configPath == "" {
		// 静态列表模式
		newCache := make(map[string]*CNEntry)
		for _, cn := range m.staticCNs {
			newCache[cn] = &CNEntry{CN: cn, Scopes: []string{"*"}, Description: "Static env var entry", Enabled: true}
		}
		m.mu.Lock()
		m.cache = newCache
		m.defaultScopes = nil
		m.lastLoadTime = time.Now()
		m.loadError = ""
		m.mu.Unlock()
		return
	}

	info, err := os.Stat(m.configPath)
	if err != nil {
		m.mu.Lock()
		m.loadError = "whitelist config file not found: " + m.configPath
		m.mu.Unlock()
		return
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		m.mu.Lock()
		m.loadError = "failed to read whitelist config: " + err.Error()
		m.mu.Unlock()
		return
	}

	var config WhitelistConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		m.mu.Lock()
		m.loadError = "failed to parse whitelist YAML: " + err.Error()
		m.mu.Unlock()
		return
	}

	// 构建新缓存（临时缓冲区）
	newCache := make(map[string]*CNEntry)
	for i := range config.Entries {
		entry := &config.Entries[i]
		if entry.Enabled {
			newCache[entry.CN] = entry
		}
	}

	// 原子交换
	m.mu.Lock()
	m.cache = newCache
	m.defaultScopes = config.DefaultScopes
	m.lastMtime = info.ModTime()
	m.lastLoadTime = time.Now()
	m.loadError = ""
	m.mu.Unlock()
}

// checkReload 检查配置文件是否变更并在需要时触发重载。
func (m *WhitelistManager) checkReload() {
	if m.configPath == "" {
		return
	}
	info, err := os.Stat(m.configPath)
	if err != nil {
		return
	}
	if info.ModTime().After(m.lastMtime) {
		m.load()
	}
}

// GetEntry 查找 CN 白名单条目。查找前触发温热载检查。
func (m *WhitelistManager) GetEntry(cn string) *CNEntry {
	m.checkReload()
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cache[cn]
}

// GetScopes 返回 CN 的 scope 列表。未找到返回 nil。
func (m *WhitelistManager) GetScopes(cn string) []string {
	entry := m.GetEntry(cn)
	if entry != nil {
		return entry.Scopes
	}
	return nil
}

// IsAllowed 检查 CN 是否在白名单中。
func (m *WhitelistManager) IsAllowed(cn string) bool {
	return m.GetEntry(cn) != nil
}

// DefaultScopes 返回未知 CN 的默认 scope（空 = fail-closed）。
func (m *WhitelistManager) DefaultScopes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.defaultScopes))
	copy(result, m.defaultScopes)
	return result
}

// AllEntries 返回所有活跃白名单条目的快照。
func (m *WhitelistManager) AllEntries() []CNEntry {
	m.checkReload()
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := make([]CNEntry, 0, len(m.cache))
	for _, e := range m.cache {
		entries = append(entries, *e)
	}
	return entries
}

// LastLoadTime 返回最近一次成功加载的时间。
func (m *WhitelistManager) LastLoadTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastLoadTime
}

// LastError 返回最近一次加载的错误信息。
func (m *WhitelistManager) LastError() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loadError
}

// Reload 强制重载白名单配置。
func (m *WhitelistManager) Reload() {
	m.load()
}

// ──────────────────────────────────────────────
// 模块级单例
// ──────────────────────────────────────────────

var (
	whitelistManager     *WhitelistManager
	whitelistManagerOnce sync.Once
)

// GetWhitelistManager 获取模块级 WhitelistManager 单例。
func GetWhitelistManager() *WhitelistManager {
	whitelistManagerOnce.Do(func() {
		settings := GetSettings()
		whitelistFile := settings.MTLSWhitelistFile
		if whitelistFile != "" {
			whitelistManager = NewWhitelistManager(whitelistFile, nil)
		} else {
			whitelistManager = NewWhitelistManager("", settings.MTLSAllowedCNs)
		}
	})
	return whitelistManager
}

// ResetWhitelistManager 重置单例（仅测试用）。
func ResetWhitelistManager() {
	whitelistManagerOnce = sync.Once{}
	whitelistManager = nil
}
