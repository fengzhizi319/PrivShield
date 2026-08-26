// Package config 的单元测试。
//
// 测试策略：
//   - TestConfigDefaults: 验证无环境变量时的默认值是否正确
//   - TestConfigCustomEnv: 验证通过环境变量覆盖配置是否生效
package config

import (
	"os"
	"testing"
)

// TestConfigDefaults 验证在所有环境变量均未设置时，
// Load() 返回的配置对象包含正确的硬编码默认值。
//
// 测试步骤：
//  1. 清除关键环境变量（确保不受外部环境干扰）
//  2. 调用 Load() 加载配置
//  3. 逐一检查默认值：Host=0.0.0.0, Port=8085, HubURL=http://127.0.0.1:8082, TLS=off
func TestConfigDefaults(t *testing.T) {
	// 清除环境变量，确保测试在干净环境下运行
	os.Unsetenv("APP_LZ_HOST")
	os.Unsetenv("APP_LZ_PORT")
	os.Unsetenv("APP_LZ_HUB_URL")

	cfg := Load()
	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}
	if cfg.Port != "8085" {
		t.Errorf("expected port 8085, got %s", cfg.Port)
	}
	if cfg.HubURL != "http://127.0.0.1:8082" {
		t.Errorf("expected hub url http://127.0.0.1:8082, got %s", cfg.HubURL)
	}
	if cfg.TLSEnabled != false {
		t.Errorf("expected tls disabled by default")
	}
}

// TestConfigCustomEnv 验证通过环境变量覆盖配置时能正确生效。
//
// 测试步骤：
//  1. 设置 APP_LZ_PORT=9095 和 APP_LZ_TLS_ENABLED=true
//  2. 调用 Load() 加载配置
//  3. 验证 Port 和 TLSEnabled 已被覆盖
//  4. 使用 defer 在测试结束后清除环境变量（避免污染其他测试）
func TestConfigCustomEnv(t *testing.T) {
	// 设置自定义环境变量
	os.Setenv("APP_LZ_PORT", "9095")
	os.Setenv("APP_LZ_TLS_ENABLED", "true")
	// 测试结束后清理，避免影响其他测试用例
	defer func() {
		os.Unsetenv("APP_LZ_PORT")
		os.Unsetenv("APP_LZ_TLS_ENABLED")
	}()

	cfg := Load()
	if cfg.Port != "9095" {
		t.Errorf("expected port 9095, got %s", cfg.Port)
	}
	if !cfg.TLSEnabled {
		t.Errorf("expected tls enabled")
	}
}
