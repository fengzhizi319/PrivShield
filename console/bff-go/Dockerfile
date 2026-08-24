# ============================================================================
# Console Go Backend — Multi-stage Dockerfile
# ============================================================================
# Build context: . (Project root)
# 构建上下文：项目根目录
#
# docker build -f console/bff-go/Dockerfile -t bff-go:latest .
#
# 本 Dockerfile 采用多阶段构建（Multi-stage Build），将「编译」和「运行」分离到
# 两个独立的基础镜像中。最终镜像仅包含静态二进制 + 最少系统库 + 非 root 用户。

# ==============================================================================
# Stage 1: Build（构建阶段）
# ==============================================================================
FROM golang:1.27-alpine3.21 AS builder

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct
ENV GOTOOLCHAIN=auto

# 安装构建依赖
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git ca-certificates

# Copy shared pkg/
COPY pkg/ ./pkg/

# 先复制模块依赖文件，利用缓存
COPY console/bff-go/go.mod console/bff-go/go.sum ./console/bff-go/
WORKDIR /app/console/bff-go
RUN go mod download

# 复制模块源码并编译
COPY console/bff-go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server ./cmd/server

# ==============================================================================
# Stage 2: Runtime（运行阶段 — 最小化攻击面）
# ==============================================================================
FROM alpine:3.21

WORKDIR /app

# 安装运行时依赖 + 创建非 root 用户
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates curl \
    && addgroup -S privacy \
    && adduser -S -G privacy -h /app -s /sbin/nologin privacy

# 复制编译产物（多阶段构建）
COPY --from=builder /app/server /app/server

# 设置目录所有权
# Set directory ownership
RUN chown -R privacy:privacy /app

EXPOSE 8081

# 容器健康检查
# Container health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8081/api/health || exit 1

# 切换到非 root 用户
# Switch to non-root user
USER privacy

ENV PRIVACY_CONSOLE_HOST=0.0.0.0
ENV PRIVACY_CONSOLE_PORT=8081
ENV PRIVACY_AGENT_GRPC_HOST=PrivShield
ENV PRIVACY_AGENT_GRPC_PORT=50051

CMD ["/app/server"]
