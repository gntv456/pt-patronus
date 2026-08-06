package schemas

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

func sunnyItemSize(it map[string]any) int64 {
	for _, k := range []string{"size", "total_size", "torrent_size", "remain"} {
		if v, ok := it[k]; ok {
			switch s := v.(type) {
			case float64:
				return int64(s)
			case string:
				return site.ParseBytes(s)
			}
		}
	}
	return 0
}

func (g *GenericJSON) SearchSunnypt(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	if page < 1 {
		page = 1
	}
	apiBase := sunnyAPIBase(baseURL)
	tok, err := sunnyAccessToken(cookie)
	if err != nil {
		return nil, err
	}
	const limit = 10
	m := map[string]any{"limit": limit, "offset": (page - 1) * limit}
	if kw := strings.TrimSpace(keyword); kw != "" {
		m["keyword"] = kw
	}
	reqBody, _ := json.Marshal(m)
	resp, err := g.hc.Do(ctx, "POST", joinURL(apiBase, "/api/v1/torrent/list"), reqBody, map[string]string{
		"Authorization": "Bearer " + tok,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Origin":        "https://sunnypt.top",
		"Referer":       "https://sunnypt.top/",
	}, "", true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	body, st := resp.Body, resp.StatusCode
	tail := string(body)
	if len(tail) > 2048 {
		tail = tail[:2048]
	}
	slog.Debug("sunnypt torrent/list", "status", st, "keyword", keyword, "len", len(body), "body", tail)
	var wrap struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &wrap) != nil || wrap.Code != 0 {
		return []site.Torrent{}, nil
	}
	items := sunnyItems(wrap.Data)
	out := make([]site.Torrent, 0, len(items))
	for _, it := range items {
		t := sunnyItemToTorrent(it)
		if t.Title != "" || t.ID != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

func sunnyItems(data json.RawMessage) []map[string]any {
	if len(data) == 0 {
		return nil
	}
	var arr []map[string]any
	if json.Unmarshal(data, &arr) == nil {
		return arr
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil {
		return nil
	}
	for _, k := range []string{"list", "records", "torrents", "items", "data", "rows"} {
		if raw, ok := obj[k]; ok {
			if json.Unmarshal(raw, &arr) == nil {
				return arr
			}
		}
	}
	return nil
}

func sunnyItemToTorrent(it map[string]any) site.Torrent {
	str := func(ks ...string) string {
		for _, k := range ks {
			if v, ok := it[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
		return ""
	}
	floatOf := func(ks ...string) (float64, bool) {
		for _, k := range ks {
			if v, ok := it[k]; ok {
				if n, ok := v.(float64); ok {
					return n, true
				}
			}
		}
		return 0, false
	}
	num := func(ks ...string) int {
		for _, k := range ks {
			if v, ok := it[k]; ok {
				switch n := v.(type) {
				case float64:
					return int(n)
				case string:
					if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
						return i
					}
				}
			}
		}
		return 0
	}
	// id:字符串或数字
	id := str("id", "tid", "torrent_id", "info_hash", "uuid", "sid", "fid")
	if id == "" {
		if n, ok := floatOf("id", "tid", "torrent_id", "info_hash", "uuid", "sid", "fid"); ok {
			id = strconv.FormatFloat(n, 'f', -1, 64)
		}
	}
	// size:字符串或字节数
	size := str("size", "total_size", "file_size", "torrent_size", "bytes")
	if size == "" {
		if n, ok := floatOf("size", "total_size", "file_size", "torrent_size", "bytes", "fileLength", "length"); ok {
			size = site.FormatBytes(int64(n))
		}
	}
	t := site.Torrent{
		ID:        id,
		Title:     str("name", "title", "subject", "fullname"),
		SubTitle:  str("subtitle", "small_desc", "description"),
		Size:      size,
		Seeders:   num("seeders", "seeding", "seed"),
		Leechers:  num("leechers", "leeching", "leech"),
		Completed: num("completed", "snatched", "finish"),
		Category:  str("category", "cat", "type"),
	}
	if id != "" {
		t.DetailURL = "https://sunnypt.top/torrent/" + id
		// chain 的 sunnypt 分支据此 id 走 Bearer 直连 api.sunnypt.top 下载 .torrent
		t.DownloadURL = "https://sunnypt.top/torrent/" + id
	}
	return t
}

var sunnyTorrentIDRe = regexp.MustCompile(`/torrent/([A-Za-z0-9]+)`)

// sunnyMapStr 从 map 里按候选键名取首个非空字符串。

func sunnyMapStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func sunnyMapNum(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if n, ok := v.(float64); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func sunnyMapStrSlice(m map[string]any, keys ...string) []string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if arr, ok := v.([]any); ok {
				out := make([]string, 0, len(arr))
				for _, it := range arr {
					if s, ok := it.(string); ok && s != "" {
						out = append(out, s)
					}
				}
				if len(out) > 0 {
					return out
				}
			}
		}
	}
	return nil
}

func sunnyIDFromURL(u string) string {
	u = strings.TrimRight(u, "/")
	if i := strings.LastIndex(u, "/"); i >= 0 {
		return u[i+1:]
	}
	return u
}

func sunnyAbsURL(u string) string {
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return "https://sunnypt.top/" + strings.TrimPrefix(u, "/")
}

func sunnyAbsURLs(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if a := sunnyAbsURL(s); a != "" {
			out = append(out, a)
		}
	}
	return out
}
