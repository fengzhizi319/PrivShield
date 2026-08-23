# ============================================================================
# Datasource Manager — Multi-stage Dockerfile
# ============================================================================
# Build context: console/ (NOT datasource-mgr/)
# 构建上下文：console/ 目录（非 datasource-mgr/ 目录）
#
# docker build -f datasource-mgr/Dockerfile -t datasource-mgr:latest .
# (从 console/ 目录执行)
# ============================================================================

# Stage 1: Build
FROM golang:1.27-alpine3.21 AS builder

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct
ENV GOTOOLCHAIN=auto

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git ca-certificates

# Copy shared pkg/ first (required by replace ../pkg in go.mod)
COPY pkg/ ./pkg/

# Copy module dependencies for layer caching
COPY datasource-mgr/go.mod datasource-mgr/go.sum ./datasource-mgr/
WORKDIR /app/datasource-mgr
RUN go mod download

# Copy module source
COPY datasource-mgr/ ./
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

EXPOSE 8083

# P27 fix: declare data volume for SQLite persistence
# 声明数据卷用于 SQLite 持久化
VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8083/api/health || exit 1

USER privacy

ENV DATASOURCE_MGR_HOST=0.0.0.0
ENV DATASOURCE_MGR_PORT=8083
ENV PRIVACY_AGENT_REST_HOST=privshield-agent
ENV PRIVACY_REST_PORT=8079

CMD ["/app/server"]
