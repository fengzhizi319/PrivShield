# ==============================================================================
# PrivShield 多阶段构建（Multi-stage Build）
# ==============================================================================
FROM python:3.13-slim-bookworm AS base

WORKDIR /app

RUN (sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources 2>/dev/null || sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list 2>/dev/null || true) \
    && apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r privacy \
    && useradd -r -g privacy -d /app -s /sbin/nologin privacy

COPY requirements-core.txt .
RUN pip install --no-cache-dir -r requirements-core.txt -i https://mirrors.aliyun.com/pypi/simple/ --extra-index-url https://pypi.tuna.tsinghua.edu.cn/simple

# ==============================================================================
# Stage 2: core —— 默认轻量运行镜像（推荐）
# ==============================================================================
FROM base AS core

COPY engine/ ./engine/
RUN ln -s engine PrivShield
COPY rules/ ./rules/
COPY config/ ./config/
COPY proto/ ./proto/
COPY docs/standard/ ./docs/standard/
COPY pyproject.toml requirements-core.txt ./
COPY scripts/ ./scripts/

COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

RUN mkdir -p /data/budget /var/log/privacy \
    && chown -R privacy:privacy /app /data /var/log/privacy

EXPOSE 8079 50051

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8079/health || exit 1

USER privacy

ENV PYTHONUNBUFFERED=1
ENV PRIVACY_REST_HOST=0.0.0.0
ENV PRIVACY_GRPC_HOST=0.0.0.0

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["python", "-m", "engine.server"]

# ==============================================================================
# Stage 3: ml —— 完整 ML 镜像（torch / transformers / onnxruntime）
# ==============================================================================
FROM core AS ml

USER root
COPY requirements-ml.txt .
RUN pip install --no-cache-dir -r requirements-ml.txt -i https://mirrors.aliyun.com/pypi/simple/ --extra-index-url https://pypi.tuna.tsinghua.edu.cn/simple \
    && find /usr/local/lib/python*/site-packages -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
USER privacy

CMD ["python", "-m", "engine.server"]
