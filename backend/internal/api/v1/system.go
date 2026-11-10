// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package v1 contains the v1 HTTP handlers grouped by resource.
package v1

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/log"
	"github.com/hxpt/ptpatronus/internal/sysinfo"
	"github.com/hxpt/ptpatronus/internal/update"
	"github.com/hxpt/ptpatronus/internal/version"
	"gorm.io/gorm"
)

// SystemHandler exposes health / system info / online update checks.
type SystemHandler struct {
	update *update.Service
	db     *gorm.DB
}

// NewSystemHandler builds a SystemHandler. upd 可为 nil（禁用更新检查）。
// db 用于 readiness 探针（/health/ready）——SQLite 场景下 DB 文件损坏/锁死时
// 进程仍存活，仅靠 liveness 会误判健康。
func NewSystemHandler(upd *update.Service, db *gorm.DB) *SystemHandler {
	return &SystemHandler{update: upd, db: db}
}

// Health is a public liveness probe.
func (h *SystemHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ptpatronus"})
}

// Readiness is a public readiness probe：liveness 之外补 DB 连通性检查
// （运维可观测性审计）。Docker healthcheck 仍挂 liveness（SQLite 同进程，
// readiness 主要供反向代理/编排摘流量用）。DB 为 nil 或 ping 失败 → 503。
func (h *SystemHandler) Readiness(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "db": "not-configured"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if sqlDB, err := h.db.DB(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "db": err.Error()})
		return
	} else if err := sqlDB.PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "db": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "ok"})
}

// Info 返回应用名称；版本号仅对已认证请求返回。
// 未认证暴露精确版本是指纹风险（对照 changelog/CVE 定位实例）；登录页不依赖版本号，
// 前端 Settings / 移动端 Account 均在登录后（带 token）调用，JWTAuth 已为其注入 uid。
func (h *SystemHandler) Info(c *gin.Context) {
	resp := gin.H{"name": "PTPatronus"}
	if _, authed := c.Get("uid"); authed {
		resp["version"] = version.Version
	}
	c.JSON(http.StatusOK, resp)
}

// Logs 返回最近运行日志（环形缓冲；?level=info|warn|error&n=500，需登录）。
// nil 切片归一为空数组，防前端 JSON null。
func (h *SystemHandler) Logs(c *gin.Context) {
	n := 500
	if v := c.Query("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 5000 {
			n = parsed
		}
	}
	entries := log.Ring.Recent(n, log.ParseLevel(c.Query("level")))
	if entries == nil {
		entries = []log.LogEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

// ClearLogs 清空内存日志环形缓冲（不影响 stdout/落盘日志；需登录）。
func (h *SystemHandler) ClearLogs(c *gin.Context) {
	log.Ring.Clear()
	c.JSON(http.StatusOK, gin.H{"cleared": true})
}

// Metrics 返回本机系统资源快照（CPU/内存/磁盘/负载等，即时采集 gopsutil）。需管理员。
// 仪表盘「系统资源」卡用。Metrics 的集合字段（disks/temps）带 omitempty：无数据时省略而非输出
// 空数组，前端用 `?? []` 兜底（记忆 [[go-nil-slice-json-null]]）。
func (h *SystemHandler) Metrics(c *gin.Context) {
	c.JSON(http.StatusOK, sysinfo.CollectSystemMetrics())
}

// UpdateCheck 在线更新检查。?platform 取 update 包 Platform 常量
// (docker|android|ios|macos|ohos|windows|agent-linux-*)，决定返回哪个
// 平台的产物信息；?version= 透传客户端版本（客户端自检：按客户端版本对比清单），
// 留空则取内置 version.Version（服务端自检场景）。需登录。
func (h *SystemHandler) UpdateCheck(c *gin.Context) {
	if h.update == nil {
		c.JSON(http.StatusOK, gin.H{"hasUpdate": false})
		return
	}
	platform := c.DefaultQuery("platform", update.PlatformDocker)
	clientVer := c.Query("version") // 客户端自检透传；空→Check 内部默认取服务端版本
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	res, err := h.update.Check(ctx, platform, clientVer)
	if err != nil {
		errcode.Respond(c, errcode.UpstreamFailed.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, res)
}

// SetManifest 写入自托管版本清单（覆盖官方源）。需管理员。
// 请求体即 Manifest；latestVersion 为空表示清除自托管清单、回退官方源。
func (h *SystemHandler) SetManifest(c *gin.Context) {
	if h.update == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	var m update.Manifest
	if err := c.ShouldBindJSON(&m); err != nil {
		errcode.Respond(c, errcode.InvalidRequest.WithErr(err))
		return
	}
	var overlay *update.Manifest
	if m.LatestVersion != "" {
		overlay = &m
	}
	if err := h.update.SetOverlay(overlay); err != nil {
		errcode.Respond(c, errcode.UpstreamFailed.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "cleared": overlay == nil})
}
