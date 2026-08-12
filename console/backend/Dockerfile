# Console Python Backend Dockerfile
# 锁定 Python 版本（3.13，与主 agent 镜像统一），确保构建可追溯性
# Pinned Python version for build reproducibility

FROM python:3.13.13-slim-bookworm

WORKDIR /app

# 安装系统依赖 + 创建非 root 用户
# Install system deps + create non-root user
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r privacy \
    && useradd -r -g privacy -d /app -s /sbin/nologin privacy

# 先复制依赖文件，利用缓存
# Copy dependency files first for layer caching
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# 复制应用代码
# Copy application code
COPY . .

# 设置目录所有权
# Set directory ownership
RUN chown -R privacy:privacy /app

EXPOSE 8080

# 容器健康检查
# Container health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# 切换到非 root 用户
# Switch to non-root user
USER privacy

ENV PYTHONUNBUFFERED=1
ENV PRIVACY_CONSOLE_HOST=0.0.0.0
ENV PRIVACY_CONSOLE_PORT=8080
ENV PRIVACY_AGENT_URL=http://privacy-local-agent:8079

CMD ["python", "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8080"]
