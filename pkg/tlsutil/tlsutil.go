// Package tlsutil provides shared TLS configuration utilities for building secure server credentials.
// Package tlsutil 提供共享的 TLS 配置工具函数，用于构建安全的服务器端凭证。
//
// 核心能力：
// 1. 统一 TLS 配置构建：支持 TLS 1.3 强制最低版本、mTLS 双向认证、公钥固定（SPKI Pinning）；
// 2. 多客户端认证模式：require/requireandverify（强制双向校验）、verify（可选校验）、request（请求证书）；
// 3. 公钥固定防御：支持 RSA、ECDSA、Ed25519 客户端公钥比对，防御 CA 劫持与证书伪造攻击；
// 4. 跨协议复用：同时支持 gRPC 和 HTTP 服务器，避免代码重复。
package tlsutil

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServerTLSConfig holds the configuration parameters for building a TLS server config.
// ServerTLSConfig 保存构建 TLS 服务器配置所需的参数。
type ServerTLSConfig struct {
	Enabled          bool   // 是否启用 TLS
	CertFile         string // 服务端 X.509 证书 PEM 文件路径
	KeyFile          string // 服务端私钥 PEM 文件路径
	CAFile           string // 验证客户端身份的受信任根 CA 证书路径
	ClientAuth       string // 客户端认证模式："require" | "verify" | "request" | ""
	PinnedPubKeyFile string // 固定的客户端 RSA 公钥 PEM 文件路径（可选）
}

// BuildServerTLSConfig constructs a *tls.Config supporting TLS 1.3, mTLS client auth, and public key pinning.
// BuildServerTLSConfig 根据配置构建 TLS 服务器配置：
// 1. 加载服务端证书与私钥，强制启用 TLS 1.3 最低版本；
// 2. 若配置了 ClientAuth，挂载根 CA 证书池，设置 RequireAndVerifyClientCert 双向认证策略；
// 3. 若配置了 PinnedPubKeyFile，注入 VerifyPeerCertificate 验证钩子，严格比对客户端公钥指纹。
func BuildServerTLSConfig(cfg *ServerTLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("TLS is disabled in configuration")
	}
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil, fmt.Errorf("TLS cert file and key file must be configured")
	}

	certFile := filepath.Clean(cfg.CertFile)
	keyFile := filepath.Clean(cfg.KeyFile)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load server x509 key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	clientAuthMode := strings.ToLower(strings.TrimSpace(cfg.ClientAuth))
	if clientAuthMode != "" {
		if cfg.CAFile == "" {
			return nil, fmt.Errorf("TLS CA file must be configured when client auth is enabled")
		}
		caFile := filepath.Clean(cfg.CAFile)
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS CA file: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", caFile)
		}
		tlsConfig.ClientCAs = caPool

		switch clientAuthMode {
		case "require", "requireandverify":
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		case "verify":
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		case "request":
			tlsConfig.ClientAuth = tls.RequestClientCert
		default:
			return nil, fmt.Errorf("unknown TLS client auth mode: %s", cfg.ClientAuth)
		}
	}

	// 注入公钥固定校验器
	if cfg.PinnedPubKeyFile != "" {
		pinnedFile := filepath.Clean(cfg.PinnedPubKeyFile)
		pinnedKey, err := LoadPublicKey(pinnedFile)
		if err != nil {
			return nil, fmt.Errorf("load pinned client public key: %w", err)
		}
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("mTLS: client did not present a certificate")
			}
			peerCert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("mTLS: failed to parse peer certificate: %w", err)
			}
			if !PublicKeysEqual(peerCert.PublicKey, pinnedKey) {
				return fmt.Errorf("mTLS: client public key does not match pinned key")
			}
			return nil
		}
	}

	return tlsConfig, nil
}

// LoadPublicKey loads a public key from PEM file (supports PKIX and X.509 Certificate formats).
// LoadPublicKey 从 PEM 格式文件中解析并提取公钥对象。
func LoadPublicKey(path string) (crypto.PublicKey, error) {
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM data found in %s", path)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		cert, certErr := x509.ParseCertificate(block.Bytes)
		if certErr == nil {
			return cert.PublicKey, nil
		}
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return pub, nil
}

// PublicKeysEqual checks if two public keys are identical (RSA, ECDSA, Ed25519).
// PublicKeysEqual 深度比对两个公钥的数学属性（支持 RSA 模数/指数、ECDSA 椭圆曲线坐标与 Ed25519 字节）。
func PublicKeysEqual(a, b crypto.PublicKey) bool {
	switch keyA := a.(type) {
	case *rsa.PublicKey:
		keyB, ok := b.(*rsa.PublicKey)
		if !ok {
			return false
		}
		return keyA.N.Cmp(keyB.N) == 0 && keyA.E == keyB.E
	case *ecdsa.PublicKey:
		keyB, ok := b.(*ecdsa.PublicKey)
		if !ok {
			return false
		}
		return keyA.X.Cmp(keyB.X) == 0 && keyA.Y.Cmp(keyB.Y) == 0 && keyA.Curve == keyB.Curve
	case ed25519.PublicKey:
		keyB, ok := b.(ed25519.PublicKey)
		if !ok {
			return false
		}
		return keyA.Equal(keyB)
	default:
		return false
	}
}
