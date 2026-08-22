// Package config provides centralized configuration for the datasource-mgr module.
// Package config 为数据源管理模块提供集中化配置管理。
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the datasource-mgr server.
type Config struct {
	Host          string // HTTP listen host
	Port          int    // HTTP listen port
	AgentRESTHost string // Upstream agent REST host
	AgentRESTPort int    // Upstream agent REST port
	AgentAPIKey   string // Optional auth key
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		Host:          getEnv("DATASOURCE_MGR_HOST", "127.0.0.1"),
		Port:          getEnvInt("DATASOURCE_MGR_PORT", 8083),
		AgentRESTHost: getEnv("PRIVACY_AGENT_REST_HOST", "127.0.0.1"),
		AgentRESTPort: getEnvInt("PRIVACY_REST_PORT", 8079),
		AgentAPIKey:   getEnv("PRIVACY_AGENT_API_KEY", ""),
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

func getEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func getEnvInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func getEnvBool(name string, def bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
