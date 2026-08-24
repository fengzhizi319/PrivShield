// Package config_test contains unit tests for the configuration loader of datasource-mgr.
// Package config_test 包含 datasource-mgr 模块运行时配置加载器的单元测试套件。
package config

import (
	"os"
	"testing"
)

// TestConfigDefaults verifies that Load() correctly falls back to default settings when no env vars are set.
// TestConfigDefaults 测试默认配置加载逻辑：
// 1. 显式清除相关的环境变量，确保测试环境纯净；
// 2. 调用 Load() 实例化配置对象；
// 3. 断言各个字段（HTTP Host/Port, gRPC Host/Port, TLS 启用状态, 拼接地址等）均符合预设的安全默认值。
func TestConfigDefaults(t *testing.T) {
	// 清理可能存在的主机与端口环境变量
	os.Unsetenv("DATASOURCE_MGR_HOST")
	os.Unsetenv("DATASOURCE_MGR_PORT")
	os.Unsetenv("DATASOURCE_MGR_GRPC_HOST")
	os.Unsetenv("DATASOURCE_MGR_GRPC_PORT")
	os.Unsetenv("DATASOURCE_MGR_TLS_ENABLED")

	cfg := Load()

	// 验证 HTTP 默认监听参数
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default Host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != 8083 {
		t.Errorf("expected default Port 8083, got %d", cfg.Port)
	}

	// 验证 gRPC 默认监听参数
	if cfg.GRPCHost != "127.0.0.1" {
		t.Errorf("expected default GRPCHost 127.0.0.1, got %s", cfg.GRPCHost)
	}
	if cfg.GRPCPort != 50053 {
		t.Errorf("expected default GRPCPort 50053, got %d", cfg.GRPCPort)
	}

	// 验证默认关闭 TLS
	if cfg.TLSEnabled {
		t.Errorf("expected default TLSEnabled to be false")
	}

	// 验证拼接地址格式
	if cfg.Address() != "127.0.0.1:8083" {
		t.Errorf("expected Address() 127.0.0.1:8083, got %s", cfg.Address())
	}
	if cfg.GRPCAddress() != "127.0.0.1:50053" {
		t.Errorf("expected GRPCAddress() 127.0.0.1:50053, got %s", cfg.GRPCAddress())
	}
}

// TestConfigCustomEnv verifies that Load() correctly parses custom environment variables.
// TestConfigCustomEnv 测试自定义环境变量覆盖逻辑：
// 1. 使用 t.Setenv 注入自定义的 HTTP、gRPC、mTLS 证书链、公钥固定、API Key 及跨域 CORS 等环境变量；
// 2. 调用 Load() 执行配置加载；
// 3. 逐项比对 Config 对象字段是否被自定义环境变量正确覆盖。
func TestConfigCustomEnv(t *testing.T) {
	// 注入自定义网络与安全配置环境变量
	t.Setenv("DATASOURCE_MGR_HOST", "0.0.0.0")
	t.Setenv("DATASOURCE_MGR_PORT", "9083")
	t.Setenv("DATASOURCE_MGR_GRPC_HOST", "0.0.0.0")
	t.Setenv("DATASOURCE_MGR_GRPC_PORT", "60053")
	t.Setenv("DATASOURCE_MGR_TLS_ENABLED", "true")
	t.Setenv("DATASOURCE_MGR_TLS_CERT_FILE", "/tmp/cert.pem")
	t.Setenv("DATASOURCE_MGR_TLS_KEY_FILE", "/tmp/key.pem")
	t.Setenv("DATASOURCE_MGR_TLS_CA_FILE", "/tmp/ca.pem")
	t.Setenv("DATASOURCE_MGR_TLS_CLIENT_AUTH", "require")
	t.Setenv("DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE", "/tmp/pinned.pem")
	t.Setenv("DATASOURCE_MGR_API_KEY", "secret-key")
	t.Setenv("DATASOURCE_MGR_CORS_ORIGINS", "http://localhost:3000,https://example.com")
	t.Setenv("DATASOURCE_MGR_LOG_FORMAT", "text")
	t.Setenv("DATASOURCE_MGR_LOG_LEVEL", "debug")

	cfg := Load()

	// 验证 HTTP 自定义监听地址
	if cfg.Host != "0.0.0.0" || cfg.Port != 9083 {
		t.Errorf("custom address mismatch: %s:%d", cfg.Host, cfg.Port)
	}

	// 验证 gRPC 自定义监听地址
	if cfg.GRPCHost != "0.0.0.0" || cfg.GRPCPort != 60053 {
		t.Errorf("custom grpc address mismatch: %s:%d", cfg.GRPCHost, cfg.GRPCPort)
	}

	// 验证 TLS 启用状态与证书链参数
	if !cfg.TLSEnabled {
		t.Errorf("expected TLSEnabled true")
	}
	if cfg.TLSCertFile != "/tmp/cert.pem" || cfg.TLSKeyFile != "/tmp/key.pem" {
		t.Errorf("custom tls cert/key mismatch")
	}
	if cfg.TLSCAFile != "/tmp/ca.pem" || cfg.TLSClientAuth != "require" {
		t.Errorf("custom tls ca/client auth mismatch")
	}
	if cfg.TLSPinnedPubKeyFile != "/tmp/pinned.pem" {
		t.Errorf("custom pinned pub key mismatch")
	}

	// 验证 API 鉴权密钥与 CORS 配置
	if cfg.APIKey != "secret-key" {
		t.Errorf("custom api key mismatch: %s", cfg.APIKey)
	}
	if len(cfg.CORSOrigins) != 2 {
		t.Errorf("expected 2 CORS origins, got %d", len(cfg.CORSOrigins))
	}
}
