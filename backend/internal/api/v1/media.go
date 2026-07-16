package v1

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/media"
)

// MediaHandler 处理媒体搜索/海报/NFO/推荐/探索。
type MediaHandler struct {
	mediaCh   *chain.MediaChain
	recommend *chain.RecommendChain
}

// NewMediaHandler 构造。
func NewMediaHandler(mediaCh *chain.MediaChain, recommend *chain.RecommendChain) *MediaHandler {
	return &MediaHandler{mediaCh: mediaCh, recommend: recommend}
}

// Search 跨源搜索媒体。「影视」端点默认只返回 movie+tv（排除音乐），
// 可用 ?type= 覆盖（""=全部 / movie / tv / music）。
func (h *MediaHandler) Search(c *gin.Context) {
	keyword := c.Query("keyword")
	year := atoiDefault(c.Query("year"), 0)
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword required"})
		return
	}
	filter := c.DefaultQuery("type", "video")
	res, err := h.mediaCh.Search(c.Request.Context(), keyword, year, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": res})
}

// Recommend 热门/趋势信息流（?source=douban|tmdb|bangumi&category=movie|tv|anime|coming&page=1&refresh=1）。
// source 空则跨源合并。各源走 TrendingCapable（豆瓣热映/TMDB trending/Bangumi 热门）。
func (h *MediaHandler) Recommend(c *gin.Context) {
	source := c.Query("source")
	category := c.DefaultQuery("category", "movie")
	page := atoiDefault(c.Query("page"), 1)
	force := c.Query("refresh") == "1"
	if h.recommend == nil {
		c.JSON(http.StatusOK, gin.H{"results": []any{}})
		return
	}
	items := h.recommend.RecommendFeeds(c.Request.Context(), source, category, page, force)
	c.JSON(http.StatusOK, gin.H{"results": items})
}

// RecommendStream SSE 流式推送趋势信息流（?source=&category=&page=&refresh=1）。
// 每源完成即推一帧 event:batch（results 为该源条目）；全部完成推 event:done。
// 供推荐页「加载一个显示一个」——边抓边显，不等最慢源。emit 经 beginSSE 串行化，chain 并发回调安全。
func (h *MediaHandler) RecommendStream(c *gin.Context) {
	if h.recommend == nil {
		return
	}
	source := c.Query("source")
	category := c.DefaultQuery("category", "movie")
	page := atoiDefault(c.Query("page"), 1)
	force := c.Query("refresh") == "1"
	emit := beginSSE(c.Writer)
	h.recommend.RecommendFeedsStream(c.Request.Context(), source, category, page, force, func(items []media.MediaInfo) {
		emit("batch", gin.H{"results": items})
	})
	emit("done", gin.H{"page": page})
}

// Discover 跨源关键词搜索媒体（?keyword=&year=&source=&type=）。复用 MediaChain.Search。
// type 默认 video（movie+tv，排除音乐）；source 非空时再按来源过滤。空 keyword 报错。
func (h *MediaHandler) Discover(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword required"})
		return
	}
	year := atoiDefault(c.Query("year"), 0)
	filter := c.DefaultQuery("type", "video")
	res, err := h.mediaCh.Search(c.Request.Context(), keyword, year, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if src := c.Query("source"); src != "" {
		filtered := make([]media.MediaInfo, 0, len(res))
		for _, m := range res {
			if m.Source == src {
				filtered = append(filtered, m)
			}
		}
		res = filtered
	}
	c.JSON(http.StatusOK, gin.H{"results": res})
}
func (h *MediaHandler) Poster(c *gin.Context) {
	keyword := c.Query("keyword")
	year := atoiDefault(c.Query("year"), 0)
	url, err := h.mediaCh.MatchPoster(c.Request.Context(), keyword, year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"poster_url": url})
}

type nfoReq struct {
	Info media.MediaInfo `json:"info"`
}

// maskSecret 脱敏：>8 字符显首尾各 4 位 + ****，否则全 ****；空串原样返回（前端据此判断未配置）。
// 与 KeepSeedHandler 同款风格，避免把 secret 明文回传前端。
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Detail 取媒体富详情（演职/推荐/时长/出品国等）。?source=tmdb|douban|bangumi&id=movie:123。
// 供「媒体详情页」用——点搜索结果卡片进详情，而非跳外站。
func (h *MediaHandler) Detail(c *gin.Context) {
	source := c.Query("source")
	id := c.Query("id")
	if source == "" || id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and id required"})
		return
	}
	res, err := h.mediaCh.Detail(c.Request.Context(), source, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// Celebrity 拉演员简介（点 cast 头像用）。
func (h *MediaHandler) Celebrity(c *gin.Context) {
	source := c.Query("source")
	id := c.Query("id")
	if source == "" || id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and id required"})
		return
	}
	res, err := h.mediaCh.Celebrity(c.Request.Context(), source, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// NFO 生成 NFO。
func (h *MediaHandler) NFO(c *gin.Context) {
	var req nfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nfo": h.mediaCh.NFO(&req.Info)})
}

// GetQuality 返回多评分订阅质量配置（GET /media/quality）。
func (h *MediaHandler) GetQuality(c *gin.Context) {
	c.JSON(http.StatusOK, h.mediaCh.LoadQuality())
}

// SetQuality 保存质量配置（PUT /media/quality）。整体覆盖；清空评分缓存即时生效。
func (h *MediaHandler) SetQuality(c *gin.Context) {
	var q chain.MediaQualityConfig
	if err := c.ShouldBindJSON(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mediaCh.SaveQuality(q); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Rating 多源评分聚合预览（GET /media/rating?title=&year=&type=）。
// 供「媒体质量」页即时预览某作品的跨源综合评分 + 闸门判定。
func (h *MediaHandler) Rating(c *gin.Context) {
	title := strings.TrimSpace(c.Query("title"))
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title required"})
		return
	}
	year := atoiDefault(c.Query("year"), 0)
	summary, err := h.mediaCh.AggregatedRating(c.Request.Context(), title, year, c.Query("type"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// mediaImageHosts 允许反代的媒体图片 CDN：host 后缀 → 注入的 Referer。
// 白名单既防 SSRF（只放行公网 CDN），又解决豆瓣防盗链（直连回 418/1x1 占位图）。
var mediaImageHosts = map[string]string{
	"doubanio.com":   "https://movie.douban.com/",
	"image.tmdb.org": "", // TMDB 不防盗链，无需 Referer
	"lain.bgm.tv":    "", // Bangumi CDN，不防盗链
	"bgm.tv":         "", // Bangumi 回退 host
}

// mediaImageReferer 按 host 匹配白名单，命中返回需要注入的 Referer。
func mediaImageReferer(host string) (string, bool) {
	h := strings.ToLower(host)
	for suffix, ref := range mediaImageHosts {
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return ref, true
		}
	}
	return "", false
}

// Image 反代媒体海报图片。仅放行白名单 CDN；按主机注入合适 Referer 后流式回传，
// 限 5MB。解决豆瓣 doubanio.com 防盗链——浏览器直连只回 1x1 占位图。
func (h *MediaHandler) Image(c *gin.Context) {
	raw := c.Query("url")
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url required"})
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}
	referer, ok := mediaImageReferer(u.Host)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "host not allowed"})
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := mediaClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	ctype := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(ctype, "image/") {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("upstream %d %s", resp.StatusCode, ctype)})
		return
	}
	c.Header("Content-Type", ctype)
	c.Header("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(c.Writer, io.LimitReader(resp.Body, 5<<20))
}
