# syntax=docker/dockerfile:1
# PTPatronus 单镜像：前端(Vue3)构建 → go:embed 内嵌 → 后端(Go) → 运行时(alpine)
# 产物：一个镜像同时提供 API（:8088）与 Web UI。

# ---- 前端构建 ----
# 前端固定 amd64 build（JS 产物平台无关），避免 arm64 走 QEMU 跑 npm install 触发
# "Illegal instruction" 崩溃。dist 复制到各平台后端 stage 即可（go:embed 不挑平台）。
FROM --platform=linux/amd64 node:20-alpine AS frontend
WORKDIR /fe
COPY frontend/package.json frontend/package-lock.json* ./
# postinstall 会跑 scripts/patch-deps.mjs（给 element-plus 打 ElTable mounted 补丁），
# 必须在 npm install 之前就位（此时 frontend/ 整体尚未 COPY）。
COPY frontend/scripts/patch-deps.mjs ./scripts/patch-deps.mjs
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
RUN apk add --no-cache ca-certificates tzdata wget su-exec && \
    addgroup -S app && adduser -S app -G app
WORKDIR /app
# --chown 在 COPY 时设定属主，避免随后 `chown -R` 把 50MB 二进制重写进新层（省 ~50MB 镜像体积）。
COPY --chown=app:app --from=backend /out/ptpatronus /app/ptpatronus
COPY --chown=app:app backend/definitions /app/definitions
COPY --chown=app:app entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && mkdir -p /app/data
# 不设 USER app：entrypoint 以 root 跑（chown 挂载卷 /app/data 给 app），再 su-exec 切 app 跑主程序。
# 这样宿主卷（compose 自动建，默认属主 root）也能被 app 写（密钥自动生成/SQLite），无需用户手动 chmod。
ENV APP_ENV=release HTTP_ADDR=:8088 DATA_DIR=/app/data DEFINITIONS_DIR=/app/definitions
EXPOSE 8088
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8088/api/v1/system/health || exit 1
ENTRYPOINT ["/app/entrypoint.sh"]
