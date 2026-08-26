// Package config 负责从环境变量加载 App-LZ BFF 的全部运行时配置。
//
// 环境变量命名规则：
//   - 优先读取 APP_LZ_* 前缀变量（推荐）
//   - 兼容无前缀的旧变量名（如 HUB_URL），方便向后兼容
//
// 每个上游微服务需要配置两个地址：
//   - HTTP URL（如 http://127.0.0.1:8082）—— 用于 REST API 调用
//   - GRPC 地址（如 127.0.0.1:50052）—— 用于拓扑探测时尝试 gRPC 连接
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config 保存 App-LZ BFF 的全部运行时配置项。
// 所有字段在 Load() 时从环境变量一次性读取，运行期不可变。
type Config struct {
	// ── HTTP Server 监听配置 ──
	Host string // 监听地址，默认 0.0.0.0
	Port string // 监听端口，默认 8085

	// ── 上游微服务 HTTP URL ──
	HubURL        string // Service Hub 调度中枢 REST 地址（默认 http://127.0.0.1:8082）
	DatasourceURL string // 数据源管理器 REST 地址（默认 http://127.0.0.1:8083）
	AuditURL      string // 审计存证服务 REST 地址（默认 http://127.0.0.1:8084）
	AgentURL      string // 隐私脱敏引擎 REST 地址（默认 http://127.0.0.1:8079）

	// ── 上游微服务 gRPC 地址（用于拓扑探测）──
	HubGRPC        string // Service Hub gRPC 地址（默认 127.0.0.1:50052）
	DatasourceGRPC string // 数据源管理器 gRPC 地址（默认 127.0.0.1:50053）
	AuditGRPC      string // 审计存证服务 gRPC 地址（默认 127.0.0.1:50054）
	AgentGRPC      string // 隐私脱敏引擎 gRPC 地址（默认 127.0.0.1:50051）

	// ── 静态文件 & 日志 ──
	StaticDir string // 前端 SPA 构建产物目录（默认 ./web/dist）
	LogLevel  string // 日志级别（默认 info）

	// ── TLS 配置 ──
	TLSEnabled   bool   // 是否启用 TLS（默认 false）
	CertFile     string // TLS 证书文件路径
	KeyFile      string // TLS 私钥文件路径
	ClientCAFile string // 客户端 CA 证书路径（用于 mTLS）

	// ── 认证配置 ──
	APIKey string // API Key（用于 BFF 自身的认证校验）
}

// Load 从环境变量加载配置，未设置时使用合理默认值。
//
// 环境变量优先级：APP_LZ_* 前缀 > 无前缀旧变量 > 硬编码默认值。
// 例如 HubURL 的读取顺序：APP_LZ_HUB_URL → HUB_URL → http://127.0.0.1:8082
func Load() *Config {
	// ── Server 监听地址 ──
	host := getEnv("APP_LZ_HOST", "0.0.0.0")
	port := getEnv("APP_LZ_PORT", "8085")

	// ── 上游微服务双协议地址（HTTP + gRPC）──
	// 每个服务先用 getEnv 读 APP_LZ_* 前缀变量，若未设置则 fallback 到无前缀旧变量，
	// 最终 fallback 到硬编码默认值（本地开发环境地址）。
	hubURL := getEnv("APP_LZ_HUB_URL", getEnv("HUB_URL", "http://127.0.0.1:8082"))
	hubGRPC := getEnv("APP_LZ_HUB_GRPC", getEnv("HUB_GRPC", "127.0.0.1:50052"))
	datasourceURL := getEnv("APP_LZ_DATASOURCE_URL", getEnv("DATASOURCE_URL", "http://127.0.0.1:8083"))
	datasourceGRPC := getEnv("APP_LZ_DATASOURCE_GRPC", getEnv("DATASOURCE_GRPC", "127.0.0.1:50053"))
	auditURL := getEnv("APP_LZ_AUDIT_URL", getEnv("AUDIT_URL", "http://127.0.0.1:8084"))
	auditGRPC := getEnv("APP_LZ_AUDIT_GRPC", getEnv("AUDIT_GRPC", "127.0.0.1:50054"))
	agentURL := getEnv("APP_LZ_AGENT_URL", getEnv("AGENT_URL", "http://127.0.0.1:8079"))
	agentGRPC := getEnv("APP_LZ_AGENT_GRPC", getEnv("AGENT_GRPC", "127.0.0.1:50051"))

	// ── 静态文件 & 日志 ──
	staticDir := getEnv("APP_LZ_STATIC_DIR", "./web/dist")
	logLevel := getEnv("APP_LZ_LOG_LEVEL", "info")

	// ── TLS 配置 ──
	// ParseBool 失败时静默回退到 false（不启用 TLS），避免因格式错误导致启动失败。
	tlsEnabled, _ := strconv.ParseBool(getEnv("APP_LZ_TLS_ENABLED", "false"))
	certFile := getEnv("APP_LZ_CERT_FILE", "")
	keyFile := getEnv("APP_LZ_KEY_FILE", "")
	clientCAFile := getEnv("APP_LZ_CLIENT_CA_FILE", "")

	// ── 认证 ──
	apiKey := getEnv("APP_LZ_API_KEY", "")

	return &Config{
		Host:           host,
		Port:           port,
		HubURL:         hubURL,
		HubGRPC:        hubGRPC,
		DatasourceURL:  datasourceURL,
		DatasourceGRPC: datasourceGRPC,
		AuditURL:       auditURL,
		AuditGRPC:      auditGRPC,
		AgentURL:       agentURL,
		AgentGRPC:      agentGRPC,
		StaticDir:      staticDir,
		LogLevel:       logLevel,
		TLSEnabled:     tlsEnabled,
		CertFile:       certFile,
		KeyFile:        keyFile,
		ClientCAFile:   clientCAFile,
		APIKey:         apiKey,
	}
}

// getEnv 读取单个环境变量，若未设置或为空则返回默认值。
// 这是配置加载的最小原子单元，被 Load() 多次调用来构建完整配置。
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// Validate 校验配置的一致性，在启动早期（fail-fast）发现致命配置错误。
//
// 当前校验规则：
//   - TLS 启用时，证书文件和私钥文件路径必须非空且在磁盘上可访问
//
// 返回 nil 表示配置合法，可以安全启动。
func (c *Config) Validate() error {
	if c.TLSEnabled {
		// TLS 开启但缺少证书/私钥路径 → 立即失败
		if c.CertFile == "" || c.KeyFile == "" {
			return fmt.Errorf("TLS enabled but APP_LZ_CERT_FILE and/or APP_LZ_KEY_FILE are empty")
		}
		// 确认证书文件在磁盘上存在且当前用户有权限读取
		if _, err := os.Stat(c.CertFile); err != nil {
			return fmt.Errorf("TLS cert file not accessible: %w", err)
		}
		// 确认私钥文件在磁盘上存在且当前用户有权限读取
		if _, err := os.Stat(c.KeyFile); err != nil {
			return fmt.Errorf("TLS key file not accessible: %w", err)
		}
	}
	return nil
}
