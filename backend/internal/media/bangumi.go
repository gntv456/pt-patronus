// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	bangumiAPIBase = "https://api.bgm.tv"
	bangumiImgBase = "https://lain.bgm.tv"
	// bgm.tv 要求 UA 带应用标识（社区规范），否则可能被限流。
	bangumiUA = "PTPatronus/1.0 (https://github.com/hxpt/ptpatronus)"
)

// Bangumi 是 bgm.tv 元数据源（动漫/游戏/音乐/书籍/三次元）。
// 走 v0 JSON API；匿名即可用（有速率限制），可选 Bearer token 提升配额。
// Bangumi subject type：1=书籍 2=动漫 3=音乐 4=游戏 6=三次元(影视剧)。
type Bangumi struct {
	token string // 可选 Bearer；非空时注入 Authorization 头
	base  string
	hc    *http.Client
}

// NewBangumi 构造；token 为空走匿名配额。
func NewBangumi(token string) *Bangumi {
	return &Bangumi{token: token, base: bangumiAPIBase, hc: &http.Client{Timeout: 15 * time.Second}}
}

// Name 实现 MediaSource。
func (b *Bangumi) Name() string { return "bangumi" }

// Types 实现 TypedSource：Bangumi 把动漫(2)/三次元(6) 都映射成 TypeTV。
func (b *Bangumi) Types() []MediaType { return []MediaType{TypeTV} }

// Categories 实现 CategoryCapable：Bangumi Trending 支持动漫/剧集/影视/即将/三次元。
func (b *Bangumi) Categories() []string { return []string{"anime", "tv", "movie", "coming", "real"} }

// do 发请求并解码到 out。method=GET 时 body 为 nil；POST 时 body 为请求体 map。
func (b *Bangumi) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", bangumiUA)
	req.Header.Set("Accept", "application/json")
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return fmt.Errorf("bangumi rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bangumi %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// bgmSubject 是 v0 /v0/search/subjects 与 /v0/subjects/{id} 的公共字段。
// rating 在两处均为 {score,total,rank} 结构（待真实校准：搜索响应偶有扁平 rating_score）。
type bgmSubject struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	NameCn  string `json:"name_cn"`
	Summary string `json:"summary"`
	Date    string `json:"date"` // YYYY-MM-DD
	Type    int    `json:"type"`
	NSFW    bool   `json:"nsfw"`
	Rating  struct {
		Score float64 `json:"score"`
		Total int     `json:"total"` // 投票人数（统计意义）
	} `json:"rating"`
	Images struct {
		Large string `json:"large"`
	} `json:"images"`
	Infobox []bgmInfoboxItem `json:"infobox"`
}

// bgmInfoboxItem infobox 键值对；value 可能是 string 或 []string，用 RawMessage 延迟解析。
type bgmInfoboxItem struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// bgmSubjectToMedia 把 Bangumi subject 映射到统一 MediaInfo。
// 动漫(2)/三次元(6) 归 TypeTV（无 TypeAnime，前端按 source=="bangumi" 区分）。
func bgmSubjectToMedia(it bgmSubject) MediaInfo {
	title := it.NameCn
	if title == "" {
		title = it.Name
	}
	return MediaInfo{
		Source:        "bangumi",
		SourceID:      "bangumi:" + strconv.FormatInt(it.ID, 10),
		Title:         title,
		OriginalTitle: it.Name,
		Type:          TypeTV,
		Overview:      it.Summary,
		ReleaseDate:   it.Date,
		PosterURL:     bangumiImage(it.Images.Large),
		Rating:        it.Rating.Score,
		VoteCount:     it.Rating.Total,
		Year:          yearFromDate(it.Date),
	}
}

// Search POST /v0/search/subjects。filter.type 限动漫(2)+三次元(6)，覆盖影视/动漫。
func (b *Bangumi) Search(ctx context.Context, keyword string, year int) ([]MediaInfo, error) {
	body := map[string]any{
		"keyword": keyword,
		"filter":  map[string]any{"type": []int{2, 6}},
		"sort":    "match",
	}
	var r struct {
		Data []bgmSubject `json:"data"`
	}
	if err := b.do(ctx, http.MethodPost, "/v0/search/subjects", body, &r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.Data))
	for _, it := range r.Data {
		mi := bgmSubjectToMedia(it)
		if year > 0 && mi.Year > 0 && abs(mi.Year-year) > 1 {
			continue // 年份容差 ±1
		}
		out = append(out, mi)
	}
	return out, nil
}

// Detail GET /v0/subjects/{id}（id 形如 "bangumi:12345" 或纯数字）。
func (b *Bangumi) Detail(ctx context.Context, id string) (*MediaInfo, error) {
	sid := strings.TrimPrefix(id, "bangumi:")
	if sid == "" {
		return nil, fmt.Errorf("bangumi: bad id %q", id)
	}
	var raw bgmSubject
	if err := b.do(ctx, http.MethodGet, "/v0/subjects/"+sid, nil, &raw); err != nil {
		return nil, err
	}
	mi := bgmSubjectToMedia(raw)
	mi.SourceID = id
	mi.Names = bangumiAliasFromInfobox(raw.Infobox)
	return &mi, nil
}

// bangumiAliasFromInfobox 从 infobox 提取「别名」（value 为 string 或 []string；string 多值按换行/顿号/分号拆分）。
func bangumiAliasFromInfobox(infobox []bgmInfoboxItem) []string {
	for _, it := range infobox {
		if it.Key != "别名" {
			continue
		}
		var arr []string
		if json.Unmarshal(it.Value, &arr) == nil {
			return arr
		}
		var s string
		if json.Unmarshal(it.Value, &s) == nil {
			return splitAliasString(s)
		}
	}
	return nil
}

func splitAliasString(s string) []string {
	s = strings.ReplaceAll(s, "\n", "/")
	s = strings.ReplaceAll(s, "、", "/")
	s = strings.ReplaceAll(s, ";", "/")
	s = strings.ReplaceAll(s, "；", "/")
	var out []string
	for _, p := range strings.Split(s, "/") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Trending 实现 TrendingCapable：按热度取动漫(category="anime")，
// 或按上映时间(category="coming")，或番组日历(category="calendar")。
// 走 /v0/search/subjects 的 heat/rank 排序（待真实校准）。
func (b *Bangumi) Trending(ctx context.Context, category string, page int) ([]MediaInfo, error) {
	if page < 1 {
		page = 1
	}
	types := []int{2} // 默认动漫
	sortBy := "heat"
	switch category {
	case "tv", "movie", "real":
		types = []int{6}
		sortBy = "heat"
	case "coming":
		types = []int{2, 6}
		sortBy = "date"
	}
	body := map[string]any{
		"keyword": "",
		"filter":  map[string]any{"type": types},
		"sort":    sortBy,
		"limit":   20,
		"offset":  (page - 1) * 20,
	}
	var r struct {
		Data []bgmSubject `json:"data"`
	}
	if err := b.do(ctx, http.MethodPost, "/v0/search/subjects", body, &r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.Data))
	for _, it := range r.Data {
		out = append(out, bgmSubjectToMedia(it))
	}
	return out, nil
}

// bangumiImage 把 Bangumi 图片路径规整为绝对 URL。
// v0 返回的 images.large 多为 //lain.bgm.tv/... 协议相对地址，补 https:。
func bangumiImage(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http") {
		return p
	}
	if strings.HasPrefix(p, "//") {
		return "https:" + p
	}
	return bangumiImgBase + p
}
