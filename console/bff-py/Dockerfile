# =============================================================================
# Console Python Backend Dockerfile（单阶段构建）
# =============================================================================
#
# 本 Dockerfile 采用单阶段构建，与 Go 后端的多阶段构建不同。
# This Dockerfile uses a single-stage build, unlike the Go backend's multi-stage.
#
# 为什么 Python 后端不需要多阶段构建？
# Why doesn't the Python backend need multi-stage build?
#
#   Go 后端（多阶段）：
#     Stage 1 (Builder)：需要完整的 Go 编译器工具链（~300MB）将源码编译为静态二进制
#     Stage 2 (Runtime)：只需要编译好的二进制（~15MB）+ 最少系统库
#     → 编译器在构建完成后丢弃，最终镜像极小
#
#   Python 后端（单阶段）：
#     Python 是解释型语言，没有独立的编译步骤，源码即运行时产物。
#     运行时必须同时具备：
#       1. Python 解释器（运行时环境，无法丢弃）
#       2. 所有 pip 安装的依赖包（运行时需要，无法剥离）
#       3. 应用源代码（直接由解释器执行）
#     → 无法将「构建」和「运行」分离，因此多阶段构建无意义
#
# 镜像体积对比：
#   Go 最终镜像：~15MB（仅静态二进制 + alpine 基础）
#   Python 最终镜像：~200MB（Python 解释器 + pip 包 + 源码）
#   这是语言特性决定的合理差异，非设计缺陷。
#
# 锁定 Python 版本（3.13，与主 agent 镜像统一），确保构建可追溯性
# Pinned Python version (3.13, aligned with main agent image) for build reproducibility
# =============================================================================

FROM python:3.13-slim-bookworm

WORKDIR /app

# 安装系统依赖 + 创建非 root 用户（单条 RUN 合并为同一镜像层，减小体积）
# Install system deps + create non-root user (single RUN = single image layer)
#
# ① 替换 Debian APT 源为阿里云镜像，加速国内构建环境的依赖下载
#    Replace Debian APT source with Aliyun mirror for faster downloads in China
#    注：bookworm 使用新格式 debian.sources（DEB822），兼容处理两种格式
#    Note: bookworm uses new DEB822 format (debian.sources), fallback to legacy sources.list
#
# ② 安装运行时最小依赖：
#    - curl：用于 HEALTHCHECK 健康探针，定期探测 /health 端点
#    - ca-certificates：TLS/HTTPS 出站连接所需的根证书包
#    --no-install-recommends：仅安装必要依赖，跳过推荐包，减小镜像体积
#    rm -rf /var/lib/apt/lists/*：清除 APT 缓存索引，进一步减小镜像层体积
#
# ③ 创建系统用户组 privacy（-r 表示系统组，GID < 1000）
#    Create system group 'privacy' (-r = system group, GID < 1000)
#
# ④ 创建系统用户 privacy：
#    -r            系统用户（UID < 1000）
#    -g privacy    归属 privacy 组
#    -d /app       家目录设为 /app（服务的工作目录）
#    -s /sbin/nologin  禁止登录 shell，防止交互式登录攻击（安全加固）
#    Create system user 'privacy':
#      -r           system user (UID < 1000)
#      -g privacy   member of privacy group
#      -d /app      home dir = /app (service working directory)
#      -s nologin   no interactive shell (security hardening)
RUN (sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources 2>/dev/null || sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list 2>/dev/null || true) \
    && apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r privacy \
    && useradd -r -g privacy -d /app -s /sbin/nologin privacy

# 先复制依赖文件，利用 Docker 层缓存
# Copy dependency files first for Docker layer caching
# 只要 requirements.txt 未变，后续的 pip install 层会命中缓存，避免重复下载
# As long as requirements.txt is unchanged, the pip install layer hits cache
COPY requirements.txt .

# 安装 Python 依赖
# Install Python dependencies
# --no-cache-dir：不缓存下载的包，减小镜像层体积
# -i / --extra-index-url：使用国内镜像源加速 pip 包下载
RUN pip install --no-cache-dir -r requirements.txt -i https://mirrors.aliyun.com/pypi/simple/ --extra-index-url https://pypi.tuna.tsinghua.edu.cn/simple

# 复制应用源代码到镜像
# Copy application source code into the image
# Python 是解释型语言，源码直接由解释器执行，无需编译步骤
# Python is interpreted — source code is executed directly by the interpreter, no build step needed
COPY . .

# 设置 /app 目录所有权为 privacy 用户和组
# Set /app directory ownership to privacy user and group
# 确保非 root 用户有权限读写工作目录中的文件
# Ensure non-root user has permission to read/write files in the working directory
RUN chown -R privacy:privacy /app

# 声明容器监听端口 8080（仅作文档说明，实际端口映射由 docker run -p 决定）
# Document the container's listening port (actual port mapping is set by docker run -p)
EXPOSE 8080

# 容器健康检查：每 30 秒探测 /health 端点
# Container health check: probe /health endpoint every 30 seconds
# --interval=30s    探测间隔 30 秒
# --timeout=5s      单次探测超时 5 秒
# --start-period=5s 容器启动后 5 秒内不探测（给应用启动时间）
# --retries=3       连续 3 次失败标记为 unhealthy
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# 切换到非 root 用户 privacy 运行服务（最小权限原则）
# Switch to non-root user 'privacy' to run the service (principle of least privilege)
USER privacy

# 运行时环境变量配置
# Runtime environment variables
#
# PYTHONUNBUFFERED=1  禁用 Python 输出缓冲，确保日志实时输出到 docker logs
#                     Disable Python output buffering for real-time docker logs
ENV PYTHONUNBUFFERED=1
# PRIVACY_CONSOLE_HOST/PORT  本代理的 HTTP 监听地址
#                            This proxy's HTTP listen address
ENV PRIVACY_CONSOLE_HOST=0.0.0.0
ENV PRIVACY_CONSOLE_PORT=8080
# PRIVACY_AGENT_URL          上游 PrivShield Python REST agent 的地址
#                            Upstream PrivShield Python REST agent URL
ENV PRIVACY_AGENT_URL=http://PrivShield:8079

# 启动命令：使用 uvicorn ASGI 服务器运行 FastAPI 应用
# Startup command: run FastAPI app with uvicorn ASGI server
# --host 0.0.0.0  监听所有网络接口（容器内必须，实际访问控制由 docker network 管理）
# --port 8080     监听端口，与 EXPOSE 和 PRIVACY_CONSOLE_PORT 保持一致
# 生产加固 / Production hardening（与主 Agent uvicorn 配置对齐）：
#   --limit-concurrency       最大并发连接数，防止过载 OOM
#   --limit-max-requests      worker 最大处理请求数，防止内存泄漏
#   --timeout-keep-alive      空闲连接超时，减少资源占用
#   --timeout-graceful-shutdown  SIGTERM 后优雅排空在途请求
CMD ["python", "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8080", "--limit-concurrency", "10000", "--limit-max-requests", "100000", "--timeout-keep-alive", "30", "--timeout-graceful-shutdown", "10"]
