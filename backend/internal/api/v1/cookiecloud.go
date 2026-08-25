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
	Mode               string `json:"mode"`
	Server             string `json:"server"`
	UUID               string `json:"uuid"`
	PasswordConfigured bool   `json:"password_configured"`
	IntervalMin        int    `json:"interval_min"`
	UpdateMode         string `json:"update_mode"`
	Direction          string `json:"direction"`
	LastSyncUnix       int64  `json:"last_sync_unix"`
	LastError          string `json:"last_error"`
}

// GetConfig 返回脱敏配置 + 上次同步状态。builtin 模式附带 endpoint 路径（前端拼 origin）。
func (h *CookieCloudHandler) GetConfig(c *gin.Context) {
	s := h.ch.GetSettings()
	resp := cookieCloudConfigResp{
		Enabled:            s.Enabled,
		Mode:               s.Mode,
		Server:             s.Server,
		UUID:               s.UUID,
		PasswordConfigured: s.PasswordEnc != "",
		IntervalMin:        s.IntervalMin,
		UpdateMode:         s.UpdateMode,
		Direction:          s.Direction,
		LastSyncUnix:       s.LastSyncUnix,
		LastError:          s.LastError,
	}
	if resp.Mode == "" {
		resp.Mode = chain.CookieCloudModeExternal
	}
	c.JSON(http.StatusOK, resp)
}

// cookieCloudConfigReq 管理员提交的配置（password 空串=保留旧密码）。
type cookieCloudConfigReq struct {
	Enabled     bool   `json:"enabled"`
	Mode        string `json:"mode"`
	Server      string `json:"server"`
	UUID        string `json:"uuid"`
	Password    string `json:"password"`
	Regenerate  bool   `json:"regenerate"`
	IntervalMin int    `json:"interval_min"`
	UpdateMode  string `json:"update_mode"`
	Direction   string `json:"direction"`
}

// SetConfig 保存配置（password 空串保留；builtin 首次保存自动生成 uuid+密码）。
// builtin 模式下若本次生成/重生了密码，响应一次性回传明文（仅此一次，之后只显示已配置）。
func (h *CookieCloudHandler) SetConfig(c *gin.Context) {
	var req cookieCloudConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prev := h.ch.GetSettings()
	s, err := h.ch.SaveSettings(chain.CookieCloudSettingsInput{
		Enabled:     req.Enabled,
		Mode:        req.Mode,
		Server:      req.Server,
		UUID:        req.UUID,
		Password:    req.Password,
		Regenerate:  req.Regenerate,
		IntervalMin: req.IntervalMin,
		UpdateMode:  req.UpdateMode,
		Direction:   req.Direction,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := gin.H{"ok": true, "uuid": s.UUID}
	// 密码只在「builtin 且本次生成/重生」时回传一次：旧配置无密码→新生成，或显式 regenerate
	if chain.NormalizeMode(s.Mode) == chain.CookieCloudModeBuiltin &&
		(req.Regenerate || prev.PasswordEnc == "") && req.Password == "" {
		if plain, derr := h.ch.DecryptedPassword(s); derr == nil && plain != "" {
			resp["password"] = plain
		}
	}
	c.JSON(http.StatusOK, resp)
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
