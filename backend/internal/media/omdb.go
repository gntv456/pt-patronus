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
	omdbDefaultBase = "https://www.omdbapi.com"
)

// OMDb 是 IMDb 元数据源（走 OMDb API 免费层，补全 IMDb 评分/投票数用于多源聚合）。
// apiKey 为空时 Search/Detail 返回 ErrNotConfigured（Chain 静默跳过）。
//
// OMDb 的 ?t=<title>（按标题精确匹配）返回单条最佳匹配，正好契合「多源聚合取每源最佳匹配」；
// imdbRating/imdbVotes 为字符串（"7.5"/"1,234,567" 或 "N/A"），需解析。Source 名记 "imdb"。
type OMDb struct {
	apiKey string
	base   string
	hc     *http.Client
}

// NewOMDb 构造；apiKey 为空时 Chain 跳过该源。
func NewOMDb(apiKey string) *OMDb {
	return &OMDb{apiKey: apiKey, base: omdbDefaultBase, hc: &http.Client{Timeout: 15 * time.Second}}
}

// Name 实现 MediaSource。
func (o *OMDb) Name() string { return "imdb" }

// Types 实现 TypedSource：IMDb 覆盖 movie/tv。
func (o *OMDb) Types() []MediaType { return []MediaType{TypeMovie, TypeTV} }

// Search 用 ?t=<title>(&y=&type=) 取单条标题最佳匹配（0 或 1 条）。
// OMDb 的 ?s= 列表不含评分，对聚合无用，故走 ?t 精确匹配。
func (o *OMDb) Search(ctx context.Context, keyword string, year int) ([]MediaInfo, error) {
	if o.apiKey == "" {
		return nil, ErrNotConfigured
	}
	params := map[string]string{"t": keyword}
	if year > 0 {
		params["y"] = strconv.Itoa(year)
	}
	if mtype, ok := omdbGuessType(keyword); ok {
		params["type"] = mtype
	}
	var raw omdbTitle
	if err := o.get(ctx, params, &raw); err != nil {
		return nil, err
	}
	if raw.Response == "False" || raw.IMDbID == "" {
		return nil, nil // 无匹配（标题不符），非错误
	}
	mi, ok := omdbToMedia(raw)
	if !ok {
		return nil, nil
	}
	if year > 0 && mi.Year > 0 && abs(mi.Year-year) > 1 {
		return nil, nil
	}
	return []MediaInfo{mi}, nil
}

// Detail 用 ?i=<imdbID>（id 形如 "tt1234567"）。填演职/时长/出品国等富字段。
func (o *OMDb) Detail(ctx context.Context, id string) (*MediaInfo, error) {
	if o.apiKey == "" {
		return nil, ErrNotConfigured
	}
	var raw omdbTitle
	if err := o.get(ctx, map[string]string{"i": id, "plot": "full"}, &raw); err != nil {
		return nil, err
	}
	if raw.Response == "False" {
		return nil, fmt.Errorf("omdb: %s", raw.Error)
	}
	mi, ok := omdbToMedia(raw)
	if !ok {
		return nil, fmt.Errorf("omdb: no data for %q", id)
	}
	mi.SourceID = id
	mi.Overview = raw.Plot
	mi.Runtime = parseMinutes([]string{raw.Runtime})
	for _, d := range strings.Split(raw.Director, ",") {
		if d = strings.TrimSpace(d); d != "" && d != "N/A" {
			mi.Directors = append(mi.Directors, d)
		}
	}
	for _, c := range strings.Split(raw.Actors, ",") {
		if c = strings.TrimSpace(c); c != "" && c != "N/A" {
			mi.Cast = append(mi.Cast, CreditPerson{Name: c})
		}
	}
	for _, g := range strings.Split(raw.Genre, ",") {
		if g = strings.TrimSpace(g); g != "" && g != "N/A" {
			mi.Genres = append(mi.Genres, g)
		}
	}
	if raw.Country != "" && raw.Country != "N/A" {
		mi.Countries = append(mi.Countries, raw.Country)
	}
	return &mi, nil
}

// omdbTitle 是 ?t/?i 的响应（扁平字段；Response/Error 用于判定成败）。
type omdbTitle struct {
	Title       string `json:"Title"`
	Year        string `json:"Year"`
	Rated       string `json:"Rated"`
	Released    string `json:"Released"`
	Runtime     string `json:"Runtime"`
	Genre       string `json:"Genre"`
	Director    string `json:"Director"`
	Actors      string `json:"Actors"`
	Country     string `json:"Country"`
	Plot        string `json:"Plot"`
	IMDbID      string `json:"imdbID"`
	Type        string `json:"Type"`
	Poster      string `json:"Poster"`
	IMDbRating  string `json:"imdbRating"`
	IMDbVotes   string `json:"imdbVotes"`
	Response    string `json:"Response"`
	Error       string `json:"Error"`
}

// get 发 GET 并解码；OMDb 一律 HTTP 200，靠 Response 字段判成败。
func (o *OMDb) get(ctx context.Context, params map[string]string, out *omdbTitle) error {
	q := make(url.Values)
	q.Set("apikey", o.apiKey)
	q.Set("r", "json")
	for k, v := range params {
		q.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+"/?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := o.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("omdb: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// omdbToMedia 把 OMDb 标题响应映射为 MediaInfo（评分/投票数解析字符串）。
func omdbToMedia(raw omdbTitle) (MediaInfo, bool) {
	if raw.IMDbID == "" {
		return MediaInfo{}, false
	}
	mt := TypeMovie
	if raw.Type == "series" {
		mt = TypeTV
	}
	return MediaInfo{
		Source:      "imdb",
		SourceID:    raw.IMDbID,
		Title:       raw.Title,
		Type:        mt,
		PosterURL:   raw.Poster,
		ReleaseDate: omdbReleasedDate(raw),
		Rating:      parseFloat(raw.IMDbRating),
		VoteCount:   parseVotes(raw.IMDbVotes),
		Year:        yearFromDate(raw.Year),
	}, true
}

// omdbGuessType 关键词无法可靠推断类型，留空让 OMDb 自行匹配（默认 movie 优先）。
// 保留函数以便后续按关键词扩展（如「第N季」→ series），当前恒 false。
func omdbGuessType(_ string) (string, bool) { return "", false }

// omdbReleasedDate 取 Released（完整日期 dd MMM yyyy）的年份兜底；不可解析返回 Year 串。
func omdbReleasedDate(raw omdbTitle) string {
	if raw.Released != "" && raw.Released != "N/A" {
		return raw.Released
	}
	return raw.Year
}

// parseFloat 解析 OMDb 评分串（"7.5" 或 "N/A"）；失败返回 0。
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "N/A" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// parseVotes 解析 OMDb 投票串（"1,234,567" 或 "N/A"）为整数；失败返回 0。
func parseVotes(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "N/A" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", "")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
