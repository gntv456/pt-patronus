package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
)

func (h *SiteHandler) Search(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	keyword := c.Query("keyword")
	category := c.Query("category") // movie/tv/music（空=综合）
	page := atoiDefault(c.Query("page"), 1)
	s, err := h.oper.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	ts, err := h.siteCh.Search(c.Request.Context(), s, keyword, category, page)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"torrents": []any{}, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"torrents": ts})
}

// SearchAll 跨启用站点聚合搜索。?site_ids=1,2,3 仅搜索指定启用站点子集（空=全部启用）。
func (h *SiteHandler) SearchAll(c *gin.Context) {
	keyword := c.Query("keyword")
	category := c.Query("category") // movie/tv/music（空=综合）
	siteIDs := parseUintSlice(c.Query("site_ids"))
	results := h.siteCh.SearchSites(c.Request.Context(), keyword, category, siteIDs)
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// SearchAllStream SSE 流式跨站搜索：每站完成推一帧 event:batch（results 为单元素数组），
// 供搜索页「搜出一个显示一个」；结束推 event:done。失败站也推送（Error 字段承载错误）。
func (h *SiteHandler) SearchAllStream(c *gin.Context) {
	keyword := c.Query("keyword")
	category := c.Query("category")
	siteIDs := parseUintSlice(c.Query("site_ids"))
	emit := beginSSE(c.Writer)
	h.siteCh.SearchSitesStream(c.Request.Context(), keyword, category, siteIDs, func(r chain.SiteResult) {
		emit("batch", gin.H{"results": []chain.SiteResult{r}})
	})
	emit("done", gin.H{})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "url required"})
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	d, err := h.siteCh.TorrentDetail(c.Request.Context(), s, detailURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.oper.UpdateCookie(id, req.Cookie); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
	c.JSON(http.StatusOK, gin.H{"definitions": out})
}
