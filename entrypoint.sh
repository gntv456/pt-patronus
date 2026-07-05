#!/bin/sh
set -e
# 容器以 root 启动本脚本。挂载的 /app/data 卷属主可能是宿主 root（compose 自动建的 ./data），
# 镜像内 chown 的 app 属主被挂载覆盖。这里启动时 chown 给 app，让主程序以 app 用户跑也能写
# （密钥自动生成 / SQLite / 运行配置）。
mkdir -p /app/data
chown -R app:app /app/data 2>/dev/null || true
# chown 完成后切 app 用户跑主程序（最小权限原则，容器内非 root）。
exec su-exec app /app/ptpatronus "$@"
