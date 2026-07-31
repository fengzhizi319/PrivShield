"""Deployment 部署产物验证测试 / Deployment Artifact Validation Tests.

中文说明：
本模块验证 Helm chart 与原生 K8s manifests 的语法正确性，不依赖实际集群：

1. Helm Chart 验证 / Helm Chart Validation:
   - helm lint: 检查 chart 结构、模板语法、values 一致性
   - helm template (default): 默认 values 渲染为合法 YAML
   - helm template (production): 生产 values 渲染并验证 TLS/Auth 环境变量

2. K8s Manifests 验证 / K8s Manifests Validation:
   - deploy/k8s/ 目录下所有 YAML 文件可被正确解析
   - 每个文件至少包含一个非空 YAML 文档

测试策略：
- helm 相关测试通过 skipif 标记，未安装 helm 时自动跳过
- K8s YAML 验证仅依赖 PyYAML，无外部工具依赖

English Description:
Validates Helm chart and raw K8s manifests syntax correctness
without requiring an actual cluster. Helm tests are skipped when
helm binary is not found in PATH.
"""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest
import yaml

# 项目根目录（从测试文件位置向上 3 层）/ Project root (3 levels up from test file)
PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
# Helm chart 目录 / Helm chart directory
HELM_DIR = PROJECT_ROOT / "deploy" / "helm" / "privacy-local-agent"
# 原生 K8s manifests 目录 / Raw K8s manifests directory
K8S_DIR = PROJECT_ROOT / "deploy" / "k8s"

# 检测 helm 可执行文件是否在 PATH 中 / Check if helm binary is in PATH
HELM = shutil.which("helm")


@pytest.mark.skipif(HELM is None, reason="helm not found in PATH")
def test_helm_lint() -> None:
    """验证 helm lint 通过：检查 chart 结构、模板语法、values 引用一致性。

    helm lint 会验证：
    - Chart.yaml 必填字段完整
    - templates/ 下的模板可渲染
    - values.yaml 中的引用路径有效
    """
    result = subprocess.run(
        [HELM, "lint", str(HELM_DIR)],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stdout + result.stderr


@pytest.mark.skipif(HELM is None, reason="helm not found in PATH")
def test_helm_template_default_values() -> None:
    """验证默认 values 渲染产生合法 YAML 文档。

    helm template 在本地渲染全部模板（不需要集群），
    输出应为多个 YAML 文档（Deployment, Service, ConfigMap 等）。
    """
    result = subprocess.run(
        [HELM, "template", "test", str(HELM_DIR)],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    # Verify at least one YAML document parses.
    docs = list(yaml.safe_load_all(result.stdout))
    assert any(doc for doc in docs)


@pytest.mark.skipif(HELM is None, reason="helm not found in PATH")
def test_helm_template_production_values() -> None:
    """验证生产模式 values 渲染并包含 TLS/Auth 环境变量。

    生产模式启用 PRIVACY_TLS_ENABLED 和 PRIVACY_AUTH_ENABLED，
    需要外部 Secret 提供证书和 API Key。
    """
    values_file = HELM_DIR / "values-production.yaml"
    result = subprocess.run(
        [
            HELM,
            "template",
            "prod",
            str(HELM_DIR),
            "-f",
            str(values_file),
            "--set",
            "security.tls.existingSecret=tls-secret",
            "--set",
            "security.auth.apiKeysSecret=keys-secret",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    docs = list(yaml.safe_load_all(result.stdout))
    # Find Deployment and assert TLS/auth env vars are present.
    deployment = next(
        (d for d in docs if isinstance(d, dict) and d.get("kind") == "Deployment"),
        None,
    )
    assert deployment is not None
    containers = deployment["spec"]["template"]["spec"]["containers"]
    env_names = {
        env["name"]
        for c in containers
        for env in c.get("env", [])
    }
    assert "PRIVACY_TLS_ENABLED" in env_names
    assert "PRIVACY_AUTH_ENABLED" in env_names


def test_k8s_manifests_are_valid_yaml() -> None:
    """验证 deploy/k8s/ 下所有 YAML 文件可被正确解析。

    包括 deployment.yaml, service.yaml, configmap.yaml,
    namespace.yaml, kustomization.yaml, secret.example.yaml。
    """
    for path in K8S_DIR.glob("*.yaml"):
        # Skip example secrets and kustomization; they are still valid YAML though.
        with path.open("r", encoding="utf-8") as f:
            content = f.read()
        # kustomization.yaml may be empty-ish; safe_load_all handles it.
        docs = list(yaml.safe_load_all(content))
        # Ensure at least one non-None document.
        assert any(d is not None for d in docs), f"{path} contains no documents"
