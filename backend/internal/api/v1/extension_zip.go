// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// 浏览器扩展（extension/）打包分发：快速开始承诺「设置 → 关于 → 下载扩展 zip」，
// 但 Docker 用户拿不到仓库目录——由后端 embed 打好的 zip 直接下发。
// zip 由 scripts/pack-extension.py 从仓库 extension/ 生成（排除 test/），
// 改动扩展后需重跑该脚本（pre-push 门禁校验新鲜度）。
import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed extension_bundle.zip
var extensionBundle []byte

// ExtensionZip GET /system/extension.zip（guarded）：下发浏览器扩展包。
// Chrome「开发者模式 → 加载已解压的扩展程序」解压即用（extension/README 有全流程）。
func (h *SystemHandler) ExtensionZip(c *gin.Context) {
	c.Data(http.StatusOK, "application/zip", extensionBundle)
}
