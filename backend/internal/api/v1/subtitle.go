package v1

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// SubtitleHandler 处理字幕搜索 / 下载 / 源配置。
type SubtitleHandler struct {
	svc *chain.SubtitleChain
}

func NewSubtitleHandler(svc *chain.SubtitleChain) *SubtitleHandler {
	return &SubtitleHandler{svc: svc}
}

// Search 跨源搜索字幕。?q=&language=（language: 空=任意 chs/cht/eng）。
func (h *SubtitleHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q 必填"})
		return
	}
	res, err := h.svc.Search(c.Request.Context(), q, c.Query("language"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Source == "" || req.SourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source/source_id 必填"})
		return
	}
	sub := subtitle.Subtitle{
		Source: req.Source, SourceID: req.SourceID, Title: req.Title,
		LangCode: req.LangCode, FileName: req.FileName, Ext: req.Ext, Extra: req.Extra,
	}
	data, name, err := h.svc.Download(c.Request.Context(), sub)
	if err != nil {
		if errors.Is(err, oper.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
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
	AssrtToken             string `json:"assrt_token"`
	AssrtTokenConfigured   bool   `json:"assrt_token_configured"`
	OpenSubtitlesKey       string `json:"opensubtitles_key"`
	OpenSubtitlesConfigured bool  `json:"opensubtitles_key_configured"`
	SubHDCookie            string `json:"subhd_cookie"`
	SubHDCookieConfigured  bool   `json:"subhd_cookie_configured"`
	ZimukuCookie           string `json:"zimuku_cookie"`
	ZimukuCookieConfigured bool   `json:"zimuku_cookie_configured"`
	TargetLang             string `json:"target_lang"`
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tl := strings.TrimSpace(req.TargetLang)
	if tl != "" && tl != "chs" && tl != "cht" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_lang 仅支持 chs/cht"})
		return
	}
	if err := h.svc.SetConfig(chain.SubtitleSettings{
		AssrtToken:       strings.TrimSpace(req.AssrtToken),
		OpenSubtitlesKey: strings.TrimSpace(req.OpenSubtitlesKey),
		SubHDCookie:      strings.TrimSpace(req.SubHDCookie),
		ZimukuCookie:     strings.TrimSpace(req.ZimukuCookie),
		TargetLang:       tl,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
