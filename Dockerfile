# syntax=docker/dockerfile:1
# PTPatronus 单镜像：前端(Vue3)构建 → go:embed 内嵌 → 后端(Go) → 运行时(alpine)
# 产物：一个镜像同时提供 API（:8088）与 Web UI。

# ---- 前端构建 ----
FROM node:20-alpine AS frontend
WORKDIR /fe
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---- 后端构建（内嵌前端 dist）----
FROM golang:1.26-alpine AS backend
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOPROXY=https://goproxy.cn,direct
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
# 用真实前端构建产物覆盖占位 dist，让 go:embed 打进二进制
COPY --from=frontend /fe/dist ./internal/web/dist
# VERSION 由 docker.yml 在 tag 构建时通过 --build-args 注入（形如 0.70.01），
# 未传则用 dev，编译期写进 internal/version.Version。
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -X github.com/hxpt/ptpatronus/internal/version.Version=${VERSION}" -o /out/ptpatronus ./cmd/ptpatronus

# ---- 运行时 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=backend /out/ptpatronus /app/ptpatronus
COPY backend/definitions /app/definitions
RUN mkdir -p /app/data && chown -R app:app /app
USER app
ENV APP_ENV=release HTTP_ADDR=:8088 DATA_DIR=/app/data DEFINITIONS_DIR=/app/definitions
EXPOSE 8088
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8088/api/v1/system/health || exit 1
ENTRYPOINT ["/app/ptpatronus"]
