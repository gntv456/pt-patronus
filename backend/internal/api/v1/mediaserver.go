package v1

import (
	"errors"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/mediaserver"
	"gorm.io/gorm"
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
		respondMediaServerError(c, err)
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
		respondMediaServerError(c, err)
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
	if !requireExists(c, h.svc.Get, id, errcode.MediaServerNotFound) {
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
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": mediaServerErrText(err)})
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
		respondMediaServerError(c, err)
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
		respondMediaServerError(c, err)
		return
	}
	if body.LibraryID != "" {
		if err := h.svc.LibraryScan(c.Request.Context(), id, body.LibraryID); err != nil {
			opResult(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "scanned": 1})
		return
	}
	// 空 library_id：扫该服务器全部库（与工具面共用 LibraryScanAll，同一套「几个库扫成了」的账）。
	// 此前本地自实现逐个扫描、第一个库失败就整体报错，扫成 1/5 与一个都没扫成回执相同。
	n, err := h.svc.LibraryScanAll(c.Request.Context(), id)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "scanned": n})
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
	data, st, err := h.svc.Cover(c.Request.Context(), id, libID, opt)
	if err != nil {
		// 一张都没贴上不是「这批片子没海报」：回错误让界面把数字与去处说清楚，
		// 而不是发一张全黑的 200 成功图（黑图会被读成「没有封面」，用户从此不再看封面墙）。
		if errors.Is(err, mediaserver.ErrNoCoverPlaced) {
			errcode.Respond(c, errcode.MediaCoverNoImage.WithDetail(err))
			return
		}
		respondMediaServerError(c, err)
		return
	}
	if st.NoImage+st.FetchFail > 0 {
		// 部分留黑：两种原因的数量挂响应头并打日志。<img> 通道看不到响应头，
		// 但界面走的是 blob 请求（coverBlob），排障与自动化都能对上账。
		c.Header("X-Cover-Stats", fmt.Sprintf("items=%d placed=%d no_image=%d fetch_fail=%d",
			st.Items, st.Placed, st.NoImage, st.FetchFail))
		slog.Warn("mediaserver: cover cells left blank", "server", id, "library", libID,
			"items", st.Items, "placed", st.Placed, "no_image", st.NoImage, "fetch_fail", st.FetchFail)
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
		respondMediaServerError(c, err)
		return
	}
	if body.Title == "" {
		body.Title = "PTPatronus 通知"
	}
	opResult(c, h.svc.NotifyOne(c.Request.Context(), id, body.Title, body.Body))
}

// respondMediaErr 把 oper 错误映射到状态码：记录不存在 → 404，输入校验 → 400 带原因，其余 → 500。
func respondMediaErr(c *gin.Context, err error) {
	respondRecordError(c, err, errcode.MediaServerNotFound, errcode.InvalidInput, errcode.Internal)
}

// Webhook 公开接收媒体服务器播放/事件回调（媒体服务器无法带 JWT，故公开；按 :id 路由）。
// 来源凭证：?token= 或 X-Emby-Token 头——服务器配置了 API Token 时 chain 层恒定时间
// 比对，不匹配静默忽略（防匿名伪造通知注入）。仅当该服务器开启 NotifyOnPlay 时解析
// 标题并通知；始终返回 200（避免服务器侧重试风暴）。
func (h *MediaServerHandler) Webhook(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		// parseID 已经回过 400 信封（AbortWithStatusJSON），这里再写一次会让响应体里
		// 出现两段 JSON —— 客户端解析必失败，等于把「哪个 id 不对」这句有用的话送不出去。
		return
	}
	// 公开端点：显式 1MB 上限兜底（webhook 负载本就极小），避免未授权大 body 放大。
	payload, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("X-Emby-Token")
	}
	h.svc.ReceiveWebhook(c.Request.Context(), id, payload, token)
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
		respondMediaServerError(c, err)
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
		// 防重入冲突是 409 语义（五轮 P2 修复：此前映射 400"参数无效"误导用户）。
		if errors.Is(err, chain.ErrSubtitleFixRunning) {
			errcode.Respond(c, errcode.Conflict.WithErr(err))
			return
		}
		respondMediaServerError(c, err)
		return
	}
	// 只回「受理了多少部」。此前这里序列化的是 Fixed/Skipped/Errors：三个字段由后台
	// goroutine 逐条累加（写者全程持 subtitleFix.mu，读者不加锁＝data race），
	// 而受理那一刻它们恒为 0 ——回出去的账面既无意义又谎报「补齐 0 条」。
	// 最终账面在通知里（notice.SourceSubtitleFix），界面据 queued=0 明确报「没东西可补」。
	c.JSON(http.StatusAccepted, gin.H{
		"queued":  result.Queued,
		"running": result.Queued > 0,
	})
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
		respondMediaServerError(c, err)
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
		// parseID 已经回过 400 信封（AbortWithStatusJSON），这里再写一次会让响应体里
		// 出现两段 JSON —— 客户端解析必失败，等于把「哪个 id 不对」这句有用的话送不出去。
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

// respondMediaServerError 库里查不到这台服务器要说「媒体服务器不存在」(404)，不能混成
// 「请求参数有误」(400)：删掉服务器后另一个标签页仍在用旧 id、书签里的旧 id 都会走到这里，
// 400 会把用户支去改参数而不是重选服务器。
func respondMediaServerError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		errcode.Respond(c, errcode.MediaServerNotFound)
		return
	}
	// oper 的输入校验文案是写给人看的中文（「地址必须是 http/https 协议…」），
	// 直接带回；只有别种错误才退回泛化的「参数有误」。
	respondInputError(c, err, errcode.InvalidInput)
}

// mediaServerErrText 给 200+{ok:false,error} 软失败面用：raw Go 错误（"record not found"）
// 会被前端原样弹给用户，这里换成中文。
//
// 传输层失败（连不上 / 超时 / 缺协议 / SSRF）交给 errtext 这一处单一来源，
// 与站点、下载器的「测试连接」同一口径——从前这里只认 record not found，
// 连接被拒时界面弹的是 `Get "http://…": dial tcp … connectex: No connection…`。
func mediaServerErrText(err error) string {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "媒体服务器不存在"
	}
	return userErrText(err)
}
