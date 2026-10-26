package v1

// 媒体源配置端点（GET/PUT：secret 脱敏回显、空串=不修改）。从 media.go 迁出。

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
)

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// mediaConfigResp 媒体源配置（GET）。secret 字段脱敏 + configured 标记；域名非敏感显明文。
type mediaConfigResp struct {
	TMDBAPIKey             string `json:"tmdb_api_key"`
	TMDBAPIKeyConfigured   bool   `json:"tmdb_api_key_configured"`
	TMDBAPIDomain          string `json:"tmdb_api_domain"`
	DoubanSecret           string `json:"douban_secret"`
	DoubanSecretConfigured bool   `json:"douban_secret_configured"`
	DoubanAppKey           string `json:"douban_app_key"`
	DoubanAppKeyConfigured bool   `json:"douban_app_key_configured"`
	BangumiToken           string `json:"bangumi_token"`
	BangumiTokenConfigured bool   `json:"bangumi_token_configured"`
	OMDbAPIKey             string `json:"omdb_api_key"`
	OMDbAPIKeyConfigured   bool   `json:"omdb_api_key_configured"`
}

// GetConfig 返回当前媒体源配置（secret 脱敏）。供「媒体源设置」页回显。
func (h *MediaHandler) GetConfig(c *gin.Context) {
	s := h.mediaCh.MediaConfig()
	c.JSON(http.StatusOK, mediaConfigResp{
		TMDBAPIKey:             maskSecret(s.TMDBAPIKey),
		TMDBAPIKeyConfigured:   s.TMDBAPIKey != "",
		TMDBAPIDomain:          s.TMDBAPIDomain,
		DoubanSecret:           maskSecret(s.DoubanSecret),
		DoubanSecretConfigured: s.DoubanSecret != "",
		DoubanAppKey:           maskSecret(s.DoubanAppKey),
		DoubanAppKeyConfigured: s.DoubanAppKey != "",
		BangumiToken:           maskSecret(s.BangumiToken),
		BangumiTokenConfigured: s.BangumiToken != "",
		OMDbAPIKey:             maskSecret(s.OMDbAPIKey),
		OMDbAPIKeyConfigured:   s.OMDbAPIKey != "",
	})
}

// mediaConfigReq 媒体源配置（PUT）。各字段空串=不修改（便于只改部分项而不必重输全部 secret）。
// max= 防止超长串入下游 HTTP/存储（secret/token/host 均有合理上限）。
type mediaConfigReq struct {
	TMDBAPIKey    string `json:"tmdb_api_key" binding:"max=128"`
	TMDBAPIDomain string `json:"tmdb_api_domain" binding:"max=255"`
	DoubanSecret  string `json:"douban_secret" binding:"max=128"`
	DoubanAppKey  string `json:"douban_app_key" binding:"max=64"`
	BangumiToken  string `json:"bangumi_token" binding:"max=128"`
	OMDbAPIKey    string `json:"omdb_api_key" binding:"max=128"`
}

// SetConfig 保存媒体源配置并即时 rebuild 源（无需重启）。空字段不修改。
func (h *MediaHandler) SetConfig(c *gin.Context) {
	var req mediaConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.mediaCh.SetMediaConfig(chain.MediaSettings{
		TMDBAPIKey:    strings.TrimSpace(req.TMDBAPIKey),
		TMDBAPIDomain: strings.TrimSpace(req.TMDBAPIDomain),
		DoubanSecret:  strings.TrimSpace(req.DoubanSecret),
		DoubanAppKey:  strings.TrimSpace(req.DoubanAppKey),
		BangumiToken:  strings.TrimSpace(req.BangumiToken),
		OMDbAPIKey:    strings.TrimSpace(req.OMDbAPIKey),
	}); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

