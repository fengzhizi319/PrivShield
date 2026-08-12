# Console Go Backend Multi-stage Dockerfile
# 锁定 Go 和 Alpine 版本，确保构建可追溯性
# Pinned Go and Alpine versions for build reproducibility

# ==============================================================================
# Stage 1: Build
# ==============================================================================
FROM golang:1.23.4-alpine3.20 AS builder

WORKDIR /app

# 安装构建依赖
# Install build dependencies
RUN apk add --no-cache git ca-certificates

# 先复制依赖文件，利用缓存
# Copy dependency files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# ==============================================================================
# Stage 2: Runtime (minimal attack surface)
# ==============================================================================
FROM alpine:3.20.3

WORKDIR /app

# 安装运行时依赖 + 创建非 root 用户
# Install runtime deps + create non-root user
RUN apk add --no-cache ca-certificates curl \
    && addgroup -S privacy \
    && adduser -S -G privacy -h /app -s /sbin/nologin privacy

# 复制编译产物
# Copy build artifact
COPY --from=builder /app/server /app/server

# 设置目录所有权
# Set directory ownership
RUN chown -R privacy:privacy /app

EXPOSE 8081

# 容器健康检查
# Container health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8081/health || exit 1

# 切换到非 root 用户
# Switch to non-root user
USER privacy

ENV PRIVACY_CONSOLE_HOST=0.0.0.0
ENV PRIVACY_CONSOLE_PORT=8081
ENV PRIVACY_AGENT_GRPC_HOST=privacy-local-agent
ENV PRIVACY_AGENT_GRPC_PORT=50051

CMD ["/app/server"]
