"""网关服务器启动器与配置加载单元测试 (Unit tests for gateway server & config loader).

覆盖以下测试点：
1. **load_config 配置加载与层级合并**：
   - 默认配置缺省值回退；
   - YAML 配置文件加载与字段合并；
   - 环境变量优先覆盖（HOST/PORT/STRATEGY/INTERVAL/TLS）；
   - GATEWAY_BACKENDS 字符串解析（逗号与竖线分割）；
2. **start_grpc_gateway 服务启动器**：
   - 开启 TLS 但未提供证书/私钥时触发 Fail-Fast（抛出 ValueError）；
   - 明文模式成功创建 grpc.aio.Server 并绑定 Insecure Port（可启动与停止）；
   - 启用 TLS / mTLS 时通过 grpc.ssl_server_credentials 正常启动。
"""

from __future__ import annotations

import socket
import pytest
import yaml

from PrivShield.gateway.balancer import LoadBalancer
from PrivShield.gateway.grpc_proxy import start_grpc_gateway
from PrivShield.gateway.server import load_config


def find_free_port() -> int:
    """获取本地可用端口。"""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.bind(("", 0))
    port = s.getsockname()[1]
    s.close()
    return port


@pytest.fixture()
def _clean_env(monkeypatch):
    """清理所有网关相关的环境变量。"""
    for var in (
        "PRIVACY_GATEWAY_CONFIG",
        "GATEWAY_REST_HOST",
        "GATEWAY_REST_PORT",
        "GATEWAY_GRPC_HOST",
        "GATEWAY_GRPC_PORT",
        "GATEWAY_STRATEGY",
        "GATEWAY_HEALTH_INTERVAL",
        "GATEWAY_TLS_ENABLED",
        "GATEWAY_TLS_CERT",
        "GATEWAY_TLS_KEY",
        "GATEWAY_TLS_CA",
        "GATEWAY_BACKENDS",
    ):
        monkeypatch.delenv(var, raising=False)


def test_load_config_defaults(_clean_env):
    """测试默认配置项。"""
    cfg = load_config()
    gw = cfg["gateway"]
    assert gw["rest_host"] == "0.0.0.0"
    assert gw["rest_port"] == 8000
    assert gw["grpc_host"] == "0.0.0.0"
    assert gw["grpc_port"] == 50000
    assert gw["strategy"] == "round_robin"
    assert gw["health_check_interval"] == 5.0
    assert gw["tls_enabled"] is False
    assert cfg["backends"] == []


def test_load_config_from_yaml(_clean_env, monkeypatch, tmp_path):
    """测试从 YAML 配置文件中加载配置。"""
    yaml_file = tmp_path / "gateway.yaml"
    content = {
        "gateway": {
            "rest_port": 8888,
            "grpc_port": 58888,
            "strategy": "least_connections",
            "health_check_interval": 3.0,
        },
        "backends": [
            {"http_url": "http://10.0.0.1:8079", "grpc_address": "10.0.0.1:50051", "weight": 5}
        ],
    }
    yaml_file.write_text(yaml.dump(content), encoding="utf-8")
    monkeypatch.setenv("PRIVACY_GATEWAY_CONFIG", str(yaml_file))

    cfg = load_config()
    gw = cfg["gateway"]
    assert gw["rest_port"] == 8888
    assert gw["grpc_port"] == 58888
    assert gw["strategy"] == "least_connections"
    assert gw["health_check_interval"] == 3.0
    assert len(cfg["backends"]) == 1
    assert cfg["backends"][0]["weight"] == 5


def test_load_config_env_overrides(_clean_env, monkeypatch):
    """测试环境变量覆盖配置。"""
    monkeypatch.setenv("GATEWAY_REST_HOST", "127.0.0.1")
    monkeypatch.setenv("GATEWAY_REST_PORT", "9000")
    monkeypatch.setenv("GATEWAY_GRPC_PORT", "60000")
    monkeypatch.setenv("GATEWAY_STRATEGY", "weighted_round_robin")
    monkeypatch.setenv("GATEWAY_HEALTH_INTERVAL", "2.5")
    monkeypatch.setenv("GATEWAY_TLS_ENABLED", "true")
    monkeypatch.setenv("GATEWAY_TLS_CERT", "/path/to/cert.crt")
    monkeypatch.setenv("GATEWAY_TLS_KEY", "/path/to/key.key")
    monkeypatch.setenv("GATEWAY_TLS_CA", "/path/to/ca.crt")
    monkeypatch.setenv(
        "GATEWAY_BACKENDS",
        "http://agent-1:8079|agent-1:50051,http://agent-2:8080|agent-2:50052",
    )

    cfg = load_config()
    gw = cfg["gateway"]
    assert gw["rest_host"] == "127.0.0.1"
    assert gw["rest_port"] == 9000
    assert gw["grpc_port"] == 60000
    assert gw["strategy"] == "weighted_round_robin"
    assert gw["health_check_interval"] == 2.5
    assert gw["tls_enabled"] is True
    assert gw["tls_cert_file"] == "/path/to/cert.crt"
    assert gw["tls_key_file"] == "/path/to/key.key"
    assert gw["tls_ca_file"] == "/path/to/ca.crt"

    backends = cfg["backends"]
    assert len(backends) == 2
    assert backends[0]["http_url"] == "http://agent-1:8079"
    assert backends[0]["grpc_address"] == "agent-1:50051"
    assert backends[1]["http_url"] == "http://agent-2:8080"
    assert backends[1]["grpc_address"] == "agent-2:50052"


@pytest.mark.anyio
async def test_start_grpc_gateway_tls_missing_cert_raises():
    """开启 TLS 但未提供证书/私钥时必须 Fail-Fast 抛出 ValueError。"""
    balancer = LoadBalancer()
    with pytest.raises(ValueError, match="tls_cert_file and/or tls_key_file are missing"):
        await start_grpc_gateway(
            host="127.0.0.1",
            port=50000,
            balancer=balancer,
            tls_enabled=True,
            tls_cert_file="",
            tls_key_file="",
        )


@pytest.mark.anyio
async def test_start_grpc_gateway_insecure_lifecycle():
    """测试明文 gRPC 网关成功启动、绑定端口并正常停止。"""
    port = find_free_port()
    balancer = LoadBalancer()
    server = await start_grpc_gateway(
        host="127.0.0.1",
        port=port,
        balancer=balancer,
        tls_enabled=False,
    )
    assert server is not None
    await server.stop(grace=0.1)


@pytest.mark.anyio
async def test_start_grpc_gateway_tls_lifecycle(tmp_path):
    """测试自签名证书下的 TLS/mTLS gRPC 网关启动与停止。"""
    import subprocess
    import os
    # 生成真实的测试自签名证书
    cert = tmp_path / "server.crt"
    key = tmp_path / "server.key"
    ca = tmp_path / "ca.crt"
    ca_key = tmp_path / "ca.key"
    clean_env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin")}

    subprocess.run(
        [
            "openssl", "req", "-x509", "-newkey", "rsa:2048", "-days", "1", "-nodes",
            "-keyout", str(ca_key), "-out", str(ca),
            "-subj", "/CN=TestCA",
        ],
        check=True,
        capture_output=True,
        env=clean_env,
    )

    subprocess.run(
        [
            "openssl", "req", "-newkey", "rsa:2048", "-nodes",
            "-keyout", str(key), "-out", str(tmp_path / "server.csr"),
            "-subj", "/CN=localhost",
        ],
        check=True,
        capture_output=True,
        env=clean_env,
    )

    subprocess.run(
        [
            "openssl", "x509", "-req", "-days", "1",
            "-in", str(tmp_path / "server.csr"),
            "-CA", str(ca), "-CAkey", str(ca_key), "-CAcreateserial",
            "-out", str(cert),
        ],
        check=True,
        capture_output=True,
        env=clean_env,
    )

    port = find_free_port()
    balancer = LoadBalancer()
    server = await start_grpc_gateway(
        host="127.0.0.1",
        port=port,
        balancer=balancer,
        tls_enabled=True,
        tls_cert_file=str(cert),
        tls_key_file=str(key),
        tls_ca_file=str(ca),
    )
    assert server is not None
    await server.stop(grace=0.1)
