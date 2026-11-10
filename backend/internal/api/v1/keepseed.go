// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/site"
)

// respondKeepSeedConfigWriteErr 辅种配置写入失败的出口（#392）。
//
// SetConfig 是「多次独立写、无事务」：先 SetToken（若给了），再 SetTorznab（若勾了）。
// 而 SystemConfigOper.Set 每个 key 一次独立读写，没有事务（systemconfig.go）——
// 所以任何一步失败，**前面已经写进去的仍留在库里**。
//
// tokenSaved 这个参数就在说这件事：它为真 ⇒ 「IYUU token 已经生效了，
// 失败的是后面的 Torznab」。用户据此才知道该不该重试、以及下次辅种会用什么。
//
// 旧形状是 Internal.WithErr ⇒ 500「服务内部错误，请稍后重试」。那句「请稍后重试」
// 在这里最坏：用户以为「什么都没存」（于是再点一次保存），而实际上 token 已经换了——
// 下次辅种会用新 token，他的直觉与现实正好相反。Torznab 那层更隐蔽：
// URL 已生效、Key 还是旧的，于是 Jackett/Prowlarr 每次请求都 401，界面却说「保存失败」。
func respondKeepSeedConfigWriteErr(c *gin.Context, err error, tokenSaved bool) {
	if tokenSaved {
		errcode.Respond(c, errcode.KeepSeedConfigPartialSaved.WithDetailKeepMessage(err))
		return
	}
	errcode.Respond(c, errcode.KeepSeedConfigWriteFailed.WithDetailKeepMessage(err))
}

// respondKeepSeedErr 辅种、查漏、处理候选都是「替用户去动下载器」的调用。
// 从前下载器根本连不上（客户端没开 / 端口写错）也报 400「请求参数有误，请检查后重试」，
// 用户回头改表单、改不动——问题在客户端那一侧，要给 502 和下一步动作。
func respondKeepSeedErr(c *gin.Context, err error) {
	if errors.Is(err, downloader.ErrCannotConnect) {
		errcode.Respond(c, errcode.DownloaderConnFailed.WithErr(err))
		return
	}
	// 查漏的两个前置条件原因是链层写给人看的中文（没配 token / 没选目标站），
	// 退回「请求参数有误」等于把下一步藏起来。
	if errors.Is(err, chain.ErrMissingNoToken) || errors.Is(err, chain.ErrMissingNoTarget) {
		errcode.Respond(c, errcode.InvalidInput.WithDetail(err))
		return
	}
	// #369 处理候选的三条「用户自己能修」的原因（链层为此立了哨兵）：
	// 没注入 MTeam 客户端 / 该站没配 api_key / action 填错。
	// 从前它们是裸 errors.New，接口层分不出它们与「内部故障」，于是全被压成
	// 「没能指出具体是哪一项：对照接口文档核对必填项」—— 让用户去改 HTTP 表单。
	// 放行方式与上面 ErrMissingNoToken 同构：WithDetail 把原话带给界面。
	// 其余错误仍走下面的 respondInputError：不外泄内部原文（既有判据钉着这条界）。
	if errors.Is(err, chain.ErrNoMTeamClient) || errors.Is(err, chain.ErrMTeamNoAPIKey) ||
		errors.Is(err, chain.ErrBadAction) {
		errcode.Respond(c, errcode.InvalidInput.WithDetail(err))
		return
	}
	respondInputError(c, err, errcode.InvalidInput)
}

// KeepSeedHandler 处理辅种候选查询。
type KeepSeedHandler struct {
	ks *chain.KeepSeedChain
}

// NewKeepSeedHandler 构造。
func NewKeepSeedHandler(ks *chain.KeepSeedChain) *KeepSeedHandler {
	return &KeepSeedHandler{ks: ks}
}

type findReq struct {
	Name string `json:"name" binding:"required,max=512"`
	Size int64  `json:"size" binding:"min=0,max=1099511627776"` // 字节；为 0 仅按名称评分（上限 1TB）
}

// Find 按名称/大小跨站查找辅种候选。
func (h *KeepSeedHandler) Find(c *gin.Context) {
	var req findReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	cands := h.ks.FindCandidates(c.Request.Context(), req.Name, req.Size)
	// 转 JSON 友好结构
	out := make([]gin.H, 0, len(cands))
	for _, cd := range cands {
		out = append(out, gin.H{
			"site_id":   cd.SiteID,
			"site_name": cd.SiteName,
			"score":     cd.Score,
			"torrent":   toTorrentJSON(cd.Torrent),
		})
	}
	c.JSON(http.StatusOK, gin.H{"candidates": out})
}

// toTorrentJSON 把 site.Torrent 转为可读 map。
func toTorrentJSON(t site.Torrent) gin.H {
	return gin.H{
		"title":        t.Title,
		"size":         t.Size,
		"size_text":    "",
		"seeders":      t.Seeders,
		"detail_url":   t.DetailURL,
		"download_url": t.DownloadURL,
	}
}

// ---- IYUU 精确辅种 ----

// GetConfig 返回辅种配置（IYUU token 掩码 + Torznab 端点）。
func (h *KeepSeedHandler) GetConfig(c *gin.Context) {
	token, err := h.ks.GetToken()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	masked := ""
	if token != "" {
		if len(token) > 8 {
			masked = token[:4] + "****" + token[len(token)-4:]
		} else {
			masked = "****"
		}
	}
	tzURL, tzKey, _ := h.ks.GetTorznab()
	tzKeyMasked := ""
	if tzKey != "" {
		if len(tzKey) > 6 {
			tzKeyMasked = tzKey[:3] + "****" + tzKey[len(tzKey)-2:]
		} else {
			tzKeyMasked = "****"
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"iyuu_token_masked":  masked,
		"configured":         token != "",
		"torznab_url":        tzURL,
		"torznab_key_masked": tzKeyMasked,
		"torznab_configured": strings.TrimSpace(tzURL) != "",
	})
}

type configReq struct {
	IYUUToken  string `json:"iyuu_token" binding:"max=256"`
	TorznabURL string `json:"torznab_url" binding:"max=512"`
	TorznabKey string `json:"torznab_apikey" binding:"max=256"`
	// HasTorznab 为 true 时才写 Torznab（避免仅改 IYUU 时清空端点）
	SetTorznab bool `json:"set_torznab"`
}

// SetConfig 保存辅种配置（IYUU token / 可选 Torznab）。
func (h *KeepSeedHandler) SetConfig(c *gin.Context) {
	var req configReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	// #392 tokenSaved 记「前面已经写进去多少」，失败出口据此说清（见 respondKeepSeedConfigWriteErr）。
	tokenSaved := false
	if tok := strings.TrimSpace(req.IYUUToken); tok != "" {
		if err := h.ks.SetToken(tok); err != nil {
			respondKeepSeedConfigWriteErr(c, err, false)
			return
		}
		tokenSaved = true
	}
	if req.SetTorznab {
		if err := h.ks.SetTorznab(req.TorznabURL, req.TorznabKey); err != nil {
			// #392 走到这里时 IYUU token 可能已经生效（无事务），必须说清。
			respondKeepSeedConfigWriteErr(c, err, tokenSaved)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type reseedReq struct {
	DownloaderID uint `json:"downloader_id" binding:"required,min=1"`
}

// Reseed 触发某下载器的 IYUU 精确辅种，返回新增 pending 候选。
func (h *KeepSeedHandler) Reseed(c *gin.Context) {
	var req reseedReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	created, err := h.ks.Reseed(c.Request.Context(), req.DownloaderID)
	if err != nil {
		respondKeepSeedErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": created, "count": len(created)})
}

type missingReq struct {
	DownloaderID  uint   `json:"downloader_id" binding:"required,min=1"`
	TargetSiteIDs []uint `json:"target_site_ids"`
}

// Missing 未辅种检查：对某下载器的做种，检查目标站点是否已辅种（IYUU hash 跨站匹配），返回未辅种清单。
func (h *KeepSeedHandler) Missing(c *gin.Context) {
	var req missingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	summary, err := h.ks.FindMissing(c.Request.Context(), req.DownloaderID, req.TargetSiteIDs)
	if err != nil {
		respondKeepSeedErr(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

// Tasks 列出辅种候选任务（?status=pending|added|skipped|failed；?site_id= 按候选站过滤）。
func (h *KeepSeedHandler) Tasks(c *gin.Context) {
	var siteID uint
	if s := c.Query("site_id"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 64); err == nil && n > 0 {
			siteID = uint(n)
		}
	}
	tasks, err := h.ks.ListTasks(c.Query("status"), siteID)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// 真实总数：列表有上限（chain 侧 500 条），不把 total/truncated 带出去，界面就会把
	// 「最近 500 条」渲染成「全部候选」，清空后还谎报「没有待处理候选」。
	// 计数失败时宁可保守报「可能被截断」，也绝不给出「这是全部」的假安心。
	total, cntErr := h.ks.CountTasks(c.Query("status"), siteID)
	truncated := cntErr != nil || len(tasks) >= chain.KeepseedTaskLimit
	if cntErr != nil {
		slog.Warn("keepseed count tasks failed", "err", cntErr)
	}
	resp := gin.H{"tasks": tasks, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

type resolveReq struct {
	Action string `json:"action" binding:"required,max=16"` // add | skip
}

// Resolve 处理一个候选任务：添加辅种 / 跳过。
func (h *KeepSeedHandler) Resolve(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	var req resolveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.ks.Resolve(c.Request.Context(), uint(id), req.Action); err != nil {
		respondKeepSeedErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetSetting 返回该下载器的辅种设置（auto_reseed / auto_reseed_add / save_path / pause_after_add）。
func (h *KeepSeedHandler) GetSetting(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("downloader_id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	s, err := h.ks.Setting(uint(id))
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, s)
}

type settingReq struct {
	DownloaderID  uint   `json:"downloader_id" binding:"required,min=1"`
	SavePath      string `json:"save_path" binding:"max=512"`
	PauseAfterAdd bool   `json:"pause_after_add"`
	AutoReseed    bool   `json:"auto_reseed"`
	AutoReseedAdd bool   `json:"auto_reseed_add"`
}

// SetSetting upsert 该下载器的辅种设置。
func (h *KeepSeedHandler) SetSetting(c *gin.Context) {
	var req settingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	s := models.CrossSeedSetting{
		DownloaderID:  req.DownloaderID,
		SavePath:      req.SavePath,
		PauseAfterAdd: req.PauseAfterAdd,
		AutoReseed:    req.AutoReseed,
		AutoReseedAdd: req.AutoReseedAdd,
	}
	if err := h.ks.SaveSetting(s); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
