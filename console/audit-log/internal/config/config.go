// Package config provides centralized configuration for the audit-log module.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the audit-log server.
type Config struct {
	Host          string // HTTP listen host
	Port          int    // HTTP listen port
	AgentRESTHost string // Upstream agent REST host
	AgentRESTPort int    // Upstream agent REST port
	AgentAPIKey   string // Optional auth key
	MaxLogEntries int    // Max audit log entries to keep in memory
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		Host:          getEnv("AUDIT_LOG_HOST", "127.0.0.1"),
		Port:          getEnvInt("AUDIT_LOG_PORT", 8084),
		AgentRESTHost: getEnv("PRIVACY_AGENT_REST_HOST", "127.0.0.1"),
		AgentRESTPort: getEnvInt("PRIVACY_REST_PORT", 8079),
		AgentAPIKey:   getEnv("PRIVACY_AGENT_API_KEY", ""),
		MaxLogEntries: getEnvInt("AUDIT_LOG_MAX_ENTRIES", 10000),
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
