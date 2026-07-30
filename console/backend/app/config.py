"""Privacy 测试控制台后端的配置模块。
Configuration module for the Privacy Test Console backend.

基于 ``pydantic-settings`` 从环境变量（可选 ``.env`` 文件）加载配置，
Loads configuration from environment variables (optional ``.env`` file) via ``pydantic-settings``,
所有项均有默认值，本地开发零配置即可运行。
all fields have defaults, enabling zero-config local development.
"""

from __future__ import annotations

from pathlib import Path

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """从环境变量加载的配置集合。
    Configuration collection loaded from environment variables.

    所有环境变量均为可选，默认值面向本地开发场景（agent 运行在
    All env vars are optional; defaults target local dev (agent running at
    默认的 ``127.0.0.1:8079``）。字段通过 ``alias`` 映射到环境变量名。
    default ``127.0.0.1:8079``). Fields map to env var names via ``alias``.
    """

    # 下游 privacy-local-agent 的 REST 基地址 / Downstream privacy-local-agent REST base URL
    privacy_agent_url: str = Field(default="http://127.0.0.1:8079", alias="PRIVACY_AGENT_URL")
    # 可选的认证 API Key（agent 开启 auth 时必填）/ Optional auth API Key (required when agent auth is enabled)
    privacy_agent_api_key: str | None = Field(default=None, alias="PRIVACY_AGENT_API_KEY")
    # 控制台后端监听地址 / Console backend listen host
    console_host: str = Field(default="127.0.0.1", alias="PRIVACY_CONSOLE_HOST")
    # 控制台后端监听端口 / Console backend listen port
    console_port: int = Field(default=8080, alias="PRIVACY_CONSOLE_PORT")
    # 前端构建产物目录（用于静态 SPA 托管）/ Frontend dist directory (for static SPA hosting)
    static_dist_dir: Path = Field(default=Path("../web/dist"), alias="PRIVACY_CONSOLE_STATIC_DIR")

    # ── 可选安全加固配置（默认关闭 / 宽松）──────────────────────
    # Optional security hardening config (disabled/relaxed by default)
    # 控制台 API Key：设置后需携带 Authorization: Bearer <key>
    # Console API Key: when set, requires Authorization: Bearer <key>
    console_api_key: str | None = Field(default=None, alias="CONSOLE_API_KEY")
    # 限流：每分钟每 IP 最大请求数（0=关闭）/ Rate limit: max requests/min/IP (0=disabled)
    console_rate_limit: int = Field(default=600, alias="CONSOLE_RATE_LIMIT")
    # 上传文件大小上限（字节）/ Max upload file size (bytes)
    max_upload_bytes: int = Field(default=10 * 1024 * 1024, alias="CONSOLE_MAX_UPLOAD_BYTES")
    # 负载均衡探测 host 白名单（逗号分隔）/ LB probe host allowlist (comma-separated)
    lb_allowed_hosts: str | None = Field(default=None, alias="LB_ALLOWED_HOSTS")

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        populate_by_name=True,
    )


# 全局配置单例：模块导入时即完成环境变量解析。
# Global config singleton: env vars are parsed at module import time.
settings = Settings()
