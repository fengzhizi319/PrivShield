# Console Go Backend Multi-stage Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 安装 git 和 ca-certificates
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 编译 Go 后端代理服务
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates curl

COPY --from=builder /app/server /app/server

EXPOSE 8081

ENV HOST=0.0.0.0
ENV PORT=8081
ENV PRIVACY_AGENT_GRPC_ADDR=privacy-local-agent:50051
ENV PRIVACY_AGENT_REST_ADDR=privacy-local-agent:8079

CMD ["/app/server"]
