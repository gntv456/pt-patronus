package v1

import (
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"io"
	"log/slog"
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
	subCh     *chain.SubscribeChain // 订阅日历用（可选；nil 时该端点 503）
}

// NewMediaHandler 构造。
func NewMediaHandler(mediaCh *chain.MediaChain, recommend *chain.RecommendChain) *MediaHandler {
	return &MediaHandler{mediaCh: mediaCh, recommend: recommend}
}

// SetSubscribeChain 注入订阅链（订阅日历需要；构造期 subCh 晚于 mediaH 构建时后注入）。
func (h *MediaHandler) SetSubscribeChain(subCh *chain.SubscribeChain) { h.subCh = subCh }

// tmdbSource 从当前源列表取出 TMDB（未启用/未配置返回 nil）。
func (h *MediaHandler) tmdbSource() *media.TMDB {
	for _, s := range h.mediaCh.Provider().Sources() {
		if t, ok := s.(*media.TMDB); ok {
			return t
		}
	}
	return nil
}

// Calendar 开播日历（GET /media/calendar?kind=tv|movie&days=14，对标 MSaber 开播日历）。
// 返回 {date: [{tmdb_id,title,air_date,...}]} 按日期分组；数据源 TMDB discover（作品级首播/上映）。
func (h *MediaHandler) Calendar(c *gin.Context) {
	kind := c.DefaultQuery("kind", "tv")
	if kind != "tv" && kind != "movie" {
		kind = "tv"
	}
	days := atoiDefault(c.Query("days"), 14)
	tm := h.tmdbSource()
	if tm == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	cal, err := tm.Calendar(c.Request.Context(), kind, days)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"kind": kind, "days": days, "calendar": cal})
}

// SubscriptionCalendar 订阅日历（GET /media/calendar/subscriptions?days=30）。
// 取当前全部订阅中的 tv TMDB id，返回未来 N 天单集开播日程（单集粒度，next_episode_to_air）。
func (h *MediaHandler) SubscriptionCalendar(c *gin.Context) {
	if h.subCh == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	tm := h.tmdbSource()
	if tm == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	days := atoiDefault(c.Query("days"), 30)
	subs, err := h.subCh.ListWithPosters(c.Request.Context())
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	var ids []string
	for _, s := range subs {
		if s.Type != "tv" || s.Source != "tmdb" {
			continue
		}
		// SourceID 两种形态：详情页订阅是 "tv:123"（带类型前缀，media 源惯例）；
		// Servarr/Trakt 同步创建的是纯数字 "123"（无前缀）。Cut 命中取后段、未命中回退
		// 整串，保证两类入口的订阅都进订阅日历（此前无前缀形态被静默跳过）。
		id := s.SourceID
		if _, tail, ok := strings.Cut(id, ":"); ok {
			id = tail
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"days": days, "calendar": map[string][]media.CalendarEpisode{}})
		return
	}
	cal, err := tm.CalendarForSubscriptions(c.Request.Context(), ids, days)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "calendar": cal})
}

// Search 跨源搜索媒体。「影视」端点默认只返回 movie+tv（排除音乐），
// 可用 ?type= 覆盖（""=全部 / movie / tv / music）。
func (h *MediaHandler) Search(c *gin.Context) {
	keyword := c.Query("keyword")
	year := atoiDefault(c.Query("year"), 0)
	if keyword == "" {
		errcode.Respond(c, errcode.MediaKeywordRequired)
		return
	}
	filter := c.DefaultQuery("type", "video")
	res, err := h.mediaCh.Search(c.Request.Context(), keyword, year, filter)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
	sum := h.recommend.RecommendFeedsStream(c.Request.Context(), source, category, page, force, func(items []media.MediaInfo) {
		emit("batch", gin.H{"results": items})
	})
	// done 帧带这一轮的账面（几个源、失败的是谁、有没有被源超时打掉）：
	// 只回 page 时「少一源」在页面上就是少几行卡片，没人看得出来。
	slog.Info("recommend stream done", "summary", sum.String(), "source", source, "category", category, "page", page)
	emit("done", gin.H{"page": page, "sources": sum.Sources, "batches": sum.Batches, "failed": sum.Failed, "cached": sum.Cached})
}

// Discover 跨源关键词搜索媒体（?keyword=&year=&source=&type=）。复用 MediaChain.Search。
// type 默认 video（movie+tv，排除音乐）；source 非空时再按来源过滤。空 keyword 报错。
func (h *MediaHandler) Discover(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		errcode.Respond(c, errcode.MediaKeywordRequired)
		return
	}
	year := atoiDefault(c.Query("year"), 0)
	filter := c.DefaultQuery("type", "video")
	res, err := h.mediaCh.Search(c.Request.Context(), keyword, year, filter)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
	// 空关键词不能进跨源匹配：会以空 query 扇出真实外网搜索（实测 8s 才回），
	// 并把结果按固定 cache key 落库被后续复用。
	if strings.TrimSpace(keyword) == "" {
		errcode.Respond(c, errcode.MediaKeywordRequired)
		return
	}
	year := atoiDefault(c.Query("year"), 0)
	url, err := h.mediaCh.MatchPoster(c.Request.Context(), keyword, year)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"poster_url": url})
}

type nfoReq struct {
	Info media.MediaInfo `json:"info"`
}

// maskSecret 脱敏：>8 字符显首尾各 4 位 + ****，否则全 ****；空串原样返回（前端据此判断未配置）。
// 与 KeepSeedHandler 同款风格，避免把 secret 明文回传前端。
// Detail 取媒体富详情（演职/推荐/时长/出品国等）。?source=tmdb|douban|bangumi&id=movie:123。
// 供「媒体详情页」用——点搜索结果卡片进详情，而非跳外站。
func (h *MediaHandler) Detail(c *gin.Context) {
	source := c.Query("source")
	id := c.Query("id")
	if source == "" || id == "" {
		errcode.Respond(c, errcode.MediaSourceIDRequired)
		return
	}
	res, err := h.mediaCh.Detail(c.Request.Context(), source, id)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// Celebrity 拉演员简介（点 cast 头像用）。
func (h *MediaHandler) Celebrity(c *gin.Context) {
	source := c.Query("source")
	id := c.Query("id")
	if source == "" || id == "" {
		errcode.Respond(c, errcode.MediaSourceIDRequired)
		return
	}
	res, err := h.mediaCh.Celebrity(c.Request.Context(), source, id)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// NFO 生成 NFO。
func (h *MediaHandler) NFO(c *gin.Context) {
	var req nfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"nfo": h.mediaCh.NFO(&req.Info)})
}

// GetQuality 返回多评分订阅质量配置（GET /media/quality）。
func (h *MediaHandler) GetQuality(c *gin.Context) {
	c.JSON(http.StatusOK, h.mediaCh.LoadQuality())
}

// respondMediaConfigWriteErr 媒体域配置写入失败的出口（#394）。
//
// 与 #387（字幕自动化配置）/ #392（辅种配置）同族，这一处是最纯粹的：单次写、
// 没有「部分生效」的坑，纯粹是**分类错了**——SaveQuality 的失败只可能是
// json.Marshal（结构体，实际不会）或 cfg.Set（存储写不进去：磁盘满/连接断/锁）。
//
// 旧形状 Internal.WithErr ⇒ 500「服务内部错误，请稍后重试」，而前端 MediaQuality.vue
// 的 catch 直接 `ElMessage.error(e?.message)` ⇒ 这句话原样弹给用户。他只会反复点保存，
// 而重试一万次也不会好（磁盘不会自己腾出空间）。
func respondMediaConfigWriteErr(c *gin.Context, err error) {
	errcode.Respond(c, errcode.SaveFailed.WithDetailKeepMessage(err))
}

// SetQuality 保存质量配置（PUT /media/quality）。整体覆盖；清空评分缓存即时生效。
func (h *MediaHandler) SetQuality(c *gin.Context) {
	var q chain.MediaQualityConfig
	if err := c.ShouldBindJSON(&q); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.mediaCh.SaveQuality(q); err != nil {
		// #394 存储写不进去是用户/运维能判断的失败，不该说成「服务内部错误」（见 helper 注释）。
		respondMediaConfigWriteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Rating 多源评分聚合预览（GET /media/rating?title=&year=&type=）。
// 供「媒体质量」页即时预览某作品的跨源综合评分 + 闸门判定。
func (h *MediaHandler) Rating(c *gin.Context) {
	title := strings.TrimSpace(c.Query("title"))
	if title == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	year := atoiDefault(c.Query("year"), 0)
	summary, err := h.mediaCh.AggregatedRating(c.Request.Context(), title, year, c.Query("type"))
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
	"hq.dyjzydj.com": "", // 短剧源封面 CDN（dj.hxpt.org 的 api/img 实际上游之一）
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
		errcode.Respond(c, errcode.UrlRequired)
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	referer, ok := mediaImageReferer(u.Host)
	if !ok {
		errcode.Respond(c, errcode.OperationForbidden)
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := mediaClient.Do(req)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	defer resp.Body.Close()
	ctype := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(ctype, "image/") {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("upstream %d %s", resp.StatusCode, ctype)})
		return
	}
	const maxPosterBytes = 5 << 20
	// 上游声明的体积超过代理上限：现在拒绝，而不是先发 200 再截一半。
	// 头一旦发出就无法改状态码，浏览器只会画一张坏海报且不告诉用户为什么——
	// 那种「图裂了」和「防盗链没配好 / 源站挂了」在界面上长得一样。
	if resp.ContentLength > maxPosterBytes {
		errcode.Respond(c, errcode.MediaImageTooLarge.WithErr(
			fmt.Errorf("上游声明 %d 字节，代理上限 %d 字节", resp.ContentLength, maxPosterBytes)))
		return
	}
	c.Header("Content-Type", ctype)
	c.Header("Cache-Control", "public, max-age=86400")
	httpclient.CopyBody(c.Writer, io.LimitReader(resp.Body, maxPosterBytes), "media image proxy",
		resp.ContentLength, "host", u.Host)
}
