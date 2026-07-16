package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	doubanAPIBase = "https://frodo.douban.com/api/v2"

	// —— 微信免签策略（零配置主路径）——
	// 公开 apiKey：豆瓣微信小程序凭据。frodo 对微信流量免 _sig 签名校验，
	// 因此只需 apiKey + 微信 UA + servicewechat Referer 即可调用（参考 douban-showcase 在用实现）。
	doubanDefaultKey = "0ac44ae016490db2204ce0a042db2916"
	doubanUA         = "Mozilla/5.0 (iPhone; CPU iPhone OS 15_3 like Mac OS X) AppleWebKit/605.1.15 " +
		"(KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.16(0x18001023) NetType/WIFI Language/zh_CN"
	doubanReferer = "https://servicewechat.com/wx2f9b06c1de1ccfca/84/page-frame.html"

	// —— 安卓 HMAC 签名策略（更稳，不依赖微信白名单）——
	// 公开 (apiKey, secret) + _ts/_sig。参考 MoviePilot app/modules/douban/apiv2.py，并已用
	// bestyize/DoubanAPI 真实抓包向量验证（见 frodoSign 注释）。DOUBAN_SECRET 非空时改以此为主策略。
	doubanSignedKey    = "0dad551ec0f84ed02907ff5c42e8ec70"
	doubanSignedSecret = "bf7dddc7c9cfe6f7"
	doubanSignedUA     = "api-client/1 com.douban.frodo/7.22.0.beta9(231) Android/23 product/Mate 40 " +
		"vendor/HUAWEI model/Mate 40 brand/HUAWEI  rom/android  network/wifi  platform/AndroidPad"
)

// Douban 是豆瓣元数据源（国产影视主源，弥补 TMDB 中文覆盖弱），走 frodo.douban.com JSON API。
//
// 两条鉴权策略，均带公开默认凭据、零配置可用：
//   - 微信免签（默认主策略）：微信小程序 apiKey + 微信 UA + servicewechat Referer，
//     frodo 对微信流量跳过 _sig 校验。
//   - 安卓 HMAC-SHA1 签名：安卓 App (apiKey, secret) + _ts/_sig，不依赖微信白名单，更稳。
//     DOUBAN_SECRET 非空时改以此为主策略；否则仅作 fallback。
//
// 任一策略命中鉴权类失败（401/403/带 sig|api_key 的 400）自动切另一策略重试一次，
// 故微信白名单收紧或签名 key 失效都能兜住（见 get/doRequest/isAuthLike）。
type Douban struct {
	apiKey string
	secret string // DOUBAN_SECRET：自定义签名密钥；空则用公开默认。非空同时把签名切为主策略
	base   string
	hc     *http.Client
}

// doubanStringList accepts both legacy arrays and current Frodo scalar strings.
// For example, pubdate changed from ["2026-07-03"] to "2026-07-03(中国大陆)".
type doubanStringList []string

func (v *doubanStringList) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*v = list
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value == "" {
		*v = nil
		return nil
	}
	*v = []string{value}
	return nil
}

type doubanYear int

func (v *doubanYear) UnmarshalJSON(data []byte) error {
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		*v = doubanYear(number)
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*v = doubanYear(yearFromDate(value))
	return nil
}

type doubanDetailPerson struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Character string `json:"character"`
	Avatar    string `json:"avatar"`
	Avatars   struct {
		Large string `json:"large"`
	} `json:"avatars"`
}

// NewDouban 构造。appKey 为空用公开微信默认 apiKey；secret 为空时签名策略用公开默认 secret
// （均已验证可用）。设 DOUBAN_SECRET 即启用「签名为主、微信免签兜底」的稳健模式。
func NewDouban(secret, appKey string) *Douban {
	if appKey == "" {
		appKey = doubanDefaultKey
	}
	return &Douban{apiKey: appKey, secret: secret, base: doubanAPIBase, hc: &http.Client{Timeout: 10 * time.Second}}
}

func (d *Douban) Name() string { return "douban" }

// Types 实现 TypedSource：豆瓣只产 movie/tv（book/music 已在 doubanType 过滤）。
func (d *Douban) Types() []MediaType { return []MediaType{TypeMovie, TypeTV} }

// Categories 实现 CategoryCapable：豆瓣 Trending 支持热映/即将上映/剧集。
func (d *Douban) Categories() []string { return []string{"movie", "coming", "tv"} }

// get 发 GET 并解码到 out。鉴权双策略 + 自动 fallback：
//   - 主策略：secret 配了(DOUBAN_SECRET)走「安卓 HMAC-SHA1 签名」；否则走「微信免签」(零配置默认)。
//   - 任一策略命中鉴权类失败(401/403/带 sig|api_key 的 400)，自动切另一策略重试一次。
//
// 两条策略各自带公开默认凭据(微信 apiKey / 安卓 key+secret)，故零配置下签名 fallback 也可用。
func (d *Douban) get(ctx context.Context, path string, params map[string]string, out any) error {
	signed := d.secret != "" // DOUBAN_SECRET 非空 → 以签名策略为主
	if err := d.doRequest(ctx, signed, path, params, out); err == nil || !isAuthLike(err) {
		return err
	}
	return d.doRequest(ctx, !signed, path, params, out) // 鉴权失败 → 换另一策略重试
}

// doRequest 按策略构造并发起一次请求。
// signed=true 注入 _ts/_sig（对实际请求 path 签名，含 /api/v2 前缀，与 MoviePilot 一致）+
// os_rom=android + 安卓 UA；否则注入微信 UA + servicewechat Referer 免签。
func (d *Douban) doRequest(ctx context.Context, signed bool, path string, params map[string]string, out any) error {
	u, err := url.Parse(d.base + path)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("apiKey", d.keyFor(signed))
	for k, v := range params {
		q.Set(k, v)
	}
	ua, referer := doubanUA, doubanReferer
	if signed {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		q.Set("os_rom", "android")
		q.Set("_ts", ts)
		q.Set("_sig", frodoSign(http.MethodGet, u.Path, ts, d.signedSecret()))
		ua, referer = doubanSignedUA, "" // 安卓 App 流量，无 servicewechat Referer
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ua)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := d.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return errDoubanRateLimited // 限速，非鉴权，不触发 fallback
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512)) // 取片段供 isAuthLike 判定
		return &doubanStatusError{path: path, status: resp.StatusCode, body: string(body)}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// keyFor 返回该策略用的 apiKey。微信策略用 d.apiKey（NewDouban 已兜底默认）；
// 签名策略下若用户没显式覆盖（仍是微信默认 key）则换成配对的安卓 key。
func (d *Douban) keyFor(signed bool) string {
	if !signed {
		return d.apiKey
	}
	if d.apiKey != "" && d.apiKey != doubanDefaultKey {
		return d.apiKey // 用户显式覆盖
	}
	return doubanSignedKey
}

// signedSecret 返回签名密钥：用户配了 DOUBAN_SECRET 用之，否则用公开默认（已验证）。
func (d *Douban) signedSecret() string {
	if d.secret != "" {
		return d.secret
	}
	return doubanSignedSecret
}

// doubanStatusError 携带非 2xx 状态码与响应体片段，供 isAuthLike 判断是否鉴权类失败。
type doubanStatusError struct {
	path   string
	status int
	body   string
}

func (e *doubanStatusError) Error() string {
	return fmt.Sprintf("douban %s: status %d", e.path, e.status)
}

var errDoubanRateLimited = errors.New("douban rate limited")

// isAuthLike 报告该错误是否像签名/凭据失败（可换策略重试）；404/429/网络错等不算。
func isAuthLike(err error) bool {
	var se *doubanStatusError
	if !errors.As(err, &se) {
		return false
	}
	switch se.status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return true
	case http.StatusBadRequest:
		b := strings.ToLower(se.body)
		return strings.Contains(b, "sig") || strings.Contains(b, "api_key") ||
			strings.Contains(b, "apikey") || strings.Contains(b, "signature")
	}
	return false
}

// frodoSign 计算豆瓣 frodo 的 _sig：base64(HMAC-SHA1(secret, "METHOD&<pctpath>&<ts>"))。
// <pctpath> 是请求 path 的百分号编码（含 /，等价 Python parse.quote(path, safe="")）。
//
// 已用 bestyize/DoubanAPI 真实抓包向量验证：
//
//	GET /api/v2/elessar/subject/27260217/photos, _ts=1599316450, secret=bf7dddc7c9cfe6f7
//	→ gfTX2YbSiADYaG/zXJ+pNo5IhbI=
func frodoSign(method, path, ts, secret string) string {
	raw := strings.ToUpper(method) + "&" + pctEncode(path) + "&" + ts
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(raw))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// pctEncode 按 RFC3986 unreserved 集合（A-Za-z0-9-._~）外全部 %XX 编码，等价 Python
// urllib.parse.quote(s, safe="")——逐字节处理，非 ASCII 字节各自编码。
func pctEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s) * 3)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0F])
	}
	return b.String()
}

// frodo /api/v2/search 响应。
//
// 2024 起 frodo 改版：结果从顶层 items 移到 subjects.items，类型字段从 type 改成
// target_type（仍在 item 层，不在 target 里）。target 内字段不变。这里两种结构都收，
// 旧客户端/旧响应也能解析，避免再次改版又静默返回 0 条。
type doubanSearchItem struct {
	Type       string `json:"target_type"` // 新 frodo：类型在 item 层
	LegacyType string `json:"type"`        // 旧 frodo 兼容
	Target     struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		CardSubtitle string `json:"card_subtitle"`
		CoverURL     string `json:"cover_url"`
		Year         string `json:"year"`
		Rating       struct {
			Value float64 `json:"value"`
			Count int     `json:"count"`
		} `json:"rating"`
	} `json:"target"`
}

func (it doubanSearchItem) typeStr() string {
	if it.Type != "" {
		return it.Type
	}
	return it.LegacyType
}

type doubanSearchResp struct {
	Subjects *struct {
		Items []doubanSearchItem `json:"items"`
	} `json:"subjects"`
	LegacyItems []doubanSearchItem `json:"items"`
}

// Search 调 /api/v2/search，返回影视（movie/tv）结果。id 形如 "movie:1292052"。
func (d *Douban) Search(ctx context.Context, keyword string, year int) ([]MediaInfo, error) {
	var r doubanSearchResp
	if err := d.get(ctx, "/search", map[string]string{"q": keyword, "count": "20", "start": "0"}, &r); err != nil {
		return nil, err
	}
	items := r.LegacyItems
	if r.Subjects != nil && len(r.Subjects.Items) > 0 {
		items = r.Subjects.Items
	}
	out := make([]MediaInfo, 0, len(items))
	for _, it := range items {
		mi, ok := doubanItemToMedia(it)
		if !ok {
			continue
		}
		if year > 0 && mi.Year > 0 && abs(mi.Year-year) > 1 {
			continue // 年份容差 ±1
		}
		out = append(out, mi)
	}
	return out, nil
}

// doubanItemToMedia 把一条 frodo 搜索/热门 item 映射到 MediaInfo。仅取影视（movie/tv）。
func doubanItemToMedia(it doubanSearchItem) (MediaInfo, bool) {
	mt := doubanType(it.typeStr())
	if mt == "" {
		return MediaInfo{}, false
	}
	t := it.Target
	mi := MediaInfo{
		Source: "douban", SourceID: string(mt) + ":" + t.ID,
		Title: t.Title, Type: mt, PosterURL: doubanPosterURL(t.CoverURL),
		Overview: t.CardSubtitle, Rating: t.Rating.Value, VoteCount: t.Rating.Count, ReleaseDate: t.Year,
	}
	mi.Year = yearFromDate(t.Year)
	return mi, true
}

// Trending 实现 TrendingCapable：豆瓣 frodo 趣味影集/热门榜。
//
// 2026 起 frodo 热门端点从 /movie/hot 等迁到 /subject_collection/<slug>/items（豆瓣 App 自用，
// 旧 /movie/hot 已 404）。响应字段扁平在 item 上（type/id/title/year/rating{value}/cover{url}/
// card_subtitle），与 /search 的 target 嵌套不同，故用独立结构 doubanCollectionResp 解析。
//
//	movie  → /subject_collection/movie_showing/items（正在热映）
//	coming → /subject_collection/movie_soon/items（即将上映）
//	tv     → /subject_collection/tv_hot/items（剧集热门）
//
// 类型按请求 category 决定（端点本身已按品类 scoped，不再按 item.type 过滤）。
func (d *Douban) Trending(ctx context.Context, category string, page int) ([]MediaInfo, error) {
	if page < 1 {
		page = 1
	}
	mt := TypeMovie
	path := "/subject_collection/movie_showing/items"
	switch category {
	case "coming":
		path = "/subject_collection/movie_soon/items"
	case "tv":
		path = "/subject_collection/tv_hot/items"
		mt = TypeTV
	}
	var r doubanCollectionResp
	if err := d.get(ctx, path, map[string]string{"start": strconv.Itoa((page - 1) * 20), "count": "20"}, &r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.Items))
	for _, it := range r.Items {
		if it.ID == "" || it.Title == "" {
			continue
		}
		mi := MediaInfo{
			Source: "douban", SourceID: string(mt) + ":" + it.ID,
			Title: it.Title, OriginalTitle: it.OriginalTitle, Type: mt,
			Overview: it.CardSubtitle, Rating: it.Rating.Value, VoteCount: it.Rating.Count,
			ReleaseDate: it.Year, PosterURL: doubanPosterURL(it.Cover.URL),
		}
		mi.Year = yearFromDate(it.Year)
		out = append(out, mi)
	}
	return out, nil
}

// doubanCollectionResp 是 /subject_collection/<slug>/items 响应：item 字段扁平
// （区别于 /search 的 target 嵌套）。cover 为 {url} 对象、rating 为 {value}，均可空。
type doubanCollectionResp struct {
	Total int                    `json:"total"`
	Items []doubanCollectionItem `json:"subject_collection_items"`
}

type doubanCollectionItem struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	CardSubtitle  string `json:"card_subtitle"`
	Year          string `json:"year"`
	Rating        struct {
		Value float64 `json:"value"`
		Count int     `json:"count"`
	} `json:"rating"`
	Cover struct {
		URL string `json:"url"`
	} `json:"cover"`
}

// frodo /api/v2/{type}/{id} 详情。填充演职(导演/主演)/出品国/时长/类型等富字段。
func (d *Douban) Detail(ctx context.Context, id string) (*MediaInfo, error) {
	mtype, sid, ok := splitTypeID(id)
	if !ok {
		return nil, fmt.Errorf("douban: bad id %q", id)
	}
	var raw struct {
		Title         string     `json:"title"`
		OriginalTitle string     `json:"original_title"`
		Year          doubanYear `json:"year"`
		Summary       string     `json:"summary"`
		Intro         string     `json:"intro"`
		Rating        struct {
			Value float64 `json:"value"`
			Count int     `json:"count"`
		} `json:"rating"`
		Images struct {
			Large string `json:"large"`
		} `json:"images"`
		Pic struct {
			Large string `json:"large"`
		} `json:"pic"`
		Genres    []string         `json:"genres"`
		PubDate   doubanStringList `json:"pubdate"`
		Countries []string         `json:"countries"`
		Durations []string         `json:"durations"` // 如 "110分钟"
		Directors []struct {
			Name string `json:"name"`
		} `json:"directors"`
		Casts  []doubanDetailPerson `json:"casts"`  // legacy Frodo response
		Actors []doubanDetailPerson `json:"actors"` // current Frodo response
		AKA    []string             `json:"aka"`    // 又名（跨语言标题匹配用）
	}
	if err := d.get(ctx, "/"+mtype+"/"+sid, nil, &raw); err != nil {
		return nil, err
	}
	mt := TypeMovie
	if mtype == "tv" {
		mt = TypeTV
	}
	date := ""
	if len(raw.PubDate) > 0 {
		date = raw.PubDate[0]
	}
	posterURL := raw.Images.Large
	if posterURL == "" {
		posterURL = raw.Pic.Large
	}
	overview := raw.Intro
	if overview == "" {
		overview = raw.Summary
	}
	mi := &MediaInfo{
		Source: "douban", SourceID: id, Title: raw.Title,
		OriginalTitle: raw.OriginalTitle, Type: mt, Overview: overview,
		Rating: raw.Rating.Value, VoteCount: raw.Rating.Count, PosterURL: doubanPosterURL(posterURL),
		ReleaseDate: date, Year: yearFromDate(date), Genres: raw.Genres,
		Countries: raw.Countries, Runtime: parseMinutes(raw.Durations),
		Names: raw.AKA,
	}
	if mi.Year == 0 {
		mi.Year = int(raw.Year)
	}
	for _, dr := range raw.Directors {
		mi.Directors = append(mi.Directors, dr.Name)
	}
	// detail 端点的 actors 只有 name 无头像，appendCast 作 celebrities 失败时的 fallback。
	seenCast := make(map[string]struct{})
	appendCast := func(c doubanDetailPerson) {
		if c.Name == "" {
			return
		}
		if _, ok := seenCast[c.Name]; ok {
			return
		}
		seenCast[c.Name] = struct{}{}
		avatar := c.Avatar
		if avatar == "" {
			avatar = c.Avatars.Large
		}
		role := c.Character
		if role == "" {
			role = c.Role
		}
		mi.Cast = append(mi.Cast, CreditPerson{Name: c.Name, Role: role, Avatar: doubanPosterURL(avatar)})
	}

	// 三个增强子请求并行、尽力而为（任一失败降级，不阻塞主结果）。detail 端点的 actors
	// 只有 name 无 avatar、无 recommendations、cover 字段是用户相册封面（非横幅），
	// 故分别向 celebrities / recommendations / photos 取带头像演员 / 推荐 / 横幅剧照。
	var (
		wg  sync.WaitGroup
		cel struct {
			Actors []struct {
				Name      string `json:"name"`
				Character string `json:"character"`
				URI       string `json:"uri"` // 含 subject_id（personage id），用于点头像拉演员简介
				Avatar    struct {
					Large string `json:"large"`
				} `json:"avatar"`
			} `json:"actors"`
		}
		recs []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Year         string `json:"year"`
			CardSubtitle string `json:"card_subtitle"`
			Pic          struct {
				Large string `json:"large"`
			} `json:"pic"`
			Rating struct {
				Value float64 `json:"value"`
			} `json:"rating"`
		}
		photos struct {
			Photos []struct {
				Image struct {
					Large struct {
						URL    string `json:"url"`
						Width  int    `json:"width"`
						Height int    `json:"height"`
					} `json:"large"`
				} `json:"image"`
			} `json:"photos"`
		}
		haveCel bool
	)
	wg.Add(3)
	go func() { defer wg.Done(); haveCel = d.get(ctx, "/"+mtype+"/"+sid+"/celebrities", nil, &cel) == nil }()
	go func() {
		defer wg.Done()
		_ = d.get(ctx, "/"+mtype+"/"+sid+"/recommendations", map[string]string{"start": "0", "count": "20"}, &recs)
	}()
	go func() {
		defer wg.Done()
		_ = d.get(ctx, "/"+mtype+"/"+sid+"/photos", map[string]string{"start": "0", "count": "10"}, &photos)
	}()
	wg.Wait()

	// 演员：celebrities 优先（带头像）；失败回退 detail 裸 actors（无头像）。
	if haveCel && len(cel.Actors) > 0 {
		seen := make(map[string]struct{}, len(cel.Actors))
		for _, a := range cel.Actors {
			if a.Name == "" {
				continue
			}
			if _, ok := seen[a.Name]; ok {
				continue
			}
			seen[a.Name] = struct{}{}
			// celebrity 头像原样返回：personage/m/public/pXXXX.jpg 若过 doubanPosterURL 会被
			// (p\d+)\.jpg 误重写成不存在的海报路径 → 404。故不重写，交前端 /media/image 反代取真图。
			mi.Cast = append(mi.Cast, CreditPerson{Name: a.Name, Role: a.Character, Avatar: a.Avatar.Large, ID: doubanSubjectID(a.URI)})
		}
	} else {
		for _, c := range raw.Casts {
			appendCast(c)
		}
		for _, c := range raw.Actors {
			appendCast(c)
		}
	}

	// 推荐：裸数组映射（镜像 tmdb.go 的 Recommendations 映射）。
	for _, r := range recs {
		if r.ID == "" {
			continue
		}
		y := yearFromDate(r.Year)
		if y == 0 && r.CardSubtitle != "" {
			// card_subtitle 形如 "2026 / 美国 / 喜剧 ..."，取首个 / 前的年份
			if i := strings.IndexByte(r.CardSubtitle, '/'); i > 0 {
				y = yearFromDate(strings.TrimSpace(r.CardSubtitle[:i]))
			}
		}
		mi.Recommendations = append(mi.Recommendations, MediaInfo{
			Source: "douban", SourceID: mtype + ":" + r.ID,
			Title: r.Title, Type: mt, PosterURL: doubanPosterURL(r.Pic.Large),
			Rating: r.Rating.Value, Year: y,
		})
	}

	// 背景：取剧照里第一张横幅（width>height）；URL 原样返回（photo 走前端反代）。
	for _, ph := range photos.Photos {
		if ph.Image.Large.URL != "" && ph.Image.Large.Width > ph.Image.Large.Height {
			mi.BackdropURL = ph.Image.Large.URL
			break
		}
	}

	return mi, nil
}

// parseMinutes 从 durations（如 "110分钟" / "110 min"）解析分钟数；取第一条有效值。
func parseMinutes(ds []string) int {
	for _, d := range ds {
		n := 0
		for _, r := range d {
			if r >= '0' && r <= '9' {
				n = n*10 + int(r-'0')
			} else if n > 0 {
				return n
			}
		}
		if n > 0 {
			return n
		}
	}
	return 0
}

// doubanSubjectIDRe 匹配 celebrities actor.uri 里的 personage subject_id。
var doubanSubjectIDRe = regexp.MustCompile(`subject_id=(\d+)`)

// doubanSubjectID 从 celebrities actor 的 uri（douban://douban.com/celebrity/{cid}?subject_id={pid}）
// 提取 personage subject_id，用于调 /elessar/subject 拉演员简介。无则空。
func doubanSubjectID(uri string) string {
	if m := doubanSubjectIDRe.FindStringSubmatch(uri); m != nil {
		return m[1]
	}
	return ""
}

// Celebrity 拉演员简介（点头像用）：调 /elessar/subject/{personageID}，返回生平/结构化/作品。
// desc 是 HTML 多段 <p>，用 goquery 提纯文本段落（避免前端 v-html XSS）。
func (d *Douban) Celebrity(ctx context.Context, personageID string) (*CelebrityProfile, error) {
	var raw struct {
		Name      string `json:"name"`
		LatinName string `json:"latin_title"`
		Cover     struct {
			Large struct {
				URL string `json:"url"`
			} `json:"large"`
		} `json:"cover"`
		FollowedCount int    `json:"followed_count"`
		Desc          string `json:"desc"`
		URL           string `json:"url"`
		Extra         struct {
			Info      [][2]string `json:"info"`
			ShortInfo string      `json:"short_info"`
		} `json:"extra"`
		Modules []struct {
			Type    string `json:"type"`
			Payload struct {
				Collections []struct {
					Works []struct {
						Roles   []string `json:"roles"`
						Subject struct {
							Title string `json:"title"`
							URL   string `json:"url"`
						} `json:"subject"`
					} `json:"works"`
				} `json:"collections"`
			} `json:"payload"`
		} `json:"modules"`
	}
	if err := d.get(ctx, "/elessar/subject/"+personageID, nil, &raw); err != nil {
		return nil, err
	}
	p := &CelebrityProfile{
		Name: raw.Name, LatinName: raw.LatinName, Avatar: raw.Cover.Large.URL,
		ShortInfo: raw.Extra.ShortInfo, Info: raw.Extra.Info,
		FollowedCount: raw.FollowedCount, DoubanURL: raw.URL,
	}
	// desc 是 HTML 多段 <p>，提纯文本段落；avatar 原样（personage 路径不过 doubanPosterURL）。
	if raw.Desc != "" {
		if doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw.Desc)); err == nil {
			doc.Find("p").Each(func(_ int, s *goquery.Selection) {
				if t := strings.TrimSpace(s.Text()); t != "" {
					p.Bio = append(p.Bio, t)
				}
			})
			if len(p.Bio) == 0 { // 无 <p> 标签时取整体文本兜底
				if t := strings.TrimSpace(doc.Text()); t != "" {
					p.Bio = []string{t}
				}
			}
		}
	}
	// 参演作品：work_collections 下各 collection 的 works（最多 20 个，跳过无标题）。
	for _, m := range raw.Modules {
		if m.Type != "work_collections" {
			continue
		}
		for _, col := range m.Payload.Collections {
			for _, w := range col.Works {
				if w.Subject.Title == "" || len(p.Works) >= 20 {
					break
				}
				p.Works = append(p.Works, CelebrityWork{
					Title: w.Subject.Title, Role: strings.Join(w.Roles, " / "), URL: w.Subject.URL,
				})
			}
		}
		break
	}
	return p, nil
}

// doubanPhotoFile 匹配豆瓣 photo 文件名（p 开头 + 数字）。
var doubanPhotoFile = regexp.MustCompile(`(p\d+)\.jpg`)

// doubanPosterURL 把豆瓣 cover_url 规整成稳定的竖版海报地址。
//
// frodo 返回的 cover_url 多为 qnmob3 的 large 横裁图（带 imageView2 .../h/120），
// 且防盗链下无 Referer 直连只回 1x1 占位图。改成 img9 的 m_ratio_poster（2:3 海报），
// 再由前端经 /media/image 反代（带 movie.douban.com Referer）取真图。
// 非 photo 封面（如 subject 的 s*.jpg）无法换算，原样返回。
func doubanPosterURL(cover string) string {
	if cover == "" {
		return ""
	}
	m := doubanPhotoFile.FindStringSubmatch(cover)
	if m == nil {
		return cover
	}
	return "https://img9.doubanio.com/view/photo/m_ratio_poster/public/" + m[1] + ".jpg"
}

// doubanType 把 frodo 的 type 映射到 MediaType（只取影视，过滤 book/music 等）。
func doubanType(t string) MediaType {
	switch strings.ToLower(t) {
	case "movie":
		return TypeMovie
	case "tv":
		return TypeTV
	}
	return ""
}
