# privacy-local-agent 多阶段构建
# 支持两种构建目标：
#   --target core : 轻量镜像，仅含隐私原语（DP / K-匿名 / 分类规则接口）
#   --target ml   : 完整镜像，额外包含 torch / transformers / onnxruntime，用于本地 LLM/NER 分类
#
# 示例：
#   docker build --target core -t privacy-local-agent:0.1.0 .
#   docker build --target ml -t privacy-local-agent:0.1.0-ml .

# ==============================================================================
# Base: 锁定 Debian Bookworm 版本，确保可追溯性
# ==============================================================================
FROM python:3.10.14-slim-bookworm AS base

WORKDIR /app

# 安装基础系统工具与 curl（用于 K8s 探针）
# 创建非 root 运行用户（安全最佳实践）
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r privacy \
    && useradd -r -g privacy -d /app -s /sbin/nologin privacy

# 先安装核心依赖，利用镜像缓存
COPY requirements-core.txt .
RUN pip install --no-cache-dir -r requirements-core.txt

# ------------------- core 目标 -------------------
FROM base AS core

# 分层 COPY：仅包含运行时必需文件，排除测试/文档/开发产物
COPY privacy_local_agent/ ./privacy_local_agent/
COPY rules/ ./rules/
COPY config/ ./config/
COPY proto/ ./proto/
COPY pyproject.toml requirements-core.txt ./
COPY scripts/ ./scripts/

# 复制 entrypoint 脚本
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# 设置目录所有权
RUN chown -R privacy:privacy /app

EXPOSE 8079 50051

# 容器健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8079/health || exit 1

# 切换到非 root 用户
USER privacy

ENV PYTHONUNBUFFERED=1
ENV PRIVACY_REST_HOST=0.0.0.0
ENV PRIVACY_GRPC_HOST=0.0.0.0

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["python", "-m", "privacy_local_agent.server"]

# ------------------- ml 目标 -------------------
FROM core AS ml

USER root
COPY requirements-ml.txt .
RUN pip install --no-cache-dir -r requirements-ml.txt \
    && find /usr/local/lib/python*/site-packages -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
USER privacy

CMD ["python", "-m", "privacy_local_agent.server"]
