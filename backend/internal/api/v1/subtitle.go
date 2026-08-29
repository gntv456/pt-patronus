package v1

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/subtitle"
	"github.com/hxpt/ptpatronus/internal/subtitle/automation"
)

// SubtitleHandler 处理字幕搜索 / 下载 / 源配置 + 字幕自动化配置。
type SubtitleHandler struct {
	svc  *chain.SubtitleChain
	auto *subauto.Service // 字幕自动化服务（可能为 nil：旧部署未启用时）
}

func NewSubtitleHandler(svc *chain.SubtitleChain, auto *subauto.Service) *SubtitleHandler {
	return &SubtitleHandler{svc: svc, auto: auto}
}

// Search 跨源搜索字幕。?q=&language=（language: 空=任意 chs/cht/eng）。
func (h *SubtitleHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	res, err := h.svc.Search(c.Request.Context(), q, c.Query("language"))
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if res == nil {
		res = []subtitle.Subtitle{}
	}
	c.JSON(http.StatusOK, gin.H{"results": res})
}

// downloadReq 下载请求体（前端把搜索结果原样回传）。
type downloadReq struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	LangCode string `json:"lang_code"`
	FileName string `json:"file_name"`
	Ext      string `json:"ext"`
	Extra    string `json:"extra"`
}

// Download 下载字幕：解 zip + 转 UTF-8 + 可选繁简，以附件返回。
func (h *SubtitleHandler) Download(c *gin.Context) {
	var req downloadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if req.Source == "" || req.SourceID == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	sub := subtitle.Subtitle{
		Source: req.Source, SourceID: req.SourceID, Title: req.Title,
		LangCode: req.LangCode, FileName: req.FileName, Ext: req.Ext, Extra: req.Extra,
	}
	data, name, err := h.svc.Download(c.Request.Context(), sub)
	if err != nil {
		if errors.Is(err, oper.ErrInvalidInput) {
			errcode.Respond(c, errcode.InvalidInput.WithErr(err))
			return
		}
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	contentType := "application/octet-stream"
	if e := strings.ToLower(name); strings.HasSuffix(e, ".srt") || strings.HasSuffix(e, ".vtt") || strings.HasSuffix(e, ".ass") || strings.HasSuffix(e, ".ssa") {
		contentType = "text/plain; charset=utf-8"
	}
	c.Header("Content-Disposition", `attachment; filename="`+sanitizeFilename(name)+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, contentType, data)
}

// subtitleConfigResp 配置回显（secret 脱敏 + _configured）。
type subtitleConfigResp struct {
	AssrtToken              string `json:"assrt_token"`
	AssrtTokenConfigured    bool   `json:"assrt_token_configured"`
	OpenSubtitlesKey        string `json:"opensubtitles_key"`
	OpenSubtitlesConfigured bool   `json:"opensubtitles_key_configured"`
	SubHDCookie             string `json:"subhd_cookie"`
	SubHDCookieConfigured   bool   `json:"subhd_cookie_configured"`
	ZimukuCookie            string `json:"zimuku_cookie"`
	ZimukuCookieConfigured  bool   `json:"zimuku_cookie_configured"`
	TargetLang              string `json:"target_lang"`
}

func (h *SubtitleHandler) GetConfig(c *gin.Context) {
	s := h.svc.Config()
	c.JSON(http.StatusOK, subtitleConfigResp{
		AssrtToken:              maskSecret(s.AssrtToken),
		AssrtTokenConfigured:    s.AssrtToken != "",
		OpenSubtitlesKey:        maskSecret(s.OpenSubtitlesKey),
		OpenSubtitlesConfigured: s.OpenSubtitlesKey != "",
		SubHDCookie:             maskSecret(s.SubHDCookie),
		SubHDCookieConfigured:   s.SubHDCookie != "",
		ZimukuCookie:            maskSecret(s.ZimukuCookie),
		ZimukuCookieConfigured:  s.ZimukuCookie != "",
		TargetLang:              s.TargetLang,
	})
}

// subtitleConfigReq 配置更新（各 secret 空串=不修改）。
type subtitleConfigReq struct {
	AssrtToken       string `json:"assrt_token" binding:"max=256"`
	OpenSubtitlesKey string `json:"opensubtitles_key" binding:"max=256"`
	SubHDCookie      string `json:"subhd_cookie" binding:"max=2048"`
	ZimukuCookie     string `json:"zimuku_cookie" binding:"max=2048"`
	TargetLang       string `json:"target_lang" binding:"max=8"`
}

func (h *SubtitleHandler) SetConfig(c *gin.Context) {
	var req subtitleConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	tl := strings.TrimSpace(req.TargetLang)
	if tl != "" && tl != "chs" && tl != "cht" {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	if err := h.svc.SetConfig(chain.SubtitleSettings{
		AssrtToken:       strings.TrimSpace(req.AssrtToken),
		OpenSubtitlesKey: strings.TrimSpace(req.OpenSubtitlesKey),
		SubHDCookie:      strings.TrimSpace(req.SubHDCookie),
		ZimukuCookie:     strings.TrimSpace(req.ZimukuCookie),
		TargetLang:       tl,
	}); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// sanitizeFilename 去掉文件名中不安全的路径/控制字符（防 Content-Disposition 注入）。
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	if name == "" {
		name = "subtitle.srt"
	}
	return name
}

// GetAutomationConfig 读字幕自动化配置（开关 / 语言 / provider 优先级 / 仅缺字幕）。
func (h *SubtitleHandler) GetAutomationConfig(c *gin.Context) {
	if h.auto == nil {
		c.JSON(http.StatusOK, subauto.DefaultConfig())
		return
	}
	c.JSON(http.StatusOK, h.auto.Config())
}

// SetAutomationConfig 写字幕自动化配置（即时生效：下次 transfer_done 现取）。
func (h *SubtitleHandler) SetAutomationConfig(c *gin.Context) {
	if h.auto == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	var cfg subauto.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.auto.SetConfig(cfg); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
