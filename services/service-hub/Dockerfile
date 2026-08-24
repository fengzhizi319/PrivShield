# ============================================================================
# Service Hub — Multi-stage Dockerfile
# ============================================================================
# Build context: . (Project root)
# 构建上下文：项目根目录
#
# docker build -f services/service-hub/Dockerfile -t service-hub:latest .
# ============================================================================

# Stage 1: Build
FROM golang:1.27-alpine3.21 AS builder

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct
ENV GOTOOLCHAIN=auto

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git ca-certificates

# Copy shared pkg/
COPY pkg/ ./pkg/

# Copy module dependencies for layer caching
COPY services/service-hub/go.mod services/service-hub/go.sum ./services/service-hub/
WORKDIR /app/services/service-hub
RUN go mod download

# Copy module source
COPY services/service-hub/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server ./cmd/server

# Stage 2: Runtime
FROM alpine:3.21

WORKDIR /app

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates curl \
    && addgroup -S privacy \
    && adduser -S -G privacy -h /app -s /sbin/nologin privacy

COPY --from=builder /app/server /app/server

RUN chown -R privacy:privacy /app

EXPOSE 8082

# P27 fix: declare data volume for SQLite persistence
# 声明数据卷用于 SQLite 持久化（部署时挂载到宿主机）
VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8082/api/health || exit 1

USER privacy

ENV SERVICE_HUB_HOST=0.0.0.0
ENV SERVICE_HUB_PORT=8082
ENV PRIVACY_AGENT_REST_HOST=privshield-agent
ENV PRIVACY_REST_PORT=8079

CMD ["/app/server"]
