// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	tmdbImgBase = "https://image.tmdb.org/t/p/w500"
)

// TMDB 是 The Movie DB（影视主源）客户端。
type TMDB struct {
	apiKey string
	base   string // https://<domain>/3，domain 可配镜像（TMDB_API_DOMAIN）
	hc     *http.Client
}

// NewTMDB 构造；apiKey 为空时 Search 返回错误（由 Chain 跳过）。
// domain 为空或 "api.themoviedb.org" 用官方域名；国内被墙可经 TMDB_API_DOMAIN 换镜像。
func NewTMDB(apiKey, domain string) *TMDB {
	if domain == "" {
		domain = "api.themoviedb.org"
	}
	return &TMDB{apiKey: apiKey, base: "https://" + domain + "/3", hc: &http.Client{Timeout: 15 * time.Second}}
}

// Name 实现 MediaSource。
func (t *TMDB) Name() string { return "tmdb" }

// Types 实现 TypedSource：TMDB 只产 movie/tv。
func (t *TMDB) Types() []MediaType { return []MediaType{TypeMovie, TypeTV} }

// Categories 实现 CategoryCapable：TMDB Trending 支持热映/即将/剧集/动漫/流行。
func (t *TMDB) Categories() []string { return []string{"movie", "coming", "tv", "anime", "popular"} }

func (t *TMDB) get(ctx context.Context, path string, params map[string]string, out any) error {
	if t.apiKey == "" {
		return ErrNotConfigured
	}
	u, err := url.Parse(t.base + path)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("api_key", t.apiKey)
	q.Set("language", "zh-CN")
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := t.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type tmdbMultiResult struct {
	Results []struct {
		ID         int64   `json:"id"`
		MediaType  string  `json:"media_type"`
		Title      string  `json:"title"`
		Name       string  `json:"name"`
		PosterPath string  `json:"poster_path"`
		Overview   string  `json:"overview"`
		Release    string  `json:"release_date"`
		FirstAir   string  `json:"first_air_date"`
		Vote       float64 `json:"vote_average"`
		VoteCount  int64   `json:"vote_count"`
	} `json:"results"`
}

// Search 用 /search/multi 搜索影视。
func (t *TMDB) Search(ctx context.Context, keyword string, year int) ([]MediaInfo, error) {
	var r tmdbMultiResult
	if err := t.get(ctx, "/search/multi", map[string]string{"query": keyword, "page": "1"}, &r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.Results))
	for _, it := range r.Results {
		if it.MediaType != "movie" && it.MediaType != "tv" {
			continue
		}
		title := it.Title
		if title == "" {
			title = it.Name
		}
		date := it.Release
		mt := TypeMovie
		if it.MediaType == "tv" {
			date = it.FirstAir
			mt = TypeTV
		}
		mi := MediaInfo{
			Source: "tmdb", SourceID: string(mt) + ":" + strconv.FormatInt(it.ID, 10),
			Title: title, Type: mt, Overview: it.Overview,
			ReleaseDate: date, Rating: it.Vote, VoteCount: int(it.VoteCount),
		}
		mi.Year = yearFromDate(date)
		mi.PosterURL = tmdbImage(it.PosterPath)
		if year > 0 && mi.Year > 0 && abs(mi.Year-year) > 1 {
			continue // 年份容差 ±1
		}
		out = append(out, mi)
	}
	return out, nil
}

// Detail 取影视详情（id 形如 "movie:123" 或 "tv:456"）。一次请求带 append_to_response
// 拿 credits/recommendations，填充演职/推荐/时长/出品国/背景图。
func (t *TMDB) Detail(ctx context.Context, id string) (*MediaInfo, error) {
	mtype, sid, ok := splitTypeID(id)
	if !ok {
		return nil, fmt.Errorf("tmdb: bad id %q", id)
	}
	var raw struct {
		ID           int64   `json:"id"`
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		PosterPath   string  `json:"poster_path"`
		BackdropPath string  `json:"backdrop_path"`
		Overview     string  `json:"overview"`
		Release      string  `json:"release_date"`
		FirstAir     string  `json:"first_air_date"`
		Runtime      int     `json:"runtime"`
		Vote         float64 `json:"vote_average"`
		VoteCount    int64   `json:"vote_count"`
		Genres       []struct {
			Name string `json:"name"`
		} `json:"genres"`
		ProdCountries []struct {
			Name string `json:"name"`
		} `json:"production_countries"`
		Credits struct {
			Crew []struct {
				Job  string `json:"job"`
				Name string `json:"name"`
			} `json:"crew"`
			Cast []struct {
				Name        string `json:"name"`
				Character   string `json:"character"`
				ProfilePath string `json:"profile_path"`
			} `json:"cast"`
		} `json:"credits"`
		Recommendations struct {
			Results []struct {
				ID         int64   `json:"id"`
				Title      string  `json:"title"`
				Name       string  `json:"name"`
				PosterPath string  `json:"poster_path"`
				Release    string  `json:"release_date"`
				FirstAir   string  `json:"first_air_date"`
				Vote       float64 `json:"vote_average"`
				Overview   string  `json:"overview"`
			} `json:"results"`
		} `json:"recommendations"`
		AlternativeTitles struct {
			Results []struct {
				Title string `json:"title"`
			} `json:"results"`
		} `json:"alternative_titles"`
		Translations struct {
			Translations []struct {
				Data struct {
					Title string `json:"title"` // movie
					Name  string `json:"name"`  // tv
				} `json:"data"`
			} `json:"translations"`
		} `json:"translations"`
	}
	if err := t.get(ctx, "/"+mtype+"/"+sid, map[string]string{"append_to_response": "credits,recommendations,alternative_titles,translations"}, &raw); err != nil {
		return nil, err
	}
	title := raw.Title
	if title == "" {
		title = raw.Name
	}
	date := raw.Release
	mt := TypeMovie
	if mtype == "tv" {
		date = raw.FirstAir
		mt = TypeTV
	}
	mi := &MediaInfo{
		Source: "tmdb", SourceID: id, Title: title, Type: mt,
		Overview: raw.Overview, ReleaseDate: date, Rating: raw.Vote, VoteCount: int(raw.VoteCount),
		PosterURL: tmdbImage(raw.PosterPath), BackdropURL: tmdbImage(raw.BackdropPath),
		Year: yearFromDate(date), Runtime: raw.Runtime,
	}
	// 别名/译名：TMDB alternative_titles + translations 各语种标题，供跨语言标题匹配。
	for _, at := range raw.AlternativeTitles.Results {
		if at.Title != "" {
			mi.Names = append(mi.Names, at.Title)
		}
	}
	for _, tr := range raw.Translations.Translations {
		t := tr.Data.Title
		if t == "" {
			t = tr.Data.Name
		}
		if t != "" {
			mi.Names = append(mi.Names, t)
		}
	}
	for _, g := range raw.Genres {
		mi.Genres = append(mi.Genres, g.Name)
	}
	for _, c := range raw.ProdCountries {
		mi.Countries = append(mi.Countries, c.Name)
	}
	for _, cr := range raw.Credits.Crew {
		if cr.Job == "Director" {
			mi.Directors = append(mi.Directors, cr.Name)
		}
	}
	for _, c := range raw.Credits.Cast {
		mi.Cast = append(mi.Cast, CreditPerson{Name: c.Name, Role: c.Character, Avatar: tmdbImage(c.ProfilePath)})
	}
	for _, r := range raw.Recommendations.Results {
		if r.PosterPath == "" {
			continue
		}
		rt := r.Title
		if rt == "" {
			rt = r.Name
		}
		rdate := r.Release
		if mt == TypeTV {
			rdate = r.FirstAir
		}
		mi.Recommendations = append(mi.Recommendations, MediaInfo{
			Source: "tmdb", SourceID: string(mt) + ":" + strconv.FormatInt(r.ID, 10),
			Title: rt, Type: mt, Overview: r.Overview, PosterURL: tmdbImage(r.PosterPath),
			ReleaseDate: rdate, Rating: r.Vote, Year: yearFromDate(rdate),
		})
	}
	return mi, nil
}

func tmdbImage(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http") {
		return path
	}
	return tmdbImgBase + path
}

// Trending 实现 TrendingCapable：TMDB 每周趋势/热门榜。
//
//	movie   → /trending/movie/week
//	tv      → /trending/tv/week
//	popular → /movie/popular
//	anime   → /discover/tv?with_genres=16（TMDB 无独立动漫 trending 端点；
//	         用「动画」类型 + 日语原产 + 热度排序近似，可按需放宽语言过滤）
//
// trending 端点结果不含 media_type（路径已指定），故按 category 决定类型。
func (t *TMDB) Trending(ctx context.Context, category string, page int) ([]MediaInfo, error) {
	if t.apiKey == "" {
		return nil, ErrNotConfigured
	}
	path := "/trending/movie/week"
	mt := TypeMovie
	params := map[string]string{"page": strconv.Itoa(max(1, page))}
	switch category {
	case "tv":
		path = "/trending/tv/week"
		mt = TypeTV
	case "popular":
		path = "/movie/popular"
	case "anime":
		// 动漫：TMDB 无独立 trending 端点，改用 discover/tv 按动画类型(genre 16) +
		// 日语原产聚焦日本动画，按热度排序。旧实现缺此分支会回落到 movie，等于忽略动漫分类。
		path = "/discover/tv"
		mt = TypeTV
		params["with_genres"] = "16"
		params["with_original_language"] = "ja"
		params["sort_by"] = "popularity.desc"
	}
	var r tmdbMultiResult
	if err := t.get(ctx, path, params, &r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.Results))
	for _, it := range r.Results {
		title := it.Title
		if title == "" {
			title = it.Name
		}
		date := it.Release
		if mt == TypeTV {
			date = it.FirstAir
		}
		out = append(out, MediaInfo{
			Source: "tmdb", SourceID: string(mt) + ":" + strconv.FormatInt(it.ID, 10),
			Title: title, Type: mt, Overview: it.Overview,
			ReleaseDate: date, Rating: it.Vote, VoteCount: int(it.VoteCount),
			PosterURL: tmdbImage(it.PosterPath), Year: yearFromDate(date),
		})
	}
	return out, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func splitTypeID(id string) (mtype, sid string, ok bool) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || (parts[0] != "movie" && parts[0] != "tv") {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func yearFromDate(date string) int {
	if len(date) >= 4 {
		y, err := strconv.Atoi(date[:4])
		if err == nil {
			return y
		}
	}
	return 0
}

// CalendarEpisode 日历上的一天条目（开播日历）。
type CalendarEpisode struct {
	TMDBID    string `json:"tmdb_id"`
	Title     string `json:"title"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	AirDate   string `json:"air_date"` // YYYY-MM-DD
	Overview  string `json:"overview,omitempty"`
	PosterURL string `json:"poster_url,omitempty"`
}

// Calendar 开播日历（对标 MSaber 开播日历）：TMDB discover 按首播/上映日期窗口聚合。
// kind=tv → /discover/tv with first_air_date.gte/lte（每集粒度用 season 信息近似：仅标 S 首播）；
// kind=movie → /discover/movie with primary_release_date.gte/lte。
// days 覆盖 [today, today+days)，按日期分组返回（map[date]→items）。
// 说明：TMDB discover 无「单集级」日期端点（that is /tv/{id}/season），日历粒度为「作品当日
