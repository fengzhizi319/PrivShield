// Package config provides centralized configuration for the service-hub module.
// Package config 为数据服务调度中枢模块（service-hub）提供集中化配置管理。
//
// 该模块负责从环境变量中解析自身 HTTP/gRPC 网络参数、上游 PrivShield Agent 引擎地址、
// datasource-mgr 数据源服务连接、SQLite 任务持久化路径、mTLS 双向证书与公钥固定，
// 并提供安全合理的回退默认值。
package config

import (
	"fmt"
	"os"
	"strconv"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
)

// Config holds all runtime configuration for the service-hub server.
// Config 结构体保存数据服务调度中枢服务器运行时的所有配置项。
type Config struct {
	// HTTP REST 服务网络监听参数
	Host string // HTTP 监听主机地址（默认 127.0.0.1）
	Port int    // HTTP 监听端口（默认 8082）

	// 上游 PrivShield Python Agent 核心引擎连接配置
	AgentRESTHost   string // Agent REST 主机地址（默认 127.0.0.1）
	AgentRESTPort   int    // Agent REST 端口（默认 8079）
	AgentAPIKey     string // 访问上游 Agent 接口所需的 API Key 认证密钥
	MaxQueueDepth   int    // 调度引擎最大任务等待队列深度（默认 1000）
	ScheduleTimeout int    // 任务单步调度与执行超时时间（秒，默认 30）

	// datasource-mgr 模拟数据源服务连接配置
	DatasourceRESTHost string // 数据源服务 HTTP REST 主机地址（默认 127.0.0.1）
	DatasourceRESTPort int    // 数据源服务 HTTP REST 端口（默认 8083）
	DatasourceGRPCHost string // 数据源服务 gRPC 主机地址（默认 127.0.0.1）
	DatasourceGRPCPort int    // 数据源服务 gRPC 端口（默认 50053）

	// gRPC 远程过程调用服务网络监听参数
	GRPCHost string // gRPC 监听主机地址（默认 127.0.0.1）
	GRPCPort int    // gRPC 监听端口（默认 50052）

	// mTLS 双向传输层安全认证配置
	TLSEnabled    bool   // 是否在 gRPC/HTTPS 服务端启用 TLS/mTLS 强加密
	TLSCertFile   string // 服务端 X.509 证书 PEM 文件路径
	TLSKeyFile    string // 服务端私钥 PEM 文件路径
	TLSCAFile     string // 验证调用方客户端身份的受信任根 CA 证书路径
	TLSClientAuth string // 客户端认证模式："require"（强制双向校验）| "verify" | "request" | ""

	// 应用层公钥指纹固定（SPKI Pinning，防御 CA 劫持与伪造）
	TLSPinnedPubKeyFile string // 固定的客户端 RSA 公钥 PEM 文件路径

	// 生产安全加固与持久化配置
	APIKey      string   // 本模块对外暴露接口的入站鉴权 API Key（为空表示免密）
	CORSOrigins []string // 允许跨域访问的 Origin 来源白名单
	DBPath      string   // SQLite 任务数据库文件物理路径（为空表示使用进程内内存存储）
	LogFormat   string   // 日志输出格式："json"（生产推荐）或 "text"（开发可读）
	LogLevel    string   // 日志输出级别："debug", "info", "warn", "error"

	// Data retention / 数据保留策略
	RetentionDays int // 终态任务保留天数，超期自动清理（0 = 不清理）

	// Graceful shutdown / 优雅关闭
	ShutdownTimeout int // HTTP 优雅关闭超时秒数（默认 5）

	// Rate limiting / 每客户端 IP 令牌桶限流
	RateLimitRPS   int // 每秒允许的请求数（默认 100，0 = 不限流）
	RateLimitBurst int // 令牌桶突发容量（默认 200）

	// ── Phase B: PostgreSQL 多副本 Hub 配置 ──
	PGDSN     string // PostgreSQL 连接字符串（为空时回退 SQLite）
	PGMaxConn int    // PostgreSQL 最大连接池大小（默认 10）
	PGMinConn int    // PostgreSQL 最小连接池大小（默认 2）
	LeaseTTL  int    // 任务租约 TTL 秒数（默认 60）
}

// Load reads configuration from environment variables with fallback defaults.
// Load 函数从系统环境变量中读取各项配置，若未设置则自动回退至预设的安全默认值。
// 执行步骤：
// 1. 调用 pkgconfig.Env* 依次解析 HTTP、Agent、Datasource、gRPC、mTLS、DB 与日志参数；
// 2. 构造并返回初始化的 *Config 实例。
func Load() *Config {
	return &Config{
		Host:            pkgconfig.EnvString("SERVICE_HUB_HOST", "127.0.0.1"),
		Port:            pkgconfig.EnvInt("SERVICE_HUB_PORT", 8082),
		AgentRESTHost:   pkgconfig.EnvString("PRIVACY_AGENT_REST_HOST", "127.0.0.1"),
		AgentRESTPort:   pkgconfig.EnvInt("PRIVACY_REST_PORT", 8079),
		AgentAPIKey:     pkgconfig.EnvString("PRIVACY_AGENT_API_KEY", ""),
		MaxQueueDepth:   pkgconfig.EnvInt("SERVICE_HUB_MAX_QUEUE", 1000),
		ScheduleTimeout: pkgconfig.EnvInt("SERVICE_HUB_SCHEDULE_TIMEOUT", 30),

		// Datasource Mgr 数据源服务连接参数
		DatasourceRESTHost: pkgconfig.EnvString("DATASOURCE_MGR_HOST", "127.0.0.1"),
		DatasourceRESTPort: pkgconfig.EnvInt("DATASOURCE_MGR_PORT", 8083),
		DatasourceGRPCHost: pkgconfig.EnvString("DATASOURCE_MGR_GRPC_HOST", "127.0.0.1"),
		DatasourceGRPCPort: pkgconfig.EnvInt("DATASOURCE_MGR_GRPC_PORT", 50053),

		// gRPC 服务监听参数（默认 127.0.0.1:50052）
		GRPCHost: pkgconfig.EnvString("SERVICE_HUB_GRPC_HOST", "127.0.0.1"),
		GRPCPort: pkgconfig.EnvInt("SERVICE_HUB_GRPC_PORT", 50052),

		// mTLS 双向传输层安全认证配置
		TLSEnabled:    pkgconfig.EnvBool("SERVICE_HUB_TLS_ENABLED", false),
		TLSCertFile:   pkgconfig.EnvString("SERVICE_HUB_TLS_CERT_FILE", ""),
		TLSKeyFile:    pkgconfig.EnvString("SERVICE_HUB_TLS_KEY_FILE", ""),
		TLSCAFile:     pkgconfig.EnvString("SERVICE_HUB_TLS_CA_FILE", ""),
		TLSClientAuth: pkgconfig.EnvString("SERVICE_HUB_TLS_CLIENT_AUTH", ""),

		// 客户端 RSA 公钥固定
		TLSPinnedPubKeyFile: pkgconfig.EnvString("SERVICE_HUB_TLS_PINNED_PUBKEY_FILE", ""),

		// 生产鉴权、跨域与存储参数
		APIKey:      pkgconfig.EnvString("SERVICE_HUB_API_KEY", ""),
		CORSOrigins: pkgconfig.EnvStringSlice("SERVICE_HUB_CORS_ORIGINS"),
		DBPath:      pkgconfig.EnvString("SERVICE_HUB_DB_PATH", ""),
		LogFormat:   pkgconfig.EnvString("SERVICE_HUB_LOG_FORMAT", "json"),
		LogLevel:    pkgconfig.EnvString("SERVICE_HUB_LOG_LEVEL", "info"),

		// Data retention / 数据保留策略（默认 30 天）
		RetentionDays: pkgconfig.EnvInt("SERVICE_HUB_RETENTION_DAYS", 30),

		// Graceful shutdown / 优雅关闭超时（默认 5 秒）
		ShutdownTimeout: pkgconfig.EnvInt("SERVICE_HUB_SHUTDOWN_TIMEOUT", 5),

		// Rate limiting / 每客户端 IP 令牌桶限流（默认 100 rps，突发 200）
		RateLimitRPS:   pkgconfig.EnvInt("SERVICE_HUB_RATE_LIMIT_RPS", 100),
		RateLimitBurst: pkgconfig.EnvInt("SERVICE_HUB_RATE_LIMIT_BURST", 200),

		// ── Phase B: PostgreSQL 多副本 Hub 配置 ──
		PGDSN:     pkgconfig.EnvString("SERVICE_HUB_PG_DSN", ""),
		PGMaxConn: pkgconfig.EnvInt("SERVICE_HUB_PG_MAX_CONNS", 10),
		PGMinConn: pkgconfig.EnvInt("SERVICE_HUB_PG_MIN_CONNS", 2),
		LeaseTTL:  pkgconfig.EnvInt("SERVICE_HUB_LEASE_TTL", 60),
	}
}

// Validate checks that the configuration is consistent and all required files exist.
// Validate 校验配置一致性：当 TLS 启用时确认证书/私钥文件存在，
// 在启动早期快速失败并给出清晰错误信息，避免运行时才暴露配置问题。
func (c *Config) Validate() error {
	if c.TLSEnabled {
		if c.TLSCertFile == "" {
			return fmt.Errorf("TLS enabled but SERVICE_HUB_TLS_CERT_FILE is not set")
		}
		if c.TLSKeyFile == "" {
			return fmt.Errorf("TLS enabled but SERVICE_HUB_TLS_KEY_FILE is not set")
		}
		if _, err := os.Stat(c.TLSCertFile); err != nil {
			return fmt.Errorf("TLS cert file not accessible: %s: %w", c.TLSCertFile, err)
		}
		if _, err := os.Stat(c.TLSKeyFile); err != nil {
			return fmt.Errorf("TLS key file not accessible: %s: %w", c.TLSKeyFile, err)
		}
	}
	return nil
}

// Address returns the full HTTP listen address formatted as "host:port".
// Address 返回完整的 HTTP 服务网络监听地址（如 "127.0.0.1:8082" 或 "0.0.0.0:8082"）。
func (c *Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// AgentBaseURL returns the upstream agent REST base URL.
// AgentBaseURL 返回默认单实例上游 Agent 引擎的 HTTP REST 基础 URL（如 "http://127.0.0.1:8079"）。
func (c *Config) AgentBaseURL() string {
	return "http://" + c.AgentRESTHost + ":" + strconv.Itoa(c.AgentRESTPort)
}

// AgentBaseURLs returns all configured upstream agent REST base URLs for load balancing/failover.
// AgentBaseURLs 返回所有已配置的上游 Agent 引擎 REST URL 列表：
// 优先读取 PRIVACY_AGENT_URLS 环境变量（逗号分隔的多个 Agent 地址），未配置时回退为单个 AgentBaseURL()。
func (c *Config) AgentBaseURLs() []string {
	envURLs := pkgconfig.EnvStringSlice("PRIVACY_AGENT_URLS")
	if len(envURLs) > 0 {
		return envURLs
	}
	return []string{c.AgentBaseURL()}
}

// DatasourceBaseURL returns the datasource manager HTTP base URL.
// DatasourceBaseURL 返回模拟数据源服务的 HTTP REST 基础 URL（如 "http://127.0.0.1:8083"）。
func (c *Config) DatasourceBaseURL() string {
	return "http://" + c.DatasourceRESTHost + ":" + strconv.Itoa(c.DatasourceRESTPort)
}

// DatasourceGRPCAddress returns the datasource manager gRPC address.
// DatasourceGRPCAddress 返回模拟数据源服务的 gRPC 监听网络地址（如 "127.0.0.1:50053"）。
func (c *Config) DatasourceGRPCAddress() string {
	return c.DatasourceGRPCHost + ":" + strconv.Itoa(c.DatasourceGRPCPort)
}

// GRPCAddress returns the full gRPC listen address formatted as "host:port".
// GRPCAddress 返回 service-hub 自身 gRPC 服务的网络监听地址（如 "127.0.0.1:50052"）。
func (c *Config) GRPCAddress() string {
	return c.GRPCHost + ":" + strconv.Itoa(c.GRPCPort)
}
