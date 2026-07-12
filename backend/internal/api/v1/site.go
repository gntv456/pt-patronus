package v1

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// SiteHandler 处理站点 CRUD、连接测试、搜索、校准诊断。
type SiteHandler struct {
	oper   *oper.SiteOper
	siteCh *chain.SiteChain
	reg    *site.Registry
	hc     *httpclient.Client // 校准诊断用
}

// NewSiteHandler 构造 SiteHandler。
func NewSiteHandler(o *oper.SiteOper, sc *chain.SiteChain, reg *site.Registry, hc *httpclient.Client) *SiteHandler {
	return &SiteHandler{oper: o, siteCh: sc, reg: reg, hc: hc}
}

type inspectReq struct {
	URL     string                       `json:"url" binding:"required,max=512"`
	Cookie  string                       `json:"cookie" binding:"max=65536"`
	Fields  map[string]site.ElementQuery `json:"fields"`
	NoLogin site.NoLoginDef              `json:"no_login"`
}

// Inspect 校准诊断：给定 URL+cookie+一组待试选择器，返回每个选择器抓到的文本 +
// 是否命中登录态 + 页面前 2KB。用于离线/在线校准 definitions/*.yaml 的 selectors。
func (h *SiteHandler) Inspect(c *gin.Context) {
	var req inspectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := site.Inspect(c.Request.Context(), h.hc, req.URL, req.Cookie, req.Fields, req.NoLogin)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// List 列出全部站点。
func (h *SiteHandler) List(c *gin.Context) {
	sites, err := h.oper.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sites": sites})
}

// Get 取单个站点。
func (h *SiteHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Create 创建站点。
func (h *SiteHandler) Create(c *gin.Context) {
	var in oper.SiteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s, err := h.oper.Create(in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Update 更新站点（密钥为空则保留原值）。
func (h *SiteHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.SiteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s, err := h.oper.Update(id, in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Delete 删除站点。
func (h *SiteHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.oper.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Test 测试站点连接与登录态。
func (h *SiteHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	level, err := h.siteCh.TestConnection(c.Request.Context(), s)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "level": level})
}

// Checkin 立即对单个站点签到（站点行「签到」按钮）。返回适配器签到结果；
// 适配器不支持签到（如 mteam）或网络失败时 success=false 并带 error，前端据此提示。
func (h *SiteHandler) Checkin(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.oper.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	r := chain.CheckinSiteResult{SiteID: s.ID, SiteName: s.Name}
	if res, e := h.siteCh.Checkin(c.Request.Context(), s, "manual"); e != nil {
		r.Error = e.Error()
	} else if res != nil {
		r.Success = res.Success
		r.Message = res.Message
		r.Bonus = res.Bonus
	}
	c.JSON(http.StatusOK, r)
}

// CheckinAll 一键签到全部启用站点（顶部「一键签到」按钮），并发执行，返回每站结果。
func (h *SiteHandler) CheckinAll(c *gin.Context) {
	results := h.siteCh.CheckinAll(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// CheckinRecords 列签到历史记录（?site_id=&limit=&offset=；按时间倒序）。
func (h *SiteHandler) CheckinRecords(c *gin.Context) {
	siteID, _ := strconv.ParseUint(c.Query("site_id"), 10, 64)
	limit := atoiDefault(c.Query("limit"), 100)
	offset := atoiDefault(c.Query("offset"), 0)
	records, err := h.siteCh.ListCheckinRecords(uint(siteID), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": records})
}

// Search 在单个站点搜索。
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

func siteDefinitionsForUI(defs []*site.SiteMetadata) []*site.SiteMetadata {
	groups := make([][]*site.SiteMetadata, 0, len(defs))
	hostGroup := map[string]int{}
	for _, d := range defs {
		if d == nil {
			continue
		}
		hosts := definitionHosts(d)
		groupIdx := -1
		for _, host := range hosts {
			if idx, ok := hostGroup[host]; ok {
				groupIdx = idx
				break
			}
		}
		if groupIdx < 0 {
			groupIdx = len(groups)
			groups = append(groups, nil)
		}
		groups[groupIdx] = append(groups[groupIdx], d)
		for _, host := range hosts {
			hostGroup[host] = groupIdx
		}
	}

	out := make([]*site.SiteMetadata, 0, len(groups))
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			si, sj := definitionScore(group[i]), definitionScore(group[j])
			if si != sj {
				return si > sj
			}
			return group[i].ID < group[j].ID
		})
		kept := *group[0]
		kept.URLs = mergeDefinitionURLs(group)
		kept.AKA = mergeDefinitionAKA(&kept, group)
		kept.Tags = mergeStrings(append([]string{}, kept.Tags...), collectTags(group)...)
		out = append(out, &kept)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Dead != out[j].Dead {
			return !out[i].Dead
		}
		ni, nj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if ni != nj {
			return ni < nj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func definitionHosts(d *site.SiteMetadata) []string {
	urls := append([]string{}, d.URLs...)
	urls = append(urls, d.LegacyURLs...)
	hosts := make([]string, 0, len(urls))
	seen := map[string]bool{}
	for _, raw := range urls {
		host := normalizedURLHost(raw)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	if len(hosts) == 0 {
		return []string{"id:" + d.ID}
	}
	return hosts
}

func normalizedURLHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	return strings.TrimPrefix(host, "www.")
}

func definitionScore(d *site.SiteMetadata) int {
	if d == nil {
		return 0
	}
	score := 0
	if !d.Dead {
		score += 100000
	}
	score += len(d.Categories) * 20
	score += len(d.LevelRequirements) * 20
	score += len(d.Selectors.Fields) * 8
	score += len(d.UserInfo.Fields) * 8
	score += len(d.UserInfo.DetailFields) * 6
	score += len(d.Detail.Fields) * 4
	score += len(d.SearchCategories) * 4
	score += len(d.AKA) * 2
	score += len(d.Tags)
	score += len(d.URLs) + len(d.LegacyURLs)
	if d.Description != "" {
		score += 5
	}
	return score
}

func mergeDefinitionURLs(group []*site.SiteMetadata) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, d := range group {
		urls := append([]string{}, d.URLs...)
		urls = append(urls, d.LegacyURLs...)
		for _, raw := range urls {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			key := normalizedURLKey(raw)
			if key == "" {
				key = raw
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, raw)
		}
	}
	return out
}

func normalizedURLKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.TrimRight(u.EscapedPath(), "/")
	if path == "" {
		path = "/"
	}
	return strings.ToLower(u.Scheme) + "://" + host + path
}

func mergeDefinitionAKA(kept *site.SiteMetadata, group []*site.SiteMetadata) []string {
	out := append([]string{}, kept.AKA...)
	for _, d := range group {
		if d.Name != "" && d.Name != kept.Name {
			out = mergeStrings(out, d.Name)
		}
		out = mergeStrings(out, d.AKA...)
	}
	return out
}

func collectTags(group []*site.SiteMetadata) []string {
	out := []string{}
	for _, d := range group {
		out = append(out, d.Tags...)
	}
	return out
}

func mergeStrings(base []string, vals ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(vals))
	for _, v := range append(base, vals...) {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// parseUintSlice 解析逗号分隔的 uint 列表（如 "1,2,3"），忽略空串/非法项；空输入返回 nil。
func parseUintSlice(s string) []uint {
	if s == "" {
		return nil
	}
	out := make([]uint, 0, 4)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, uint(n))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
