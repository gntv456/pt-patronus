package schemas

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// Sunny（sunnypt.top）已从 NexusPHP 整体改版为 Next.js SPA + 独立 JSON API（2026 年较大更新）。
//
// 鉴权模型：浏览器持 AuthSession cookie（NextAuth JWT），其 payload 内含 accessToken
// （自包含 JWT）；SPA 取之作 `Authorization: Bearer <accessToken>` 打 https://api.sunnypt.top。
// 旧的 c_secure_* cookie 为遗留、不再生效；sunnypt.top 主站的 NextAuth 会话层对后端直接请求
// 会回 shouldClearAuth（拒登），但 accessToken 自包含、独立校验，直接打 api.sunnypt.top 可用。
//
// 端点契约（Bearer + JSON，外层 {code,msg,data}，code==0 成功）：
//
//	GET /api/v1/user/details/info   → 上传/下载/真实量/分享率/魔力/等级/加入时间…
//	GET /api/v1/user/seed-bonus     → {seed_bonus}
//	GET /api/v1/user/details/seeding→ 做种列表摘要（count + 总体积）
//
// 本文件为 *GenericJSON 实现 def.ID=="sunnypt" 的 FetchUserInfo / Search；其余 tnode 站不受影响。

var sunnyAuthSessionRe = regexp.MustCompile(`AuthSession=([^;\s]+)`)

// sunnyCST 站点时区（timezone_offset=+0800）。
var sunnyCST = time.FixedZone("CST", 8*3600)

// sunnyLevels class id → 等级名（对齐 definitions/sunnypt.yaml level_requirements）。
var sunnyLevels = map[int]string{
	1: "User", 2: "Power User", 3: "Elite User", 4: "Crazy User",
	5: "Insane User", 6: "Veteran User", 7: "Extreme User",
	8: "Ultimate User", 9: "Nexus Master",
}

// sunnyAPIBase 由站点 base_url 推导数据 API 域（sunnypt.top → api.sunnypt.top）。
func sunnyAPIBase(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return "https://api.sunnypt.top"
	}
	host := u.Hostname()
	if !strings.HasPrefix(host, "api.") {
		host = "api." + host
	}
	u.Host = host
	if port := u.Port(); port != "" {
		u.Host += ":" + port
	}
	return strings.TrimRight(u.String(), "/")
}

// sunnyJWTPayload base64url 解码 JWT 的 payload 段（补齐填充）。
func sunnyJWTPayload(token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("sunnypt: JWT 段数不足")
	}
	pad := strings.Repeat("=", (4-len(parts[1])%4)%4)
	return base64.URLEncoding.DecodeString(parts[1] + pad)
}

// sunnyAccessToken 从 cookie 串里解析 AuthSession JWT，取内层 accessToken。
// 兼容 cookie 串中存在多份 AuthSession（取最长 = 完整 JWT）。
func sunnyAccessToken(cookie string) (string, error) {
	matches := sunnyAuthSessionRe.FindAllString(cookie, -1)
	var best string
	for _, m := range matches {
		val := strings.TrimPrefix(m, "AuthSession=")
		if strings.Count(val, ".") == 2 && len(val) > len(best) {
			best = val
		}
	}
	if best == "" {
		return "", fmt.Errorf("sunnypt: AuthSession cookie 未找到")
	}
	payload, err := sunnyJWTPayload(best)
	if err != nil {
		return "", err
	}
	var p struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("sunnypt: 解析 AuthSession payload: %w", err)
	}
	if p.AccessToken == "" {
		return "", fmt.Errorf("sunnypt: AuthSession 内无 accessToken")
	}
	return p.AccessToken, nil
}

// sunnyFmtDate 秒级时间戳 → 「2006-01-02 15:04」（站点时区 +0800）。
func sunnyFmtDate(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).In(sunnyCST).Format("2006-01-02 15:04")
}

// sunnyGet Bearer GET 数据 API；401/403 → ErrNotLoggedIn。仅用 accessToken，不发主站 cookie。
func (g *GenericJSON) sunnyGet(ctx context.Context, apiBase, cookie, path string) ([]byte, int, error) {
	tok, err := sunnyAccessToken(cookie)
	if err != nil {
		return nil, 0, err
	}
	resp, err := g.hc.GetWithHeaders(ctx, joinURL(apiBase, path), "", map[string]string{
		"Authorization": "Bearer " + tok,
		"Accept":        "application/json",
		"Origin":        "https://sunnypt.top",
		"Referer":       "https://sunnypt.top/",
	}, true)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, resp.StatusCode, site.ErrNotLoggedIn
	}
	return resp.Body, resp.StatusCode, nil
}

// FetchSunnyptUserInfo 抓 /user/details/info 并映射到 UserInfo；做种数/体积 best-effort 取
// /user/details/seeding 摘要。
func (g *GenericJSON) FetchSunnyptUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {

	apiBase := sunnyAPIBase(baseURL)
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/user/details/info")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID             int64   `json:"id"`
			Added          int64   `json:"added"`
			Username       string  `json:"username"`
			Email          string  `json:"email"`
			Avatar         string  `json:"avatar"`
			Class          int     `json:"class"`
			Ratio          float64 `json:"ratio"`
			SeedBonus      float64 `json:"seed_bonus"`
			SeedPoints     float64 `json:"seed_points"`
			Uploaded       string  `json:"uploaded"`
			Downloaded     string  `json:"downloaded"`
			RealUploaded   string  `json:"real_uploaded"`
			RealDownloaded string  `json:"real_downloaded"`
			UploadNum      int     `json:"upload_num"`
			DownloadNum    int     `json:"download_num"`
			LastAccess     int64   `json:"last_access"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("sunnypt: 解析用户信息: %w", err)
	}
	if wrap.Code != 0 {
		return nil, fmt.Errorf("sunnypt: 用户信息接口 code=%d msg=%s", wrap.Code, wrap.Msg)
	}
	d := wrap.Data
	u := &site.UserInfo{
		ID:             strconv.FormatInt(d.ID, 10),
		NumericUserID:  int(d.ID),
		Name:           d.Username,
		Email:          d.Email,
		Avatar:         d.Avatar,
		Level:          sunnyLevels[d.Class],
		LevelID:        d.Class,
		Ratio:          d.Ratio,
		Bonus:          d.SeedBonus,
		SeedingBonus:   d.SeedBonus,
		Uploaded:       site.ParseBytes(d.Uploaded),
		Downloaded:     site.ParseBytes(d.Downloaded),
		TrueUploaded:   site.ParseBytes(d.RealUploaded),
		TrueDownloaded: site.ParseBytes(d.RealDownloaded),
		Uploads:        d.UploadNum,
		JoinDate:       sunnyFmtDate(d.Added),
		LastAccessAt:   sunnyFmtDate(d.LastAccess),
	}
	g.sunnyFillSeeding(ctx, apiBase, cookie, u)
	return u, nil
}

// sunnyFillSeeding best-effort 从 /user/details/seeding 取做种数与总体积。
// 响应结构以实测为准（兼容 data 为 {list:[...],total} 或直接数组两种形态）；解析失败不报错。
func (g *GenericJSON) sunnyFillSeeding(ctx context.Context, apiBase, cookie string, u *site.UserInfo) {
	body, st, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/user/details/seeding?page=1")
	if err != nil || st != 200 {
		return
	}
	var raw struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &raw) != nil || raw.Code != 0 {
		return
	}
	// 形态1:data 是对象，含 list/records/torrents + total/count
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw.Data, &obj) == nil {
		for _, lk := range []string{"list", "records", "torrents", "data", "items"} {
			if arr, ok := obj[lk]; ok {
				var items []map[string]any
				if json.Unmarshal(arr, &items) == nil {
					u.Seeding = len(items)
					var size int64
					for _, it := range items {
						size += sunnyItemSize(it)
					}
					u.SeedingSize = size
					return
				}
			}
		}
		if t, ok := obj["total"]; ok {
			var n float64
			if json.Unmarshal(t, &n) == nil {
				u.Seeding = int(n)
			}
		}
		return
	}
	// 形态2:data 直接是数组
	var arr []map[string]any
	if json.Unmarshal(raw.Data, &arr) == nil {
		u.Seeding = len(arr)
		var size int64
		for _, it := range arr {
			size += sunnyItemSize(it)
		}
		u.SeedingSize = size
	}
}

// sunnyItemSize 从种子项 map 里取体积（兼容 size/total_size/remain 字段，数值或 "1.5 GB" 字符串）。
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

// SearchSunnypt 种子搜索。客户端直连 POST api.sunnypt.top/api/v1/torrent/list(Bearer
// accessToken + JSON body;浏览器 Network 实测:OPTIONS 预检 access-control-request-method=POST)。
// body 为前端筛选 schema(categories/keyword/matchMode/perPage/teams/titleOptions/sort/order/page)。
// 记录原始响应前 2KB 便于校准;code!=0 静默返回空,不拖垮聚合搜索。
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

// sunnyItems 从 data 取种子数组(兼容 data 为数组,或对象内 list/records/torrents/items/data/rows)。
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

// sunnyItemToTorrent 把种子项按常见键名 best-effort 映射(字段名待据真实响应校准)。
// id/size 兼容数字类型(JSON 数字在 Go 里是 float64)。
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

// sunnyMapNum 从 map 里按候选键名取首个数字(JSON number→float64)。
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

// sunnyMapStrSlice 从 map 里按候选键名取字符串数组(截图等)。
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

// sunnyIDFromURL 从外链 URL 取末段 id(imdb→tt1234567,douban/tmdb→数字)。
func sunnyIDFromURL(u string) string {
	u = strings.TrimRight(u, "/")
	if i := strings.LastIndex(u, "/"); i >= 0 {
		return u[i+1:]
	}
	return u
}

// sunnyAbsURL 把相对图片路径(pic/xxx.png)补成绝对 URL;已是绝对地址则原样返回。
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

// FetchTorrentDetail 种子详情。detailURL = https://sunnypt.top/torrent/<id>;
// 抽 id → GET api.sunnypt.top/api/v1/torrent/<id>(Bearer)→ 映射 TorrentDetail。
// 仅 def.ID=="sunnypt" 实现;其余 tnode 站返回 ErrNotSupported(由 chain 降级)。
func (g *GenericJSON) FetchTorrentDetail(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, detailURL string) (*site.TorrentDetail, error) {

	if def.ID != "sunnypt" {
		return nil, fmt.Errorf("sunnypt: detail not supported for site %s", def.ID)
	}
	m := sunnyTorrentIDRe.FindStringSubmatch(detailURL)
	if len(m) < 2 {
		return nil, fmt.Errorf("sunnypt: 无法从 %s 提取种子 id", detailURL)
	}
	apiBase := sunnyAPIBase(baseURL)
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/torrent/detail?id="+m[1])
	if err != nil {
		return nil, err
	}
	rb := string(body)
	if len(rb) > 12288 {
		rb = rb[:12288]
	}
	slog.Debug("sunnypt detail raw", "id", m[1], "len", len(body), "body", rb)

	var wrap struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &wrap) != nil || wrap.Code != 0 {
		return &site.TorrentDetail{DetailURL: detailURL}, nil
	}
	var d map[string]any
	if json.Unmarshal(wrap.Data, &d) != nil {
		return &site.TorrentDetail{DetailURL: detailURL}, nil
	}
	td := &site.TorrentDetail{
		DetailURL:   detailURL,
		Title:       sunnyMapStr(d, "name", "title", "subject", "fullname"),
		SubTitle:    sunnyMapStr(d, "subName", "subtitle", "small_desc", "smallDescr"),
		Description: sunnyMapStr(d, "description", "body", "content", "desc"),
		MediaInfo:   sunnyMapStr(d, "technical_info", "technicalInfo", "mediaInfo"),
		PosterURL:   sunnyAbsURL(sunnyMapStr(d, "cover", "poster", "poster_url")),
		Category:    sunnyMapStr(d, "category", "cat"),
		Codec:       sunnyMapStr(d, "video_codec", "codec"),
		Resolution:  sunnyMapStr(d, "resolution"),
		Source:      sunnyMapStr(d, "medium", "source"),
	}
	td.IMDBID = sunnyIDFromURL(sunnyMapStr(d, "imdb_url", "imdb"))
	td.DoubanID = sunnyIDFromURL(sunnyMapStr(d, "douban_url", "douban"))
	td.TMDBID = sunnyIDFromURL(sunnyMapStr(d, "tmdb_url", "tmdb"))
	if sz := sunnyMapStr(d, "size", "total_size", "file_size"); sz != "" {
		td.Size = sz
	} else if n, ok := sunnyMapNum(d, "size", "total_size", "file_size"); ok {
		td.Size = site.FormatBytes(int64(n))
	}
	if ss := sunnyMapStrSlice(d, "screenshot", "screenshots", "images"); len(ss) > 0 {
		td.Screenshots = sunnyAbsURLs(ss)
	} else if one := sunnyMapStr(d, "screenshot", "screenshots"); one != "" {
		// sunnypt 的 screenshot 是逗号分隔的字符串,拆成多条。
		td.Screenshots = sunnyAbsURLs(strings.Split(one, ","))
	}
	if n, ok := sunnyMapNum(d, "seeders"); ok {
		td.Seeders = int(n)
	}
	if n, ok := sunnyMapNum(d, "leechers"); ok {
		td.Leechers = int(n)
	}
	return td, nil
}
