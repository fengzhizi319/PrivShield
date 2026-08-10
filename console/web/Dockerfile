# Console Web Frontend Multi-stage Dockerfile
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json pnpm-lock.yaml* package-lock.json* ./
RUN npm install -g pnpm && pnpm install --frozen-lockfile || npm install

COPY . .

RUN pnpm run build || npm run build

FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 5173

CMD ["nginx", "-g", "daemon off;"]
