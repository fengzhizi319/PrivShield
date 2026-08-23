// Package config provides centralized configuration for the datasource-mgr module.
// Package config 为数据源管理模块提供集中化配置管理。
package config

import (
	"strconv"

	pkgconfig "github.com/fengzhizi319/PrivShield/console/pkg/config"
)

// Config holds all runtime configuration for the datasource-mgr server.
type Config struct {
	Host          string // HTTP listen host
	Port          int    // HTTP listen port
	AgentRESTHost string // Upstream agent REST host
	AgentRESTPort int    // Upstream agent REST port
	AgentAPIKey   string // Optional auth key for upstream agent

	// Production hardening / 生产加固
	APIKey      string   // Inbound API key for this module / 本模块入站 API Key
	CORSOrigins []string // Allowed CORS origins / 允许的 CORS 来源
	DBPath      string   // SQLite database path (empty = in-memory) / SQLite 数据库路径
	LogFormat   string   // "json" or "text" / 日志格式
	LogLevel    string   // "debug", "info", "warn", "error" / 日志级别
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		Host:          pkgconfig.EnvString("DATASOURCE_MGR_HOST", "127.0.0.1"),
		Port:          pkgconfig.EnvInt("DATASOURCE_MGR_PORT", 8083),
		AgentRESTHost: pkgconfig.EnvString("PRIVACY_AGENT_REST_HOST", "127.0.0.1"),
		AgentRESTPort: pkgconfig.EnvInt("PRIVACY_REST_PORT", 8079),
		AgentAPIKey:   pkgconfig.EnvString("PRIVACY_AGENT_API_KEY", ""),

		// Production hardening / 生产加固
		APIKey:      pkgconfig.EnvString("DATASOURCE_MGR_API_KEY", ""),
		CORSOrigins: pkgconfig.EnvStringSlice("DATASOURCE_MGR_CORS_ORIGINS"),
		DBPath:      pkgconfig.EnvString("DATASOURCE_MGR_DB_PATH", ""),
		LogFormat:   pkgconfig.EnvString("DATASOURCE_MGR_LOG_FORMAT", "json"),
		LogLevel:    pkgconfig.EnvString("DATASOURCE_MGR_LOG_LEVEL", "info"),
	}
}

// Address returns the full HTTP listen address.
func (c *Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// AgentBaseURL returns the upstream agent REST base URL.
func (c *Config) AgentBaseURL() string {
	return "http://" + c.AgentRESTHost + ":" + strconv.Itoa(c.AgentRESTPort)
}
