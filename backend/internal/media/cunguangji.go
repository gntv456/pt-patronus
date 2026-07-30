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

// 寸光集（dj.hxpt.org / 本地 D:\www.ptang.top「寸光集」）默认站点根。
// 可经 media.cunguangji_base（或 CUNGUANGJI_BASE）覆盖，便于联调本地实例。
const cunguangjiDefaultBase = "https://dj.hxpt.org"

// Cunguangji 是寸光集短剧源：走站点公开只读 API /api/explore（列表/搜索）+
// /api/explore/<id>（详情）。短剧归 TypeTV（视频桶），仅产「短剧」category（CategoryCapable），
// 故 RecommendChain 在「短剧」分类下只取本源，其它源被跳过。
//
// 封面由站点端点规整为绝对 https://<host>/api/img?url=… 地址（站点自带反代，浏览器 <img>
// 直连即可取图），故 PTPatronus 无需把 dj.hxpt.org 加入图片反代白名单。
type Cunguangji struct {
	base string
	hc   *http.Client
}

// NewCunguangji 构造；base 为空用默认 dj.hxpt.org，自动去掉尾斜杠。
func NewCunguangji(base string) *Cunguangji {
	if base = strings.TrimSpace(base); base == "" {
		base = cunguangjiDefaultBase
	}
	base = strings.TrimRight(base, "/")
	return &Cunguangji{base: base, hc: &http.Client{Timeout: 10 * time.Second}}
}

func (x *Cunguangji) Name() string { return "cunguangji" }

// Types 实现 TypedSource：短剧归 TypeTV（影视/视频桶），使「影视」搜索纳入短剧、
// 音乐过滤时跳过本源。
func (x *Cunguangji) Types() []MediaType { return []MediaType{TypeTV} }

// Categories 实现 CategoryCapable：寸光集只产「短剧」。
func (x *Cunguangji) Categories() []string { return []string{"duanju"} }

// get 发 GET 并解码到 out；非 2xx 返回携带状态的错误（Chain 仅记 Debug、不影响其它源）。
func (x *Cunguangji) get(ctx context.Context, path string, params map[string]string, out any) error {
	u, err := url.Parse(x.base + path)
	if err != nil {
		return err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PTPatronus/1.0 (https://github.com/hxpt/ptpatronus)")
	req.Header.Set("Accept", "application/json")
	resp, err := x.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cunguangji %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// xgItem 是 /api/explore 列表项（站点端点返回 camelCase，与 /api/list 一致）。
type xgItem struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	AltTitle      string  `json:"altTitle"`
	Cover         string  `json:"cover"`
	Description   string  `json:"description"`
	ReleaseYear   int     `json:"releaseYear"`
	Rating        float64 `json:"rating"`
	Tags          string  `json:"tags"`
	Region        string  `json:"region"`
	TotalEpisodes int     `json:"totalEpisodes"`
	Status        string  `json:"status"`
}

type xgListResp struct {
	Items []xgItem `json:"items"`
	Total int      `json:"total"`
}

// xgDetailItem 是 /api/explore/<id> 详情；额外带演职。
type xgDetailItem struct {
	xgItem
	Casts []struct {
		Name   string `json:"name"`
		Role   string `json:"role"`
		Avatar string `json:"avatar"`
	} `json:"casts"`
}

// resolveURL 把站点返回的相对路径（如 /api/img?url=…）补成绝对地址（拼 x.base）；
// 已是绝对 http(s) URL 或空则原样返回。站点端点故意只回站内相对路径（nginx 反代下
// req.nextUrl.origin 不可靠），由本端用配置的 base 兜成绝对地址。
func (x *Cunguangji) resolveURL(u string) string {
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return x.base + u
	}
	return u
}

// itemToMedia 把一条短剧映射到 MediaInfo（封面相对路径补绝对）。
func (x *Cunguangji) itemToMedia(it xgItem) MediaInfo {
	mi := MediaInfo{
		Source:        "cunguangji",
		SourceID:      strconv.Itoa(it.ID),
		Title:         it.Title,
		OriginalTitle: it.AltTitle,
		Type:          TypeTV,
		Overview:      it.Description,
		PosterURL:     x.resolveURL(it.Cover),
		Year:          it.ReleaseYear,
		Rating:        it.Rating,
		Genres:        splitTags(it.Tags),
	}
	if it.Status != "" {
		// 把状态（连载中/已完结）补进简介前端可见，避免新增字段。
		if mi.Overview != "" {
			mi.Overview = "《" + it.Status + "》 " + mi.Overview
		} else {
			mi.Overview = it.Status
		}
	}
	return mi
}

// splitTags 拆 tags 字符串（逗号/竖线/中文逗号分隔）为切片；空返回 nil。
func splitTags(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '|' || r == '，'
	})
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Search 调 /api/explore?q=<keyword>。短剧站点的关键词搜索。
func (x *Cunguangji) Search(ctx context.Context, keyword string, year int) ([]MediaInfo, error) {
	var r xgListResp
	if err := x.get(ctx, "/api/explore", map[string]string{"q": keyword, "page": "1", "pageSize": "20"}, &r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.Items))
	for _, it := range r.Items {
		if it.Title == "" {
			continue
		}
		if year > 0 && it.ReleaseYear > 0 && abs(it.ReleaseYear-year) > 1 {
			continue // 年份容差 ±1
		}
		out = append(out, x.itemToMedia(it))
	}
	return out, nil
}

// Trending 实现 TrendingCapable：取寸光集高分短剧（推荐信息流）。仅在「短剧」分类被调用。
func (x *Cunguangji) Trending(ctx context.Context, category string, page int) ([]MediaInfo, error) {
	if page < 1 {
		page = 1
	}
	var r xgListResp
	if err := x.get(ctx, "/api/explore", map[string]string{
		"sort":     "top",
		"page":     strconv.Itoa(page),
		"pageSize": "20",
	}, &r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.Items))
	for _, it := range r.Items {
		if it.Title == "" {
			continue
		}
		out = append(out, x.itemToMedia(it))
	}
	return out, nil
}

// Detail 调 /api/explore/<id> 取单条短剧详情（含演职）。
func (x *Cunguangji) Detail(ctx context.Context, id string) (*MediaInfo, error) {
	if id = strings.TrimSpace(id); id == "" {
		return nil, fmt.Errorf("cunguangji: empty id")
	}
	var d xgDetailItem
	if err := x.get(ctx, "/api/explore/"+url.PathEscape(id), nil, &d); err != nil {
		return nil, err
	}
	if d.Title == "" {
		return nil, fmt.Errorf("cunguangji: drama %s not found", id)
	}
	mi := x.itemToMedia(d.xgItem)
	for _, c := range d.Casts {
		if c.Name == "" {
			continue
		}
		mi.Cast = append(mi.Cast, CreditPerson{Name: c.Name, Role: c.Role, Avatar: x.resolveURL(c.Avatar)})
	}
	return &mi, nil
}
