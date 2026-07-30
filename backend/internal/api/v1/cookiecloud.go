package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// CookieCloudHandler 处理 CookieCloud 同步配置 / 手动触发同步。
type CookieCloudHandler struct {
	ch *chain.CookieCloudChain
}

// NewCookieCloudHandler 构造。
func NewCookieCloudHandler(ch *chain.CookieCloudChain) *CookieCloudHandler {
	return &CookieCloudHandler{ch: ch}
}

// cookieCloudConfigResp 对外脱敏配置视图（不回传密文密码，仅报告是否已配置）。
type cookieCloudConfigResp struct {
	Enabled            bool   `json:"enabled"`
	Server             string `json:"server"`
	UUID               string `json:"uuid"`
	PasswordConfigured bool   `json:"password_configured"`
	IntervalMin        int    `json:"interval_min"`
	UpdateMode         string `json:"update_mode"`
	Direction          string `json:"direction"`
	LastSyncUnix       int64  `json:"last_sync_unix"`
	LastError          string `json:"last_error"`
}

// GetConfig 返回脱敏配置 + 上次同步状态。
func (h *CookieCloudHandler) GetConfig(c *gin.Context) {
	s := h.ch.GetSettings()
	c.JSON(http.StatusOK, cookieCloudConfigResp{
		Enabled:            s.Enabled,
		Server:             s.Server,
		UUID:               s.UUID,
		PasswordConfigured: s.PasswordEnc != "",
		IntervalMin:        s.IntervalMin,
		UpdateMode:         s.UpdateMode,
		Direction:          s.Direction,
		LastSyncUnix:       s.LastSyncUnix,
		LastError:          s.LastError,
	})
}

// cookieCloudConfigReq 管理员提交的配置（password 空串=保留旧密码）。
type cookieCloudConfigReq struct {
	Enabled     bool   `json:"enabled"`
	Server      string `json:"server"`
	UUID        string `json:"uuid"`
	Password    string `json:"password"`
	IntervalMin int    `json:"interval_min"`
	UpdateMode  string `json:"update_mode"`
	Direction   string `json:"direction"`
}

// SetConfig 保存配置（password 空串保留）。
func (h *CookieCloudHandler) SetConfig(c *gin.Context) {
	var req cookieCloudConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := h.ch.SaveSettings(chain.CookieCloudSettingsInput{
		Enabled:     req.Enabled,
		Server:      req.Server,
		UUID:        req.UUID,
		Password:    req.Password,
		IntervalMin: req.IntervalMin,
		UpdateMode:  req.UpdateMode,
		Direction:   req.Direction,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Sync 手动强制同步一次（忽略间隔），返回结果汇总。
func (h *CookieCloudHandler) Sync(c *gin.Context) {
	report, err := h.ch.Sync(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "report": report})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "report": report})
}

// Push 仅推送本地站点 cookie 到 CookieCloud（管理员）。
func (h *CookieCloudHandler) Push(c *gin.Context) {
	report, err := h.ch.Push(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "report": report})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "report": report})
}
