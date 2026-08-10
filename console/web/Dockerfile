# Console Web Frontend Multi-stage Dockerfile
# 锁定 Node 和 Nginx 版本，确保构建可追溯性
# Pinned Node and Nginx versions for build reproducibility

# ==============================================================================
# Stage 1: Build
# ==============================================================================
FROM node:20.18.0-alpine3.20 AS builder

WORKDIR /app

# 先复制依赖文件，利用缓存
# Copy dependency files first for layer caching
COPY package.json pnpm-lock.yaml* package-lock.json* ./

# 安装 pnpm 并安装依赖
# Install pnpm and dependencies
RUN npm install -g pnpm && pnpm install --frozen-lockfile || npm install

# 复制源码并构建
# Copy source and build
COPY . .
RUN pnpm run build || npm run build

# ==============================================================================
# Stage 2: Runtime (Nginx static serving)
# ==============================================================================
FROM nginx:1.26.2-alpine

# 创建非 root 用户（Nginx 仍需 root 启动以绑定 80 端口，但 worker 进程降权）
# Create non-root user (Nginx still needs root to bind port 80, but workers run unprivileged)
RUN addgroup -S privacy \
    && adduser -S -G privacy -h /var/cache/nginx -s /sbin/nologin privacy

# 复制构建产物与 Nginx 配置
# Copy build artifacts and Nginx config
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 设置目录所有权
# Set directory ownership
RUN chown -R privacy:privacy /usr/share/nginx/html \
    && chown -R privacy:privacy /var/cache/nginx \
    && chown -R privacy:privacy /var/log/nginx \
    && touch /var/run/nginx.pid \
    && chown -R privacy:privacy /var/run/nginx.pid

EXPOSE 5173

# 容器健康检查
# Container health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:5173/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
