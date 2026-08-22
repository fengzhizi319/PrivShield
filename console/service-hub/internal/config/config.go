// Package config provides centralized configuration for the service-hub module.
// Package config 为数据服务调度中枢模块提供集中化配置管理。
//
// Environment variables / 环境变量：
//
//	| Variable                  | Default    | Description                          |
//	|---------------------------|------------|--------------------------------------|
//	| SERVICE_HUB_HOST          | 127.0.0.1  | HTTP listen host / HTTP 监听地址      |
//	| SERVICE_HUB_PORT          | 8082       | HTTP listen port / HTTP 监听端口      |
//	| PRIVACY_AGENT_REST_HOST   | 127.0.0.1  | Upstream agent REST host / 上游 agent REST 地址 |
//	| PRIVACY_REST_PORT         | 8079       | Upstream agent REST port / 上游 agent REST 端口 |
//	| PRIVACY_AGENT_API_KEY     | (empty)    | Optional auth key / 可选认证密钥       |
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the service-hub server.
// Config 保存数据服务调度中枢服务器运行时的所有配置项。
type Config struct {
	Host            string // HTTP listen host / HTTP 监听地址
	Port            int    // HTTP listen port / HTTP 监听端口
	AgentRESTHost   string // Upstream agent REST host / 上游 agent REST 地址
	AgentRESTPort   int    // Upstream agent REST port / 上游 agent REST 端口
	AgentAPIKey     string // Optional auth key / 可选认证密钥
	MaxQueueDepth   int    // Max task queue depth / 最大任务队列深度
	ScheduleTimeout int    // Schedule timeout in seconds / 调度超时（秒）

	// gRPC server configuration / gRPC 服务器配置
	GRPCHost string // gRPC listen host / gRPC 监听地址
	GRPCPort int    // gRPC listen port / gRPC 监听端口

	// mTLS configuration / mTLS 双向认证配置
	TLSEnabled    bool   // Enable TLS/mTLS on gRPC server / 在 gRPC 服务端启用 TLS/mTLS
	TLSCertFile   string // Server certificate PEM / 服务端证书文件路径
	TLSKeyFile    string // Server private key PEM / 服务端私钥文件路径
	TLSCAFile     string // CA cert for client verification / 用于校验客户端证书的 CA 证书路径
	TLSClientAuth string // Client auth mode: "require" | "verify" | "" / 客户端认证模式

	// Public key pinning / 公钥固定（额外安全层）
	TLSPinnedPubKeyFile string // Pinned client public key PEM / 固定的客户端公钥文件路径
}

// Load reads configuration from environment variables.
// Load 从环境变量读取所有配置项。
func Load() *Config {
	return &Config{
		Host:            getEnv("SERVICE_HUB_HOST", "127.0.0.1"),
		Port:            getEnvInt("SERVICE_HUB_PORT", 8082),
		AgentRESTHost:   getEnv("PRIVACY_AGENT_REST_HOST", "127.0.0.1"),
		AgentRESTPort:   getEnvInt("PRIVACY_REST_PORT", 8079),
		AgentAPIKey:     getEnv("PRIVACY_AGENT_API_KEY", ""),
		MaxQueueDepth:   getEnvInt("SERVICE_HUB_MAX_QUEUE", 1000),
		ScheduleTimeout: getEnvInt("SERVICE_HUB_SCHEDULE_TIMEOUT", 30),

		// gRPC / gRPC 配置
		GRPCHost: getEnv("SERVICE_HUB_GRPC_HOST", "127.0.0.1"),
		GRPCPort: getEnvInt("SERVICE_HUB_GRPC_PORT", 50052),

		// mTLS / 双向认证配置
		TLSEnabled:    getEnvBool("SERVICE_HUB_TLS_ENABLED", false),
		TLSCertFile:   getEnv("SERVICE_HUB_TLS_CERT_FILE", ""),
		TLSKeyFile:    getEnv("SERVICE_HUB_TLS_KEY_FILE", ""),
		TLSCAFile:     getEnv("SERVICE_HUB_TLS_CA_FILE", ""),
		TLSClientAuth: getEnv("SERVICE_HUB_TLS_CLIENT_AUTH", ""),

		// Public key pinning / 公钥固定
		TLSPinnedPubKeyFile: getEnv("SERVICE_HUB_TLS_PINNED_PUBKEY_FILE", ""),
	}
}

// Address returns the full HTTP listen address.
// Address 返回完整的 HTTP 监听地址。
func (c *Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// AgentBaseURL returns the upstream agent REST base URL.
// AgentBaseURL 返回上游 agent REST 基础地址。
func (c *Config) AgentBaseURL() string {
	return "http://" + c.AgentRESTHost + ":" + strconv.Itoa(c.AgentRESTPort)
}

// GRPCAddress returns the full gRPC listen address.
// GRPCAddress 返回完整的 gRPC 监听地址。
func (c *Config) GRPCAddress() string {
	return c.GRPCHost + ":" + strconv.Itoa(c.GRPCPort)
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
