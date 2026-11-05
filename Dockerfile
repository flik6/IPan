# syntax=docker/dockerfile:1.7

# ---------- 前端构建 ----------
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---------- 后端构建 ----------
FROM golang:1.26-alpine AS build
WORKDIR /src
# 纯 Go 实现的 SQLite 驱动不需要 cgo，可以静态编译
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
ARG VERSION=dev
RUN go build -trimpath -ldflags="-s -w -X ipan/internal/httpapi.Version=${VERSION}" \
    -o /out/ipan ./cmd/ipan

# ---------- 运行镜像 ----------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S ipan && adduser -S -G ipan -h /app ipan
WORKDIR /app

COPY --from=build /out/ipan /app/ipan
COPY --from=web /src/web/dist /app/web/dist

# 数据目录（SQLite）需要持久化
RUN mkdir -p /app/data && chown -R ipan:ipan /app
VOLUME ["/app/data"]

USER ipan
ENV IPAN_LISTEN=:8080 \
    IPAN_DATA_DIR=/app/data \
    TZ=Asia/Shanghai

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:8080/api/setup/status" >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/ipan"]
CMD ["-static", "/app/web/dist"]
