// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// 更新产物托管：CI/发版人把客户端产物（apk/hap/ipa/zip…）放进 <DataDir>/downloads/
// （scp 或本上传端点），服务器自动算 sha256 并挂进自托管 overlay 清单——
// 「人工回填网盘直链」退役。机制说明见 internal/update/artifacts.go 顶部。
import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/update"
)

func downloadsDir(h *SystemHandler) string {
	if h.dataDir == "" {
		return ""
	}
	return filepath.Join(h.dataDir, "downloads")
}

// Downloads GET /system/downloads/:file（guarded）：托管产物直链下发。
// url 是清单里的相对路径（/api/v1/system/downloads/<file>），客户端拼 baseUrl 后
// 应用内 OTA 与浏览器外链都落到这里；http.ServeFile 自动处理 Range（断点续传）。
// 只放行规范产物名（ParseArtifactName 认不出的一律 404）——目录穿越与任意文件
// 读取在名字校验这一层就进不来。
func (h *SystemHandler) Downloads(c *gin.Context) {
	dir := downloadsDir(h)
	if dir == "" {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	name := c.Param("file")
	if platform, _ := update.ParseArtifactName(name); platform == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeFile(c.Writer, c.Request, path)
}

// CurrentOverlay 当前生效的自托管清单（nil = 未设置，走官方源）。
func (h *SystemHandler) CurrentOverlay() *update.Manifest {
	if h.update == nil {
		return nil
	}
	return h.update.OverlaySnapshot()
}

// ListArtifacts GET /admin/update/artifacts（管理员）：当前托管产物清单 +
// 生效中的 overlay 摘要（排障「为什么没挂上」用）。
func (h *SystemHandler) ListArtifacts(c *gin.Context) {
	dir := downloadsDir(h)
	if dir == "" || h.update == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	files, err := update.ListArtifacts(dir)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"artifacts": files, "overlay": h.CurrentOverlay()})
}

// ScanArtifacts POST /admin/update/artifacts/scan（管理员）：手动触发一次扫描挂载。
// CI 走 scp 时可以用它即时生效（不等每小时兜底扫描）。
func (h *SystemHandler) ScanArtifacts(c *gin.Context) {
	dir := downloadsDir(h)
	if dir == "" || h.update == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	if err := h.update.ApplyArtifacts(dir); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "overlay": h.CurrentOverlay()})
}

// UploadArtifact POST /admin/update/artifacts（管理员，multipart）：
// 字段 file=产物文件，filename 必须是规范名 <platform>-<version>.<ext>。
// 存盘后立即扫描挂载——管理员从 Web 界面拖一个 APK 就能让对应端 OTA 生效。
func (h *SystemHandler) UploadArtifact(c *gin.Context) {
	dir := downloadsDir(h)
	if dir == "" || h.update == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		missing := errors.New("缺 file 字段（multipart）")
		errcode.Respond(c, errcode.InvalidRequest.WithErr(missing))
		return
	}
	// 服务端只认规范名：platform/version/ext 白名单不满足直接 400（不落盘）。
	platform, version := update.ParseArtifactName(fh.Filename)
	if platform == "" {
		bad := errors.New("文件名必须是 <平台>-<版本>.<后缀>，如 android-1.03.31.apk；" +
			"后缀限 apk/hap/ipa/zip/dmg/exe/tar.gz")
		errcode.Respond(c, errcode.InvalidRequest.WithErr(bad))
		return
	}
	if fh.Size > update.ArtifactMaxBytes {
		tooBig := errors.New("产物超过单文件上限")
		errcode.Respond(c, errcode.InvalidRequest.WithErr(tooBig))
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	dst := filepath.Join(dir, fh.Filename)
	if err := c.SaveUploadedFile(fh, dst); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// 同平台旧版本文件不删——历史版本保留可回滚（清单只挂最新）。
	if err := h.update.ApplyArtifacts(dir); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"platform": platform,
		"version":  version,
		"overlay":  h.CurrentOverlay(),
	})
}
