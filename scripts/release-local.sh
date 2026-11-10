#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# 本地一键发版：前端 build+embed → docker build/push → agent 交叉编译 → Android+Windows 客户端打包。
# 用法：bash scripts/release-local.sh [版本号]   （不传则读 backend/internal/version/version.go）
#
# 前置（一次性）：
#   - docker login ghcr.io -u gntv456 --password-stdin   # PAT 需 write:packages 权限
#   - 已装：node/npm、go、docker、flutter、（Windows 客户端还需 Visual Studio + Desktop C++）
#   - iOS/macOS 客户端需 Mac，本脚本不构建（在 Mac 上 flutter build ios/macos --release）。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  VERSION=$(grep 'var Version ' backend/internal/version/version.go | sed 's/.*"\(.*\)".*/\1/')
fi
if [ -z "$VERSION" ]; then echo "无法确定版本号"; exit 1; fi
echo "▶ Release $VERSION"
OUT="dist/release-$VERSION"
mkdir -p "$OUT"
IMAGE="ghcr.io/gntv456/pt-patronus"
LDFLAGS="-s -w -X github.com/hxpt/ptpatronus/internal/version.Version=$VERSION"

# 0. 后端测试门槛：发版前必须通过 vet + 全量 -short 测试，防止带病发版
#    （CI 跑 -race 更严；这里本地去掉 race 换速度，作为发版兜底）。
echo "=== [0] backend test gate (go vet + go test -short) ==="
(cd backend && go vet ./... && go test ./... -short -timeout 20m)

# 1. 前端 build（npm install 触发 postinstall 打 element-plus 补丁）+ embed 进后端 web/dist
echo "=== [1/6] frontend build + embed ==="
(cd frontend && npm install --no-audit --no-fund && npm run build)
rm -rf backend/internal/web/dist
cp -r frontend/dist backend/internal/web/dist

# 2. docker build + push ghcr（本地 docker，不占 GitHub Actions 额度）
echo "=== [2/7] docker build ==="
if ! docker info >/dev/null 2>&1; then
  echo "  ⚠ docker daemon 不可用（Docker Desktop 未启动/未装），跳过 docker build/push。"
  echo "    Docker Desktop 启动 + docker login ghcr 后，手动补："
  echo "    docker build --build-arg VERSION=$VERSION -t $IMAGE:$VERSION -t $IMAGE:latest . && docker push $IMAGE:$VERSION && docker push $IMAGE:latest"
else
  docker build --build-arg VERSION="$VERSION" -t "$IMAGE:$VERSION" -t "$IMAGE:latest" .
  read -rp "Push $IMAGE:$VERSION & :latest to ghcr? [y/N] " PUSH
  if [[ "$PUSH" =~ ^[Yy]$ ]]; then
    docker push "$IMAGE:$VERSION"; docker push "$IMAGE:latest"
    echo "  pushed. docker pull $IMAGE:$VERSION"
  else
    echo "  skipped push (镜像已本地构建: $IMAGE:$VERSION)"
  fi
fi

# 3. 监控 Agent —— 纯 Go 交叉编译 linux amd64/arm64
echo "=== [3/6] agent linux amd64/arm64 ==="
(cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$LDFLAGS" -o "../$OUT/ptpatronus-agent-linux-amd64" ./cmd/ptpatronus-agent)
(cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$LDFLAGS" -o "../$OUT/ptpatronus-agent-linux-arm64" ./cmd/ptpatronus-agent)

# 4. Android APK（split-per-abi：arm64-v8a 95% 手机 + armeabi-v7a 老设备；x86_64 模拟器丢弃）
echo "=== [4/6] android apk ==="
(cd mobile && flutter build apk --release --split-per-abi)
cp mobile/build/app/outputs/flutter-apk/*-arm64-v8a-release.apk  "$OUT/ptpatronus-$VERSION-android-arm64.apk"
cp mobile/build/app/outputs/flutter-apk/*-armeabi-v7a-release.apk "$OUT/ptpatronus-$VERSION-android-armv7a.apk"

# 5. Windows 桌面（需 Visual Studio + Desktop C++ 工作负载）
echo "=== [5/6] windows zip ==="
(cd mobile && flutter build windows --release)
powershell.exe -NoProfile -Command "Compress-Archive -Path 'mobile\\build\\windows\\x64\\runner\\Release\\*' -DestinationPath '$OUT\\ptpatronus-$VERSION-windows-x64.zip' -Force"

# 6. 校验和
echo "=== [6/7] checksums ==="
(cd "$OUT" && sha256sum * > SHA256SUMS.txt)

# 7. OTA manifest 同步（latestVersion + docker image + sha256；url 需手动填网盘直链再上传）
echo "=== [7/7] sync OTA manifest ==="
node scripts/update-manifest.mjs "$VERSION" "$OUT"

echo
echo "✓ 完成。产物在 $OUT/ :"
ls -la "$OUT"
echo
echo "后续：上传 $OUT/* 到网盘 / GitHub Release（手动或 upload-release-to-server workflow）。"
echo "iOS/macOS 在 Mac 上：cd mobile && flutter build ios --release --no-codesign / flutter build macos --release"
