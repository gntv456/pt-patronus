package media


import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/hxpt/ptpatronus/internal/core/safego"
)


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
