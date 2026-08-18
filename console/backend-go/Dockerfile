# =============================================================================
# Console Go Backend — Multi-stage Dockerfile
# =============================================================================
#
# 本 Dockerfile 采用多阶段构建（Multi-stage Build），将「编译」和「运行」分离到
# 两个独立的基础镜像中，核心目的：
#
#   - 构建阶段（Stage 1）需要完整的 Go 编译器、标准库和 git 等工具
#   - 运行阶段（Stage 2）只需要一个静态链接的二进制 + 极少的系统库
#
# 最终产出的运行镜像不包含 Go 编译器、源码及中间产物，仅保留：
#   1. 编译好的静态链接二进制 /app/server
#   2. CA 根证书（用于 HTTPS/TLS 出站请求）
#   3. curl（用于 HEALTHCHECK 健康探针）
#   4. 非 root 用户 privacy（安全最佳实践，最小权限运行）
#
# 运行时依赖分析（Runtime dependency analysis）：
#   必须：
#     - /app/server 二进制（核心，所有业务逻辑已编译入内）
#       包括：gRPC 客户端、HTTP 路由、mapper、示例数据等
#   不需要：
#     - 配置文件 — config.Load() 全部从环境变量读取，零配置文件依赖
#     - Go 运行时/编译器 — CGO_ENABLED=0 静态二进制无外部依赖
#     - embed 嵌入资源 — 示例数据（samples）硬编码在 Go 源码中
#   可选（通过环境变量或 volume 挂载启用）：
#     - 前端静态文件（PRIVACY_CONSOLE_STATIC_DIR）— 启用 Console UI 托管
#     - TLS 证书文件 — 启用 mTLS 双向认证时通过 volume 挂载传入
#
# 两阶段对比：
#   ┌──────────────┬───────────────────────────┬──────────────────────────────┐
#   │              │ Stage 1: Builder          │ Stage 2: Runtime             │
#   ├──────────────┼───────────────────────────┼──────────────────────────────┤
#   │ 基础镜像       │ golang:1.23.4-alpine3.20  │ alpine:3.20.3 (~7MB)         │
#   │ 包含内容       │ Go 编译器 + 源码 + 依赖      │ 仅静态二进制 + 最少系统库         │
#   │ 镜像体积       │ ~300MB（仅构建用）          │ ~15MB（最终交付）               │
#   │ 是否进入最终    │ 否，构建完即丢弃             │ 是，作为生产运行镜像              │
#   │ 安全暴露面     │ 大（含编译器、git 等）        │ 极小（无开发工具）               │
#   └──────────────┴───────────────────────────┴──────────────────────────────┘
#
# 锁定 Go 和 Alpine 版本，确保构建可追溯性
# Pinned Go and Alpine versions for build reproducibility
# =============================================================================

# ==============================================================================
# Stage 1: Build（构建阶段）
# ==============================================================================
# 基础镜像：golang:1.23.4-alpine3.20
#   - 包含完整的 Go 1.23.4 编译器、标准库和 go mod 工具
#   - 基于 Alpine 3.20，体积远小于 debian-based 的 Go 镜像（~300MB vs ~900MB）
#   - 此阶段安装 git（用于 go mod 拉取依赖）并下载所有 Go module
#   - 编译产出：CGO_ENABLED=0 的静态链接 Linux ELF 二进制，无 glibc 依赖
#   - 该阶段的所有内容（源码、依赖缓存、编译中间文件）不会进入最终镜像
# ==============================================================================
#指定基础镜像
FROM golang:1.23.4-alpine3.20 AS builder

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct
ENV GOTOOLCHAIN=auto

# 安装构建依赖
# Install build dependencies
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git ca-certificates

# 先复制依赖文件，利用缓存
# Copy dependency files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# ==============================================================================
# Stage 2: Runtime（运行阶段 — 最小化攻击面）
# ==============================================================================
# 基础镜像：alpine:3.20.3（仅 ~7MB 的最小 Linux 用户空间）
#   - 不含任何编译器或开发工具，与 Stage 1 完全隔离
#   - 通过 COPY --from=builder 仅取走编译好的二进制文件
#   - 不包含 Go 源码、go mod 缓存、git 等，大幅减少镜像层数和漏洞暴露面
#   - 创建专用非 root 用户 privacy 运行服务，遵循最小权限原则
#   - 仅安装运行时必需的最小依赖：ca-certificates（TLS 根证书）+ curl（探针）
#
# 为何只需一个二进制即可运行：
#   - Go 的 CGO_ENABLED=0 编译产出完全静态链接的 ELF 二进制，无 glibc/动态库依赖
#   - 所有配置通过环境变量注入（config.Load()），无需挂载配置文件
#   - 示例数据（samples）硬编码在 Go 源码中，编译时已进入二进制
#   - 无 embed 嵌入的外部资源文件
#   - 前端静态文件（Console UI）和 TLS 证书为可选项，按需通过 volume 挂载
# ==============================================================================
FROM alpine:3.20.3

WORKDIR /app

# 安装运行时依赖 + 创建非 root 用户（单条 RUN 合并为同一镜像层，减小体积）
# Install runtime deps + create non-root user (single RUN = single image layer)
#
# ① 替换 Alpine 包管理器源为阿里云镜像，加速国内构建环境的依赖下载
#    Replace Alpine package source with Aliyun mirror for faster downloads in China
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    # ② 安装运行时最小依赖：
    #    - ca-certificates：TLS/HTTPS 出站连接所需的根证书包，
    #      使 Go 二进制能验证上游 gRPC/REST 服务的 TLS 证书
    #    - curl：轻量 HTTP 客户端，专用于 HEALTHCHECK 健康探针，
    #      定期探测 /health 端点以判断容器是否正常运行
    #    --no-cache：不缓存包索引，减少镜像层体积（约节省 2MB）
    && apk add --no-cache ca-certificates curl \
    # ③ 创建系统用户组 privacy（-S 表示系统组，无密码、无过期策略）
    #    Create system group 'privacy' (-S = system group, no password/expiry)
    && addgroup -S privacy \
    # ④ 创建系统用户 privacy 并加入 privacy 组：
    #    -S         系统用户（UID < 1000，不创建邮件 spool 等用户态文件）
    #    -G privacy 归属 privacy 组
    #    -h /app    家目录设为 /app（服务的工作目录）
    #    -s /sbin/nologin  禁止登录 shell，防止交互式登录攻击
    #    Create system user 'privacy':
    #      -S          system user (UID < 1000)
    #      -G privacy  member of privacy group
    #      -h /app     home dir = /app (service working directory)
    #      -s nologin  no interactive shell (security hardening)
    && adduser -S -G privacy -h /app -s /sbin/nologin privacy

# 复制编译产物（多阶段构建的核心指令）
# Copy build artifact (the key instruction of multi-stage build)
#
# --from=builder  指定从 Stage 1（别名为 builder 的 golang 镜像）中复制文件，
#                 而非从宿主机本地文件系统复制。
#                 这是多阶段构建的关键：仅从构建阶段取走需要的产物，
#                 Stage 1 中的源码、go mod 缓存、编译中间文件等全部留在 builder 中，
#                 不会进入最终镜像。
#
# 源路径 /app/server  Stage 1 中 go build 产出的静态链接 ELF 二进制
# 目标路径 /app/server  当前 Stage 2 镜像中的 /app/server
#
# 该二进制使用 CGO_ENABLED=0 编译，特性：
#   - 完全静态链接，无 glibc / 动态库依赖，可在 alpine 裸镜像直接运行
#   - 去掉了 DWARF 调试信息和符号表（-ldflags="-w -s"），体积最小化
#   - 包含所有业务逻辑：gRPC 客户端、HTTP 路由、mapper、示例数据等
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
ENV PRIVACY_AGENT_GRPC_HOST=PrivShield
ENV PRIVACY_AGENT_GRPC_PORT=50051

CMD ["/app/server"]
