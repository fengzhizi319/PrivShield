# =============================================================================
# PrivShield App-LZ Web — Multi-stage Dockerfile
# =============================================================================
FROM node:20.18.0-alpine3.20 AS builder

WORKDIR /app

COPY . .

RUN if [ ! -d "dist" ] || [ ! -f "dist/index.html" ]; then \
        npm config set registry https://registry.npmmirror.com \
        && npm config set fetch-retries 5 \
        && npm install --no-audit \
        && npx vite build; \
    fi

FROM nginx:1.26.2-alpine

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories 2>/dev/null || true \
    && addgroup -S privacy \
    && adduser -S -G privacy -h /var/cache/nginx -s /sbin/nologin privacy

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

RUN chown -R privacy:privacy /usr/share/nginx/html \
    && chown -R privacy:privacy /var/cache/nginx \
    && chown -R privacy:privacy /var/log/nginx \
    && touch /var/run/nginx.pid \
    && chown -R privacy:privacy /var/run/nginx.pid

EXPOSE 5174

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:5174/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
