package media

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
)

// Trending 实现 TrendingCapable：豆瓣 frodo 趣味影集/热门榜 + 榜单聚合。
//
// 2026 起 frodo 热门端点从 /movie/hot 等迁到 /subject_collection/<slug>/items（豆瓣 App 自用，
// 旧 /movie/hot 已 404）。响应字段扁平在 item 上（type/id/title/year/rating{value}/cover{url}/
// card_subtitle），与 /search 的 target 嵌套不同，故用独立结构 doubanCollectionResp 解析。
//
//	movie   → /subject_collection/movie_showing/items（正在热映）
//	coming  → /subject_collection/movie_soon/items（即将上映）
//	tv      → /subject_collection/tv_hot/items（剧集热门）
//	topMovie → /subject_collection/movie_top250/items（电影 Top250，榜单聚合）
//	topTv    → 网页版 /j/search_subjects 高分剧集聚合（frodo 无剧集 Top250 榜单端点，实测 404）
//	weekly  → /subject_collection/movie_weekly_best/items（一周口碑榜，榜单聚合）
//
// 类型按请求 category 决定（端点本身已按品类 scoped，不再按 item.type 过滤）。
//
// 封面解析：2026 起 frodo 榜单端点的封面字段是 pic{large,normal}（cover 对象已不存在，
// 实测 topMovie/weekly 均如此），movie_showing 这类仍回 cover.url，故两个字段都解析；
// pic.large 已是 m_ratio_poster 竖版海报，doubanPosterURL 会原样保留。
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
	case "topMovie":
		path = "/subject_collection/movie_top250/items"
	case "topTv":
		return d.trendingTopTv(ctx, page)
	case "weekly":
		path = "/subject_collection/movie_weekly_best/items"
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
		poster := it.Cover.URL
		if poster == "" {
			poster = it.Pic.Large
		}
		mi := MediaInfo{
			Source: "douban", SourceID: string(mt) + ":" + it.ID,
			Title: it.Title, OriginalTitle: it.OriginalTitle, Type: mt,
			Overview: it.CardSubtitle, Rating: it.Rating.Value, VoteCount: it.Rating.Count,
			ReleaseDate: it.Year, PosterURL: doubanPosterURL(poster),
		}
		mi.Year = yearFromDate(it.Year)
		out = append(out, mi)
	}
	return out, nil
}

// trendingTopTv 抓网页版 search_subjects 排行近似「剧集 Top250」：豆瓣 frodo 无对应榜单
// 端点（tv_top250 等 slug 实测 404），而网页版接口公开无签名，tag=热门 全量翻页后过滤
// rate≥8.5 按 rate 排序取前 20——与「高分经典剧集」语义一致。每页 50 条、最多取 5 页，
// 单页失败跳过；全部失败才返回错误（由 RecommendChain 标记源不可用）。
//
// 该接口每条只有 id/title/rate/cover——缺年份/类型/产地，卡片比其他推荐源少一整行
// 字段（对标电影 Top250 的 card_subtitle）。故入围前 20 后并发调 frodo Detail 富化
// （带 6s 条目超时 + 整体上限，单条失败保留裸数据不拖垮整页）。
func (d *Douban) trendingTopTv(ctx context.Context, page int) ([]MediaInfo, error) {
	const perPage, maxPages = 50, 5
	type webItem struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Rate  string `json:"rate"`
		Cover string `json:"cover"`
	}
	var all []webItem
	for p := 0; p < maxPages; p++ {
		var r struct {
			Subjects []webItem `json:"subjects"`
		}
		q := map[string]string{
			"type": "tv", "tag": "热门", "sort": "recommend",
			"page_limit": strconv.Itoa(perPage), "page_start": strconv.Itoa(p * perPage),
		}
		if err := d.getWeb(ctx, "/j/search_subjects", q, &r); err != nil {
			break // 豆瓣对高频翻页限流，拿多少算多少
		}
		all = append(all, r.Subjects...)
		if len(r.Subjects) < perPage {
			break
		}
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("douban: search_subjects tv empty")
	}
	type rated struct {
		it   webItem
		rate float64
	}
	var rs []rated
	for _, it := range all {
		if it.ID == "" || it.Title == "" || it.Rate == "" {
			continue
		}
		v, err := strconv.ParseFloat(it.Rate, 64)
		if err != nil || v < 8.5 {
			continue
		}
		rs = append(rs, rated{it, v})
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].rate > rs[j].rate })
	start := (page - 1) * 20
	if start >= len(rs) {
		return []MediaInfo{}, nil
	}
	rs = rs[start:]
	if len(rs) > 20 {
		rs = rs[:20]
	}
	// 并发 Detail 富化（限流 5 路）：单条 6s 超时；失败则该条仅少富字段，仍上榜。
	out := make([]MediaInfo, len(rs))
	wg := sync.WaitGroup{}
	sem := make(chan struct{}, 5)
	for i := range rs {
		out[i] = MediaInfo{
			Source: "douban", SourceID: string(TypeTV) + ":" + rs[i].it.ID,
			Title: rs[i].it.Title, Type: TypeTV,
			Rating: rs[i].rate, PosterURL: doubanPosterURL(rs[i].it.Cover),
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			mctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			if mi, err := d.Detail(mctx, out[i].SourceID); err == nil && mi != nil {
				keepRating, keepPoster := out[i].Rating, out[i].PosterURL // 榜单值为准（Detail 未回评分/海报差异）
				if mi.Rating == 0 {
					mi.Rating = keepRating
				}
				if mi.PosterURL == "" {
					mi.PosterURL = keepPoster
				}
				out[i] = *mi
			}
		}(i)
	}
	wg.Wait()
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
	Pic struct {
		Large  string `json:"large"`
		Normal string `json:"normal"`
	} `json:"pic"`
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
	safego.Go("douban:542", func() { defer wg.Done(); haveCel = d.get(ctx, "/"+mtype+"/"+sid+"/celebrities", nil, &cel) == nil })
	safego.Go("douban:543", func() {
		defer wg.Done()
		_ = d.get(ctx, "/"+mtype+"/"+sid+"/recommendations", map[string]string{"start": "0", "count": "20"}, &recs)
	})
	safego.Go("douban:547", func() {
		defer wg.Done()
		_ = d.get(ctx, "/"+mtype+"/"+sid+"/photos", map[string]string{"start": "0", "count": "10"}, &photos)
	})
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

// Celebrity 拉演员简介（点头像用）：调 /elessar/subject/{personageID}，返回生平/结构化/作品。
// desc 是 HTML 多段 <p>，用 goquery 提纯文本段落（避免前端 v-html XSS）。
