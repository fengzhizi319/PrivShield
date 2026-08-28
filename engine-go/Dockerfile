# ── Stage 1: Go 编译环境 ──
FROM golang:1.25-alpine3.21 AS builder

WORKDIR /build
ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0
ENV GOWORK=off

# 先拷贝 go.mod 利用缓存
COPY privacy-go-sdk/go.mod ./privacy-go-sdk/go.mod
COPY engine-go/go.mod ./engine-go/go.mod
RUN cd privacy-go-sdk && go mod download || true
RUN cd engine-go && go mod download || true

# 拷贝源码
COPY privacy-go-sdk/ ./privacy-go-sdk/
COPY engine-go/ ./engine-go/

# 编译 Agent 与 Gateway
RUN cd engine-go && \
    go build -ldflags="-s -w -X 'main.Version=6.0.0' -X 'main.BuildTime=$(date -u)'" \
    -o /build/bin/privshield-agent ./cmd/privshield-agent && \
    go build -ldflags="-s -w -X 'main.Version=6.0.0' -X 'main.BuildTime=$(date -u)'" \
    -o /build/bin/privshield-gateway ./cmd/privshield-gateway

# ── Stage 2: 极简运行时镜像 ──
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S privacy \
    && adduser -S privacy -G privacy

WORKDIR /app

COPY --from=builder /build/bin/privshield-agent /app/privshield-agent
COPY --from=builder /build/bin/privshield-gateway /app/privshield-gateway
COPY config/ /app/config/
COPY rules/ /app/rules/

RUN chown -R privacy:privacy /app

USER privacy

EXPOSE 8079 50051 8000 50000

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8079/health || exit 1

ENTRYPOINT ["/app/privshield-agent"]
