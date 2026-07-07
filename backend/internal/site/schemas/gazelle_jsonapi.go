package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// GazelleJSONAPI 是 Gazelle 系站点的 JSON API 适配器（RED/OPS/DIC 等）。
// 走 /ajax.php?action=index|browse|user 信封 {status,response,error}，cookie 会话鉴权。
// GGn（gazellegames）用 X-API-Key token + /api.php 是离群点，M4 延后。
// 适配器是跨站单例，authkey 按 baseURL 缓存（PT-depiler 实例级缓存的正确性修正）。
// 契约来源：PT-depiler GazelleJSONAPI.ts。
type GazelleJSONAPI struct {
	hc   *httpclient.Client
	keys sync.Map // baseURL → gazelleAuthKey（authkey/passkey 每站不同）
}

type gazelleAuthKey struct{ authkey, passkey string }

// NewGazelleJSONAPI 构造适配器，注入共享 httpclient。
func NewGazelleJSONAPI(hc *httpclient.Client) *GazelleJSONAPI {
	return &GazelleJSONAPI{hc: hc}
}

func (g *GazelleJSONAPI) Schema() string { return "gazelle_jsonapi" }
func (g *GazelleJSONAPI) Name() string   { return "site.gazelle_jsonapi" }
func (g *GazelleJSONAPI) Priority() int  { return 100 }

// gazelleEnvelope 是 WhatCD/Gazelle ajax.php 统一响应信封。
type gazelleEnvelope struct {
	Status   string          `json:"status"` // success | failure | error
	Response json.RawMessage `json:"response"`
	Error    string          `json:"error"`
}

// gazelleIndexResp 是 action=index 的响应（当前用户 + authkey/passkey + 基础统计）。
type gazelleIndexResp struct {
	Username  string `json:"username"`
	ID        int64  `json:"id"`
	Authkey   string `json:"authkey"`
	Passkey   string `json:"passkey"`
	Userstats struct {
		Uploaded                  any    `json:"uploaded"`
		Downloaded                any    `json:"downloaded"`
		Ratio                     any    `json:"ratio"`
		Class                     any    `json:"class"`
		BonusPoints               any    `json:"bonusPoints"`
		BonusPointsPerHour        any    `json:"bonusPointsPerHour"`
		SeedingBonusPointsPerHour any    `json:"seedingBonusPointsPerHour"`
		SeedingCount              any    `json:"seedingCount"`
		SeedingSize               any    `json:"seedingSize"`
		LeechingCount             any    `json:"leechingCount"`
		JoinedDate                string `json:"joinedDate"`
		LastAccess                string `json:"lastAccess"`
	} `json:"userstats"`
}

// gazelleUserResp 是 action=user&id=N 的响应（扩展资料，best-effort 阶段 2）。
type gazelleUserResp struct {
	Stats struct {
		JoinedDate string `json:"joinedDate"`
		LastAccess string `json:"lastAccess"`
	} `json:"stats"`
	Community struct {
		Seeding  any `json:"seeding"`
		Uploaded any `json:"uploaded"`
	} `json:"community"`
}

// gazelleRequest GET /ajax.php?action=<action>&<params>，校验信封。
// status!="success" 时 isGazelleAuthError(error)→ErrNotLoggedIn（改进 PT-depiler 静默返空）；
// HTTP 401/403→ErrNotLoggedIn；CF 守卫。
func (g *GazelleJSONAPI) gazelleRequest(ctx context.Context, baseURL, cookie, action string, params map[string]string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("action", action)
	for k, v := range params {
		q.Set(k, v)
	}
	target := joinURL(baseURL, "ajax.php") + "?" + q.Encode()
	resp, err := g.hc.Get(ctx, target, cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	var env gazelleEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("parse gazelle_jsonapi response: %w", err)
	}
	if env.Status != "success" {
		if isGazelleAuthError(env.Error) {
			return nil, site.ErrNotLoggedIn
		}
		return nil, fmt.Errorf("gazelle_jsonapi %s failed: %s", action, env.Error)
	}
	return env.Response, nil
}

// isGazelleAuthError 启发式判定 ajax.php 业务错误是否为鉴权失效（error 串因 fork 而异，M6 调）。
func isGazelleAuthError(msg string) bool {
	low := strings.ToLower(msg)
	for _, k := range []string{"auth", "login", "credentials", "session", "token", "log in", "not logged"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// getAuthKey 取并缓存该站 authkey/passkey（action=index 一次），用于构造下载链。
func (g *GazelleJSONAPI) getAuthKey(ctx context.Context, baseURL, cookie string) (gazelleAuthKey, error) {
	if v, ok := g.keys.Load(baseURL); ok {
		return v.(gazelleAuthKey), nil
	}
	data, err := g.gazelleRequest(ctx, baseURL, cookie, "index", nil)
	if err != nil {
		return gazelleAuthKey{}, err
	}
	var idx gazelleIndexResp
	if err := json.Unmarshal(data, &idx); err != nil {
		return gazelleAuthKey{}, fmt.Errorf("parse gazelle index: %w", err)
	}
	k := gazelleAuthKey{authkey: idx.Authkey, passkey: idx.Passkey}
	g.keys.Store(baseURL, k)
	return k, nil
}

// buildGazelleDownloadURL 构造 {base}/torrents.php?action=download&id={tid}&authkey={ak}&torrent_pass={pk}。
func buildGazelleDownloadURL(baseURL string, torrentID any, k gazelleAuthKey) string {
	return fmt.Sprintf("%s/torrents.php?action=download&id=%v&authkey=%s&torrent_pass=%s",
		strings.TrimRight(baseURL, "/"), torrentID, k.authkey, k.passkey)
}

// Search action=browse。response.results[] 异构：group（含 torrents[]）或 flat torrent。
// 判别：JSON 含 "torrents" 键=group（PT-depiler "torrents" in group）。
func (g *GazelleJSONAPI) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {
	if def != nil && def.ID == "gazellegames" {
		return g.searchGazelleGames(ctx, baseURL, cookie, keyword, page)
	}
	if page < 1 {
		page = 1
	}
	params := map[string]string{}
	if kw := strings.TrimSpace(keyword); kw != "" {
		params["searchstr"] = kw
	}
	if page > 1 {
		params["page"] = strconv.Itoa(page)
	}
	data, err := g.gazelleRequest(ctx, baseURL, cookie, "browse", params)
	if err != nil {
		return nil, err
	}
	var browse struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(data, &browse); err != nil {
		return nil, fmt.Errorf("parse gazelle browse: %w", err)
	}
	// authkey best-effort：缺失则下载链留空（仍可点详情页降级）。
	k, kerr := g.getAuthKey(ctx, baseURL, cookie)
	out := make([]site.Torrent, 0, len(browse.Results))
	for _, raw := range browse.Results {
		// 判别 group vs flat：是否含 "torrents" 键（PT-depiler "torrents" in group）。
		// 用 map 键检查而非子串匹配，避免 groupName 含 "torrents" 字样误判。
		var probe map[string]json.RawMessage
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		_, isGroup := probe["torrents"]
		if isGroup {
			var grp struct {
				GroupName   string `json:"groupName"`
				Artist      string `json:"artist"`
				GroupYear   any    `json:"groupYear"`
				ReleaseType string `json:"releaseType"`
				Torrents    []struct {
					TorrentID   any  `json:"torrentId"`
					Size        any  `json:"size"`
					Seeders     any  `json:"seeders"`
					Leechers    any  `json:"leechers"`
					Snatches    any  `json:"snatches"`
					IsFreeleech bool `json:"isFreeleech"`
				} `json:"torrents"`
			}
			if json.Unmarshal(raw, &grp) != nil {
				continue
			}
			title := gazelleGroupTitle(grp.Artist, grp.GroupName, grp.GroupYear, grp.ReleaseType)
			for _, tor := range grp.Torrents {
				out = append(out, site.Torrent{
					ID:          fmt.Sprint(tor.TorrentID),
					Title:       title,
					Size:        formatMTBytes(toInt64(tor.Size)),
					Seeders:     int(toInt64(tor.Seeders)),
					Leechers:    int(toInt64(tor.Leechers)),
					Completed:   int(toInt64(tor.Snatches)),
					Free:        tor.IsFreeleech,
					DownloadURL: gazelleDL(baseURL, tor.TorrentID, k, kerr),
				})
			}
		} else {
			var tor struct {
				TorrentID   any    `json:"torrentId"`
				GroupName   string `json:"groupName"`
				Size        any    `json:"size"`
				Seeders     any    `json:"seeders"`
				Leechers    any    `json:"leechers"`
				Snatches    any    `json:"snatches"`
				Category    string `json:"category"`
				IsFreeleech bool   `json:"isFreeleech"`
			}
			if json.Unmarshal(raw, &tor) != nil {
				continue
			}
			out = append(out, site.Torrent{
				ID:          fmt.Sprint(tor.TorrentID),
				Title:       strings.TrimSpace(tor.GroupName),
				Category:    tor.Category,
				Size:        formatMTBytes(toInt64(tor.Size)),
				Seeders:     int(toInt64(tor.Seeders)),
				Leechers:    int(toInt64(tor.Leechers)),
				Completed:   int(toInt64(tor.Snatches)),
				Free:        tor.IsFreeleech,
				DownloadURL: gazelleDL(baseURL, tor.TorrentID, k, kerr),
			})
		}
	}
	return out, nil
}

func (g *GazelleJSONAPI) gazelleGamesRequest(ctx context.Context, baseURL, token, request string, params map[string]string) (json.RawMessage, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("gazellegames api token required")
	}
	q := url.Values{}
	q.Set("request", request)
	for k, v := range params {
		q.Set(k, v)
	}
	target := joinURL(baseURL, "api.php") + "?" + q.Encode()
	resp, err := g.hc.GetWithHeaders(ctx, target, "", map[string]string{
		"X-API-Key": token,
		"Accept":    "application/json",
	}, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	var env gazelleEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("parse gazellegames response: %w", err)
	}
	if env.Status != "success" {
		if isGazelleAuthError(env.Error) {
			return nil, site.ErrNotLoggedIn
		}
		return nil, fmt.Errorf("gazellegames %s failed: %s", request, env.Error)
	}
	return env.Response, nil
}

func (g *GazelleJSONAPI) getGazelleGamesAuthKey(ctx context.Context, baseURL, token string) (gazelleAuthKey, error) {
	cacheKey := baseURL + "|gazellegames"
	if v, ok := g.keys.Load(cacheKey); ok {
		return v.(gazelleAuthKey), nil
	}
	data, err := g.gazelleGamesRequest(ctx, baseURL, token, "quick_user", nil)
	if err != nil {
		return gazelleAuthKey{}, err
	}
	var idx struct {
		Authkey string `json:"authkey"`
		Passkey string `json:"passkey"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return gazelleAuthKey{}, fmt.Errorf("parse gazellegames quick_user: %w", err)
	}
	k := gazelleAuthKey{authkey: idx.Authkey, passkey: idx.Passkey}
	g.keys.Store(cacheKey, k)
	return k, nil
}

type gazelleGamesGroup struct {
	Artists []struct {
		Name string `json:"name"`
	} `json:"Artists"`
	Year     string                         `json:"Year"`
	Torrents map[string]gazelleGamesTorrent `json:"Torrents"`
}

type gazelleGamesTorrent struct {
	ID            any    `json:"ID"`
	GroupID       any    `json:"GroupID"`
	CategoryID    any    `json:"CategoryID"`
	Language      string `json:"Language"`
	Format        string `json:"Format"`
	Encoding      string `json:"Encoding"`
	Region        string `json:"Region"`
	Remastered    string `json:"Remastered"`
	RemasterYear  string `json:"RemasterYear"`
	RemasterTitle string `json:"RemasterTitle"`
	Scene         string `json:"Scene"`
	ReleaseTitle  string `json:"ReleaseTitle"`
	GameDOXType   string `json:"GameDOXType"`
	Size          any    `json:"Size"`
	Time          string `json:"Time"`
	Miscellaneous string `json:"Miscellaneous"`
	FreeTorrent   string `json:"FreeTorrent"`
	Seeders       any    `json:"Seeders"`
	Leechers      any    `json:"Leechers"`
	Snatched      any    `json:"Snatched"`
}

var gazelleGamesCategoryMap = map[int64]string{
	1: "Games",
	2: "Applications",
	3: "E-Books",
	4: "OST",
}

func (g *GazelleJSONAPI) searchGazelleGames(ctx context.Context, baseURL, token, keyword string, page int) ([]site.Torrent, error) {
	params := map[string]string{
		"search_type": "torrents",
		"order_by":    "time",
		"order_way":   "desc",
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		params["searchstr"] = strings.ReplaceAll(kw, ".", " ")
	}
	if page > 1 {
		params["page"] = strconv.Itoa(page)
	}
	data, err := g.gazelleGamesRequest(ctx, baseURL, token, "search", params)
	if err != nil {
		return nil, err
	}
	var groups map[string]gazelleGamesGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("parse gazellegames search: %w", err)
	}
	k, kerr := g.getGazelleGamesAuthKey(ctx, baseURL, token)
	out := []site.Torrent{}
	for _, group := range groups {
		for _, tor := range group.Torrents {
			id := fmt.Sprint(tor.ID)
			groupID := fmt.Sprint(tor.GroupID)
			out = append(out, site.Torrent{
				ID:          id,
				Title:       strings.TrimSpace(tor.ReleaseTitle),
				SubTitle:    gazelleGamesSubtitle(group, tor),
				DetailURL:   fmt.Sprintf("%s/torrents.php?id=%s&torrentid=%s", strings.TrimRight(baseURL, "/"), groupID, id),
				DownloadURL: gazelleDL(baseURL, id, k, kerr),
				Size:        formatMTBytes(toInt64(tor.Size)),
				Seeders:     int(toInt64(tor.Seeders)),
				Leechers:    int(toInt64(tor.Leechers)),
				Completed:   int(toInt64(tor.Snatched)),
				Time:        site.ApplyFilters(tor.Time, []site.Filter{{Name: "parse_time"}}),
				Category:    gazelleGamesCategory(group, tor),
				Free:        tor.FreeTorrent == "1",
			})
		}
	}
	return out, nil
}

func gazelleGamesSubtitle(group gazelleGamesGroup, tor gazelleGamesTorrent) string {
	var parts []string
	relYear := ""
	if tor.Miscellaneous != "GameDOX" {
		switch {
		case tor.RemasterYear != "" && tor.RemasterYear != "0":
			relYear = tor.RemasterYear
		case group.Year != "" && group.Year != "0":
			relYear = group.Year
		case tor.Remastered == "1":
			relYear = "Unknown Release(s)"
		}
	}
	edition := tor.RemasterTitle
	if tor.Miscellaneous == "GameDOX" {
		edition = tor.GameDOXType
	}
	for _, v := range []string{relYear, edition, tor.Language, tor.Region, tor.Format, tor.Encoding, tor.Miscellaneous} {
		if s := strings.TrimSpace(v); s != "" {
			parts = append(parts, s)
		}
	}
	if toInt64(tor.Scene) > 0 {
		parts = append(parts, "Scene")
	}
	return strings.Join(parts, " / ")
}

func gazelleGamesCategory(group gazelleGamesGroup, tor gazelleGamesTorrent) string {
	if len(group.Artists) > 0 && strings.TrimSpace(group.Artists[0].Name) != "" {
		return strings.TrimSpace(group.Artists[0].Name)
	}
	if cat := gazelleGamesCategoryMap[toInt64(tor.CategoryID)]; cat != "" {
		return cat
	}
	return fmt.Sprint(tor.CategoryID)
}

// gazelleGroupTitle 拼 "artist - groupName [year] [releaseType]"（PT-depiler transformGroupTorrent）。
func gazelleGroupTitle(artist, name string, year any, releaseType string) string {
	var b strings.Builder
	if a := strings.TrimSpace(artist); a != "" {
		b.WriteString(a)
		b.WriteString(" - ")
	}
	b.WriteString(strings.TrimSpace(name))
	if y := toInt64(year); y > 0 {
		fmt.Fprintf(&b, " [%d]", y)
	}
	if rt := strings.TrimSpace(releaseType); rt != "" {
		b.WriteString(" [" + rt + "]")
	}
	return strings.TrimSpace(b.String())
}

// gazelleDL 构造下载链；authkey 缺失（kerr!=nil 或空）则留空（点详情页降级）。
func gazelleDL(baseURL string, tid any, k gazelleAuthKey, kerr error) string {
	if kerr != nil || k.authkey == "" {
		return ""
	}
	return buildGazelleDownloadURL(baseURL, tid, k)
}

// gazelleClass 格式化 userstats.class（字符串或数字）为等级名；nil→""。
func gazelleClass(c any) string {
	switch v := c.(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// FetchUserInfo 两阶段：action=index（id/基础统计）+ action=user&id=N（joinDate/seeding/uploads，best-effort）。
func (g *GazelleJSONAPI) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {
	if def != nil && def.ID == "gazellegames" {
		return g.fetchGazelleGamesUserInfo(ctx, baseURL, cookie)
	}
	data, err := g.gazelleRequest(ctx, baseURL, cookie, "index", nil)
	if err != nil {
		return nil, err
	}
	var idx gazelleIndexResp
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse gazelle index: %w", err)
	}
	out := &site.UserInfo{
		ID:                  strconv.FormatInt(idx.ID, 10),
		Name:                idx.Username,
		Level:               gazelleClass(idx.Userstats.Class),
		Uploaded:            toInt64(idx.Userstats.Uploaded),
		Downloaded:          toInt64(idx.Userstats.Downloaded),
		Ratio:               toFloat64(idx.Userstats.Ratio),
		Bonus:               toFloat64(idx.Userstats.BonusPoints),
		BonusPerHour:        toFloat64(idx.Userstats.BonusPointsPerHour),
		SeedingBonusPerHour: toFloat64(idx.Userstats.SeedingBonusPointsPerHour),
		Seeding:             int(toInt64(idx.Userstats.SeedingCount)),
		Leeching:            int(toInt64(idx.Userstats.LeechingCount)),
		SeedingSize:         toInt64(idx.Userstats.SeedingSize),
	}
	if out.BonusPerHour == 0 {
		out.BonusPerHour = toFloat64(idx.Userstats.SeedingBonusPointsPerHour)
	}
	if idx.Userstats.JoinedDate != "" {
		out.JoinDate = idx.Userstats.JoinedDate
	}
	if idx.Userstats.LastAccess != "" {
		out.LastAccessAt = idx.Userstats.LastAccess
	}
	if def != nil && len(def.UserInfo.Fields) > 0 {
		site.ParseJSONUserInfoFields(data, def.UserInfo.Fields, out)
	}
	if idx.ID > 0 {
		if udata, e := g.gazelleRequest(ctx, baseURL, cookie, "user",
			map[string]string{"id": strconv.FormatInt(idx.ID, 10)}); e == nil {
			var u gazelleUserResp
			if json.Unmarshal(udata, &u) == nil {
				if out.JoinDate == "" && u.Stats.JoinedDate != "" {
					out.JoinDate = u.Stats.JoinedDate
				}
				if out.LastAccessAt == "" && u.Stats.LastAccess != "" {
					out.LastAccessAt = u.Stats.LastAccess
				}
				if s := toInt64(u.Community.Seeding); out.Seeding == 0 && s > 0 {
					out.Seeding = int(s)
				}
				if n := toInt64(u.Community.Uploaded); n > 0 {
					out.Uploads = int(n)
				}
			}
		}
	}
	applyStaticUserInfoFields(def, out)
	if def != nil && len(def.UserInfo.Pages) > 0 {
		if err := fetchUserInfoPages(ctx, &BaseHTMLAdapter{hc: g.hc}, def, baseURL, cookie, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func applyStaticUserInfoFields(def *site.SiteMetadata, u *site.UserInfo) {
	if def == nil || u == nil || len(def.UserInfo.Fields) == 0 {
		return
	}
	fields := map[string]site.ElementQuery{}
	for key, q := range def.UserInfo.Fields {
		if strings.TrimSpace(q.Text) != "" && q.Selector == "" && len(q.Selectors) == 0 && q.Data == "" {
			fields[key] = q
		}
	}
	if len(fields) == 0 {
		return
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<html></html>"))
	if err != nil {
		return
	}
	site.ParseUserInfoFields(doc, fields, u)
}

func (g *GazelleJSONAPI) fetchGazelleGamesUserInfo(ctx context.Context, baseURL, token string) (*site.UserInfo, error) {
	data, err := g.gazelleGamesRequest(ctx, baseURL, token, "quick_user", nil)
	if err != nil {
		return nil, err
	}
	var idx struct {
		Username  string `json:"username"`
		ID        int64  `json:"id"`
		Authkey   string `json:"authkey"`
		Passkey   string `json:"passkey"`
		Userstats struct {
			Uploaded   any `json:"uploaded"`
			Downloaded any `json:"downloaded"`
			Ratio      any `json:"ratio"`
			Class      any `json:"class"`
		} `json:"userstats"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse gazellegames quick_user: %w", err)
	}
	if idx.Authkey != "" || idx.Passkey != "" {
		g.keys.Store(baseURL+"|gazellegames", gazelleAuthKey{authkey: idx.Authkey, passkey: idx.Passkey})
	}
	out := &site.UserInfo{
		ID:         strconv.FormatInt(idx.ID, 10),
		Name:       idx.Username,
		Level:      gazelleClass(idx.Userstats.Class),
		Uploaded:   toInt64(idx.Userstats.Uploaded),
		Downloaded: toInt64(idx.Userstats.Downloaded),
		Ratio:      toFloat64(idx.Userstats.Ratio),
	}
	if idx.ID > 0 {
		if udata, e := g.gazelleGamesRequest(ctx, baseURL, token, "user", map[string]string{"id": out.ID}); e == nil {
			var u struct {
				Stats struct {
					JoinedDate string `json:"joinedDate"`
					Gold       any    `json:"gold"`
				} `json:"stats"`
				Community struct {
					Seeding    any `json:"seeding"`
					Uploaded   any `json:"uploaded"`
					HourlyGold any `json:"hourlyGold"`
					SeedSize   any `json:"seedSize"`
				} `json:"community"`
			}
			if json.Unmarshal(udata, &u) == nil {
				out.JoinDate = u.Stats.JoinedDate
				out.Seeding = int(toInt64(u.Community.Seeding))
				out.Uploads = int(toInt64(u.Community.Uploaded))
				out.Bonus = toFloat64(u.Stats.Gold)
				out.BonusPerHour = toFloat64(u.Community.HourlyGold)
				out.SeedingSize = toInt64(u.Community.SeedSize)
			}
		}
	}
	return out, nil
}
