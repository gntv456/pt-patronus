package v1

import (
	"errors"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/mediaserver"
)

// MediaServerHandler 处理媒体服务器 CRUD + 连接测试 + 库操作 + 封面生成 + webhook 接收。
type MediaServerHandler struct {
	svc *chain.MediaServerChain
}

func NewMediaServerHandler(svc *chain.MediaServerChain) *MediaServerHandler {
	return &MediaServerHandler{svc: svc}
}

// List 列出全部媒体服务器。
func (h *MediaServerHandler) List(c *gin.Context) {
	ms, err := h.svc.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if ms == nil {
		ms = []models.MediaServer{} // 兜底空数组（避免 nil slice → JSON null）
	}
	c.JSON(http.StatusOK, gin.H{"servers": ms})
}

// Create 创建媒体服务器。
func (h *MediaServerHandler) Create(c *gin.Context) {
	var in oper.MediaServerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	m, err := h.svc.Create(in)
	if err != nil {
		respondMediaErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// Update 更新媒体服务器（token 空串=保留）。
func (h *MediaServerHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.MediaServerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	m, err := h.svc.Update(id, in)
	if err != nil {
		respondMediaErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// Delete 删除媒体服务器。
func (h *MediaServerHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opResult(c, h.svc.Delete(id))
}

// Test 连接 + 鉴权探活。
func (h *MediaServerHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	info, err := h.svc.Test(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "info": info})
}

// Libraries 列媒体库。
func (h *MediaServerHandler) Libraries(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	libs, err := h.svc.Libraries(c.Request.Context(), id)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"libraries": libs})
}

// Scan 触发整库扫描（body: {library_id}；空=扫全部库）。
func (h *MediaServerHandler) Scan(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body struct {
		LibraryID string `json:"library_id"`
	}
	// 空 body（io.EOF）= 扫全部库（合法用法）；仅非法 JSON（非 EOF）才拒绝，
	// 避免坏请求因 LibraryID 零值静默落入"全库扫描"分支。
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if body.LibraryID != "" {
		opResult(c, h.svc.LibraryScan(c.Request.Context(), id, body.LibraryID))
		return
	}
	// 空 library_id：扫该服务器全部库。
	libs, err := h.svc.Libraries(c.Request.Context(), id)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	for _, lib := range libs {
		if e := h.svc.LibraryScan(c.Request.Context(), id, lib.ID); e != nil {
			opResult(c, e)
			return
		}
	}
	opResult(c, nil)
}

// Refresh 刷新库元数据（body: {library_id}）。
func (h *MediaServerHandler) Refresh(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body struct {
		LibraryID string `json:"library_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.LibraryID == "" {
		errcode.Respond(c, errcode.MediaLibraryRequired)
		return
	}
	opResult(c, h.svc.Refresh(c.Request.Context(), id, body.LibraryID))
}

// Cover 生成封面拼图（返回 image/jpeg）。query: library_id=&cols=&rows=&w=&h=。
func (h *MediaServerHandler) Cover(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	libID := c.Query("library_id")
	if libID == "" {
		errcode.Respond(c, errcode.MediaLibraryRequired)
		return
	}
	opt := mediaserver.DefaultCoverOptions()
	if v, err := strconv.Atoi(c.Query("cols")); err == nil && v > 0 {
		opt.Cols = v
	}
	if v, err := strconv.Atoi(c.Query("rows")); err == nil && v > 0 {
		opt.Rows = v
	}
	if v, err := strconv.Atoi(c.Query("w")); err == nil && v > 0 {
		opt.ThumbW = v
	}
	if v, err := strconv.Atoi(c.Query("h")); err == nil && v > 0 {
		opt.ThumbH = v
	}
	data, err := h.svc.Cover(c.Request.Context(), id, libID, opt)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "image/jpeg", data)
}

// Notify 向该服务器发管理员通知（body: {title, body}）。
func (h *MediaServerHandler) Notify(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if body.Title == "" {
		body.Title = "PTPatronus 通知"
	}
	opResult(c, h.svc.NotifyOne(c.Request.Context(), id, body.Title, body.Body))
}

// respondMediaErr 把 oper 错误映射到状态码：输入校验（类型/URL 非法）→ 400，其余 → 500。
func respondMediaErr(c *gin.Context, err error) {
	if errors.Is(err, oper.ErrInvalidInput) {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	errcode.Respond(c, errcode.Internal.WithErr(err))
}

// Webhook 公开接收媒体服务器播放/事件回调（媒体服务器无法带 JWT，故公开；按 :id 路由）。
// 仅当该服务器开启 NotifyOnPlay 时解析标题并通知；始终返回 200（避免服务器侧重试风暴）。
func (h *MediaServerHandler) Webhook(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"ok": false})
		return
	}
	// 公开端点：显式 1MB 上限兜底（webhook 负载本就极小），避免未授权大 body 放大。
	payload, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	h.svc.ReceiveWebhook(c.Request.Context(), id, payload)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PlaybackReport 观影报告（GET /mediaserver/:id/playback-report?days=30）。
func (h *MediaServerHandler) PlaybackReport(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	days := atoiDefault(c.Query("days"), 30)
	rep, err := h.svc.PlaybackReport(c.Request.Context(), id, days)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, rep)
}

// Health 媒体健康中心巡检（GET /mediaserver/health?subtitles=1&upgradeable=1）。全服务器扫描：
// 未匹配 / 缺集 / 缺字幕 / 可升级 四类问题（subtitles=1 开启本地字幕扫描，需后端可访问媒体路径；
// upgradeable=1 开启洗版检测，库内同 TMDB 存在更高分辨率版本时低版本条目记 upgradeable）。
func (h *MediaServerHandler) Health(c *gin.Context) {
	rep := h.svc.HealthCheck(c.Request.Context(), c.Query("subtitles") == "1", c.Query("upgradeable") == "1")
	c.JSON(http.StatusOK, rep)
}

// FixSubtitles 字幕补齐（POST /mediaserver/fix-subtitles，body=健康报告或复用现算）。
func (h *MediaServerHandler) FixSubtitles(c *gin.Context) {
	var rep chain.HealthReport
	if err := c.ShouldBindJSON(&rep); err != nil || len(rep.Issues) == 0 {
		rep = *h.svc.HealthCheck(c.Request.Context(), true, false) // body 空 → 现算全库
	}
	result, err := h.svc.FixMissingSubtitles(c.Request.Context(), &rep, c.Query("lang"))
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusAccepted, result)
}

// Duplicates 重复媒体检测（GET /mediaserver/:id/duplicates?library_id=）。
func (h *MediaServerHandler) Duplicates(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	libID := c.Query("library_id")
	if libID == "" {
		errcode.Respond(c, errcode.MediaLibraryRequired)
		return
	}
	groups, err := h.svc.DetectDuplicates(c.Request.Context(), id, libID)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

// Direct 302 直链改写（公开：GET /mediaserver/:id/direct/*path）。
// 读取服务器配置，开启直链则 302 到 {DirectLinkBase}/{path}；未开启返 404。
// 不泄露 Token——仅做静态 URL 改写，让播放器直连高带宽 CDN（对标 Emby 302 反代直链）。
func (h *MediaServerHandler) Direct(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		errcode.Respond(c, errcode.ResourceNotFound)
		return
	}
	target, _, err := h.svc.DirectLink(c.Request.Context(), id, c.Param("path"))
	if err != nil {
		// 统一返 404 + 通用文案：不区分「服务器不存在」与「未开启直链」，避免未授权枚举服务器 id。
		errcode.Respond(c, errcode.ResourceNotFound)
		return
	}
	c.Redirect(http.StatusFound, target)
}
