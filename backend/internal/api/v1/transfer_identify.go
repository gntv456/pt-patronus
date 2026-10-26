package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// ---- 识别词表与名称识别测试（对标 MSaber「名称识别测试」） ----

// GetIdentifyWords 识别词表（GET /transfer/identify-words）。
func (h *TransferHandler) GetIdentifyWords(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"words": transfer.LoadIdentifyWords(h.ch.KV())})
}

// SetIdentifyWords 保存识别词表（PUT /transfer/identify-words）。
func (h *TransferHandler) SetIdentifyWords(c *gin.Context) {
	var in struct {
		Words []transfer.IdentifyWord `json:"words" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if err := transfer.SaveIdentifyWords(h.ch.KV(), in.Words); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": len(in.Words)})
}

// TestIdentifyWords 识别词效果测试（POST：{name} → 规整后名称 + 识别元数据）。
func (h *TransferHandler) TestIdentifyWords(c *gin.Context) {
	var in struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	ws := transfer.LoadIdentifyWords(h.ch.KV())
	norm := transfer.ApplyIdentifyWords(ws, in.Name)
	meta := transfer.ParseMeta(norm)
	c.JSON(http.StatusOK, gin.H{"normalized": norm, "title": meta.Title, "season": meta.Season, "episode": meta.Episode})
}

// TestRecognize 名称识别测试（POST：{name, year?, media?} → 本地识别元数据 + 媒体源匹配结果）。
// 对标 MSaber「名称识别测试」：先本地 ParseMeta（含识别词规整），再可选调媒体源确认能否匹配，
// 用于验证文件名可识别性与识别词效果。media=true 走媒体源查询（默认仅本地）。
func (h *TransferHandler) TestRecognize(c *gin.Context) {
	var in struct {
		Name  string `json:"name" binding:"required"`
		Year  int    `json:"year"`
		Media bool   `json:"media"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	ws := transfer.LoadIdentifyWords(h.ch.KV())
	norm := transfer.ApplyIdentifyWords(ws, in.Name)
	meta := transfer.ParseMeta(norm)
	out := gin.H{
		"normalized": norm,
		"title":      meta.Title,
		"year":       meta.Year,
		"season":     meta.Season,
		"episode":    meta.Episode,
		"resolution": meta.Resolution,
		"group":      meta.ReleaseGroup,
		"is_tv":      meta.IsTV,
	}
	if in.Media && meta.Title != "" {
		m, err := h.ch.SearchMedia(c.Request.Context(), meta.Title, in.Year, "")
		if err == nil && m != nil {
			out["matched"] = gin.H{"title": m.Title, "year": m.Year, "type": m.Type}
		} else {
			out["matched"] = nil
			// 「媒体源挂了」和「库里真没这部片」必须分得开——否则用户会照着「未匹配」
			// 去补识别词，而真正的问题是源查询失败（链层已给出中文原因）。
			if err != nil {
				out["media_error"] = errtext.Text(err)
			}
		}
	}
	c.JSON(http.StatusOK, out)
}
