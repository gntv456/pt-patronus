// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
)

func (h *SiteHandler) Search(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		errcode.Respond(c, errcode.MediaKeywordRequired)
		return
	}
	category := c.Query("category") // movie/tv/music（空=综合）
	page := atoiDefault(c.Query("page"), 1)
	s, err := h.oper.Get(id)
	if err != nil {
		respondSiteLookup(c, err, h.oper)
		return
	}
	ts, err := h.siteCh.Search(c.Request.Context(), s, keyword, category, page)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"torrents": []any{}, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"torrents": ts})
}

// SearchAll 跨启用站点聚合搜索。?site_ids=1,2,3 仅搜索指定启用站点子集（空=全部启用）。
// keyword 空/纯空白 → 400（与 /media/search 一致）：空关键词会把「种子列表首页」当搜索结果
// 扇出到全部站点，浪费配额且结果与关键词无关。
func (h *SiteHandler) SearchAll(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		errcode.Respond(c, errcode.MediaKeywordRequired)
		return
	}
	category := c.Query("category") // movie/tv/music（空=综合）
	siteIDs := parseUintSlice(c.Query("site_ids"))
	results, err := h.siteCh.SearchSites(c.Request.Context(), keyword, category, siteIDs)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if results == nil {
		results = []chain.SiteResult{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// SearchAllStream SSE 流式跨站搜索：每站完成推一帧 event:batch（results 为单元素数组），
// 供搜索页「搜出一个显示一个」；结束推 event:done。失败站也推送（Error 字段承载错误）。
// keyword 空/纯空白 → 400（与 SearchAll 同因）：SSE 已写 200 头后无法改码，须在 beginSSE 前校验。
func (h *SiteHandler) SearchAllStream(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		errcode.Respond(c, errcode.MediaKeywordRequired)
		return
	}
	category := c.Query("category")
	siteIDs := parseUintSlice(c.Query("site_ids"))
	emit := beginSSE(c.Writer)
	listErr := h.siteCh.SearchSitesStream(c.Request.Context(), keyword, category, siteIDs, func(r chain.SiteResult) {
		emit("batch", gin.H{"results": []chain.SiteResult{r}})
	})
	done := gin.H{}
	if listErr != "" {
		done["list_error"] = listErr
	}
	emit("done", done)
}

// TorrentDetail 抓取并解析单条种子详情页（?url=<详情页绝对 URL>）。
// URL 必须落在该站点 baseURL 的同注册域下（chain 内 sameHost 守卫）。
func (h *SiteHandler) TorrentDetail(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	detailURL := c.Query("url")
	if detailURL == "" {
		errcode.Respond(c, errcode.UrlRequired)
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		respondSiteLookup(c, err, h.oper)
		return
	}
	d, err := h.siteCh.TorrentDetail(c.Request.Context(), s, detailURL)
	if err != nil {
		// #399 归统一判据 respondFetchError，不再自己抄一份 WithErr。
		//
		// 旧形状（裸 BadGateway.WithErr）有两处错：
		//  1. **绕过 SSRF 分档**：站点建在内网被守卫拦下时，也只回「上游服务不可用」，
		//     而用户唯一能做的动作（换公网地址或开 ALLOW_PRIVATE_SITES）没被说出来；
		//  2. **不认凭据哨兵**：TorrentDetail 链层第一步就是 credentialWithErr，
		//     主密钥换过时那句「请到『站点』里重新保存凭据；本次未向站点发请求」
		//     被整段丢进日志，界面只见「上游服务不可用，请稍后重试」。
		//
		// 与 message 的四个出口同型（同一份「抓站点」判据），故走同一个函数，
		// 而不是在这里再抄一遍 errors.Is ——#377 的教训正是「哨兵铺开了、出口没跟上」。
		respondFetchError(c, err, errcode.BadGateway)
		return
	}
	c.JSON(http.StatusOK, gin.H{"detail": d})
}

type cookieReq struct {
	Cookie string `json:"cookie" binding:"required,max=65536"`
}

// UpdateCookie 上传站点 cookie（WebView 登录后由客户端调用）。
func (h *SiteHandler) UpdateCookie(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req cookieReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.oper.UpdateCookie(id, req.Cookie); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetCookies 站点 Cookie 台账（浏览器扩展对比视图用）：
// 返回全部站点的 cookie 指纹（sha256 前 12 位 + 长度 + 更新时间可辨异同，不泄露明文）。
// 完整明文属敏感操作，与 backup/export 同级——仅在路由层挂 RequireAdmin 的
// GetCookiesReveal 端点上开放；本端点不受任何 query 参数控制（防 ?reveal=1 穿透
// admin 闸门：此前 GetCookiesReveal 靠改写 RawQuery 复用本函数，导致非 admin 的
// /sites/cookies 也能被 query 参数打开明文回显）。
func (h *SiteHandler) GetCookies(c *gin.Context) {
	h.cookies(c, false)
}

// GetCookiesReveal 是 GetCookies 的明文版；路由层 RequireAdmin 收敛，
// 使「把服务器 cookie 拉到浏览器」与 backup/export 同为管理员能力。
func (h *SiteHandler) GetCookiesReveal(c *gin.Context) {
	h.cookies(c, true)
}

// cookies 台账实现。reveal 仅由本文件内的 admin 端点置 true，与请求参数无关。
func (h *SiteHandler) cookies(c *gin.Context, reveal bool) {
	sites, err := h.oper.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	out := make([]gin.H, 0, len(sites))
	for _, s := range sites {
		item := gin.H{
			"id": s.ID, "name": s.Name, "base_url": s.BaseURL,
			"is_active": s.IsActive, "has_cookie": s.CookieEnc != "",
		}
		if s.CookieEnc != "" {
			ck, err := h.oper.Cookie(&s)
			if err != nil {
				// 解密失败（如库里是旧 SECRET_KEY 时代加密的数据）：如实标记为
				// 「密文不可用」而非冒充有效指纹——扩展台账据此显示该站需重新录入。
				item["has_cookie"] = false
				item["stale"] = true
			} else {
				sum := sha256.Sum256([]byte(ck))
				fp := hex.EncodeToString(sum[:])[:12]
				item["fingerprint"] = fp
				item["length"] = len(ck)
				if reveal {
					item["cookie"] = ck
				}
			}
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"sites": out})
}

// Definitions 列出可用站点定义（供 UI 选站点模板）。
func (h *SiteHandler) Definitions(c *gin.Context) {
	defs := siteDefinitionsForUI(h.reg.Definitions())
	out := make([]gin.H, 0, len(defs))
	for _, d := range defs {
		// urls：站点真实地址列表（取首个作「添加站点」默认 base_url）。
		// dead：死站标记。UI 选站模板据此标注（死站不参与活跃抓取，仅保留信息可展示/可选）。
		// aka/description/tags：含中文名别名与描述，供 UI 搜索（很多站 name 是英文，靠 aka 中文名才能检索到）。
		out = append(out, gin.H{"id": d.ID, "name": d.Name, "schema": d.Schema, "dead": d.Dead, "urls": d.URLs, "aka": d.AKA, "description": d.Description, "tags": d.Tags})
	}
	// diagnostics：四类「站点定义被静默忽略」的账面（正则编不出来 / 未知键 / 同目录重复 id /
	// 子目录没读）。只有日志时用户看到的是「这个站抓不到东西」，界面读到了才看得出「有 N 条规则没生效」。
	c.JSON(http.StatusOK, gin.H{"definitions": out, "diagnostics": h.reg.Diagnostics()})
}

// DefinitionDiagnostics 单独给出「站点定义被静默忽略」的账面（四类：正则编不出来 /
// 未知键 / 同目录重复 id / 子目录没读）。独立端点是为了让界面能廉价地只拉这一小张表，
// 不必把 300 多条定义整个再读一遍。
func (h *SiteHandler) DefinitionDiagnostics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"diagnostics": h.reg.Diagnostics()})
}
