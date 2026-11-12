// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Avistaz 是 Avistaz 网络适配器（avistaz/cinemaz/exoticaz/privatehd）。
// from-scratch JWT token API：POST /api/v1/jackett/auth（username+password+pid）→ Bearer token，
// GET /api/v1/jackett/torrents。UserInfo 按站规禁用（按站规禁用），只返 {Name: username}。
// 凭据由 chain.credential() 打包成 JSON {username,password,pid} 单串传入（cookie 参数位）。
type Avistaz struct {
	hc   *httpclient.Client
	keys sync.Map // baseURL → avistazToken
}

type avistazToken struct {
	jwt    string
	expiry time.Time
}

// NewAvistaz 构造适配器。
func NewAvistaz(hc *httpclient.Client) *Avistaz { return &Avistaz{hc: hc} }

func (a *Avistaz) Schema() string { return "avistaz" }
func (a *Avistaz) Name() string   { return "site.avistaz" }
func (a *Avistaz) Priority() int  { return 100 }

type avistazAuthResp struct {
	Token  string `json:"token"`
	Expiry any    `json:"expiry"` // 相对秒（int/float/string）
}

type avistazTorrent struct {
	ID        any            `json:"id"`
	FileName  string         `json:"file_name"`
	FileSize  any            `json:"file_size"`
	URL       string         `json:"url"`
	Download  string         `json:"download"`
	Category  map[string]any `json:"category"` // {id: name}
	CreatedAt string         `json:"created_at"`
	Seed      any            `json:"seed"`
	Leech     any            `json:"leech"`
	Completed any            `json:"completed"`
}

// errAvistazRelogin sentinel：torrents API 401/403 → 调用方失效缓存+重 auth+重试一次。
var errAvistazRelogin = errors.New("avistaz: token expired, re-auth needed")

// parseAvistazCred 拆解 credential() 打包的 JSON {username,password,pid}。malformed → 三空（auth 干净失败）。
func parseAvistazCred(cred string) (username, password, pid string) {
	username, password, pid, _ = parseAvistazCredFull(cred)
	return username, password, pid
}

func parseAvistazCredFull(cred string) (username, password, pid, cookie string) {
	var m map[string]string
	if err := json.Unmarshal([]byte(cred), &m); err == nil {
		return m["username"], m["password"], m["pid"], m["cookie"]
	}
	return "", "", "", ""
}

// authRequest POST /api/v1/jackett/auth（form-encoded username+password+pid）→ 缓存 token。
func (a *Avistaz) authRequest(ctx context.Context, baseURL, cred string) (avistazToken, error) {
	u, p, pid := parseAvistazCred(cred)
	form := url.Values{}
	form.Set("username", u)
	form.Set("password", p)
	form.Set("pid", pid)
	resp, err := a.hc.Do(ctx, "POST", joinURL(baseURL, "/api/v1/jackett/auth"),
		[]byte(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		"", true)
	if err != nil {
		return avistazToken{}, err
	}
	if resp.IsCloudflare() {
		return avistazToken{}, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return avistazToken{}, site.ErrNotLoggedIn
	}
	var ar avistazAuthResp
	if err := json.Unmarshal(resp.Body, &ar); err != nil {
		return avistazToken{}, fmt.Errorf("parse avistaz auth: %w", err)
	}
	if ar.Token == "" {
		return avistazToken{}, site.ErrNotLoggedIn
	}
	return avistazToken{jwt: ar.Token, expiry: time.Now().Add(time.Duration(toInt64(ar.Expiry)) * time.Second)}, nil
}

// getToken 缓存+懒加载（sync.Map，key=baseURL）。过期/缺失则 authRequest。
func (a *Avistaz) getToken(ctx context.Context, baseURL, cred string) (string, error) {
	if v, ok := a.keys.Load(baseURL); ok {
		t := v.(avistazToken)
		if t.jwt != "" && time.Now().Before(t.expiry) {
			return t.jwt, nil
		}
	}
	t, err := a.authRequest(ctx, baseURL, cred)
	if err != nil {
		return "", err
	}
	a.keys.Store(baseURL, t)
	return t.jwt, nil
}

// torrentRequest GET /api/v1/jackett/torrents（Bearer）。401/403→errAvistazRelogin；
// 400/404/422→nil body 不报错（空结果）。
func (a *Avistaz) torrentRequest(ctx context.Context, baseURL, jwt, params string) ([]byte, error) {
	target := joinURL(baseURL, "/api/v1/jackett/torrents") + params
	resp, err := a.hc.Do(ctx, "GET", target, nil,
		map[string]string{"Authorization": "Bearer " + jwt, "Accept": "application/json"},
		"", true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	switch resp.StatusCode {
	case 401, 403:
		return nil, errAvistazRelogin
	case 400, 404, 422:
		return nil, nil // 空结果
	}
	return resp.Body, nil
}

// Search getToken → torrentRequest（401 重试一次）→ 解析 data[]。
func (a *Avistaz) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cred, keyword, category string, page int) ([]site.Torrent, error) {

	if page < 1 {
		page = 1
	}
	q := url.Values{}
	q.Set("in", "1")
	q.Set("limit", "50")
	if kw := strings.TrimSpace(keyword); kw != "" {
		q.Set("search", kw)
	}
	q.Set("page", strconv.Itoa(page))
	params := "?" + q.Encode()

	jwt, err := a.getToken(ctx, baseURL, cred)
	if err != nil {
		return nil, err
	}
	body, err := a.torrentRequest(ctx, baseURL, jwt, params)
	if err != nil {
		if errors.Is(err, errAvistazRelogin) { // 失效+重 auth+重试一次
			a.keys.Delete(baseURL)
			jwt, err = a.getToken(ctx, baseURL, cred)
			if err != nil {
				return nil, err
			}
			body, err = a.torrentRequest(ctx, baseURL, jwt, params)
		}
		if err != nil {
			return nil, err
		}
	}
	if body == nil { // 400/404/422 → 空
		return nil, nil
	}
	var br struct {
		Data []avistazTorrent `json:"data"`
	}
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf("parse avistaz browse: %w", err)
	}
	base, _ := url.Parse(baseURL)
	out := make([]site.Torrent, 0, len(br.Data))
	for _, it := range br.Data {
		out = append(out, site.Torrent{
			ID:          fmt.Sprint(it.ID),
			Title:       it.FileName,
			Size:        formatMTBytes(toInt64(it.FileSize)),
			Seeders:     int(toInt64(it.Seed)),
			Leechers:    int(toInt64(it.Leech)),
			Completed:   int(toInt64(it.Completed)),
			Time:        it.CreatedAt,
			Category:    firstMapValue(it.Category),
			DownloadURL: site.ResolveURL(base, it.Download),
			DetailURL:   site.ResolveURL(base, it.URL),
		})
	}
	return out, nil
}

// firstMapValue 取 map 首个值（avistaz category 是 {id:name}，取首值作分类名）。
func firstMapValue(m map[string]any) string {
	for _, v := range m {
		return fmt.Sprint(v)
	}
	return ""
}

// FetchUserInfo 按站规禁用用户数据，只返 {Name: username}（按站规禁用）。
// 绝不报错、绝不访问 /profile。
func (a *Avistaz) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cred string) (*site.UserInfo, error) {
	if def != nil && def.ID == "animez" {
		return fetchAnimeZUserInfo(ctx, a.hc, baseURL, cred)
	}
	u, _, _, _ := parseAvistazCredFull(cred)
	return &site.UserInfo{Name: u}, nil
}

func fetchAnimeZUserInfo(ctx context.Context, hc *httpclient.Client, baseURL, cred string) (*site.UserInfo, error) {
	username, _, _, cookie := parseAvistazCredFull(cred)
	out := &site.UserInfo{Name: username}
	if username == "" || cookie == "" {
		return out, nil
	}
	profilePath := "/profile/" + url.PathEscape(username)
	resp, err := hc.Get(ctx, joinURL(baseURL, profilePath), cookie, true)
	if err != nil || resp == nil || resp.IsCloudflare() || resp.StatusCode == 401 || resp.StatusCode == 403 {
		return out, nil
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return out, nil
	}
	parseAnimeZProfile(doc, out)
	if out.Name == "" {
		out.Name = username
	}
	if out.SeedingSize == 0 {
		out.SeedingSize = fetchAnimeZSeedingSize(ctx, hc, baseURL, profilePath, cookie)
	}
	return out, nil
}

func parseAnimeZProfile(doc *goquery.Document, out *site.UserInfo) {
	if doc == nil || out == nil {
		return
	}
	header := doc.Find("main > div > div > div:first-child > div > div > div > div").First()
	if name := strings.TrimSpace(header.Children().First().Text()); name != "" {
		out.Name = normalizeSpace(name)
	} else if title := strings.TrimSpace(doc.Find("title").First().Text()); title != "" {
		out.Name = normalizeSpace(strings.Split(title, " - ")[0])
	}
	if level := normalizeSpace(header.Find("span.badge.badge-sm").First().Text()); level != "" {
		out.Level = level
	}
	out.Uploaded = site.ParseBytes(animeZDatagridValue(doc, "Uploaded"))
	out.Downloaded = site.ParseBytes(animeZDatagridValue(doc, "Downloaded"))
	out.Ratio = animeZRatio(animeZDatagridValue(doc, "Ratio"))
	out.Bonus = site.ToFloat64(animeZDatagridValue(doc, "Bonus Points"))
	out.Uploads = site.ToInt(animeZDatagridValue(doc, "Uploads"))
	out.Seeding = site.ToInt(animeZDatagridValue(doc, "Seeding"))
	out.Leeching = site.ToInt(animeZDatagridValue(doc, "Leeching"))
	out.JoinDate = parseAnimeZDate(animeZDatagridValue(doc, "Member Since"))
}

func animeZDatagridValue(doc *goquery.Document, label string) string {
	var got string
	doc.Find(".datagrid-item").EachWithBreak(func(_ int, item *goquery.Selection) bool {
		title := normalizeSpace(item.Find(".datagrid-title").First().Text())
		if strings.EqualFold(title, label) {
			got = normalizeSpace(item.Find(".datagrid-content").First().Text())
			return false
		}
		return true
	})
	return got
}

func animeZRatio(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if strings.EqualFold(strings.TrimSuffix(raw, "."), "inf") {
		return 0
	}
	return site.ToFloat64(raw)
}

func parseAnimeZDate(raw string) string {
	raw = normalizeSpace(raw)
	for _, layout := range []string{"January 02, 2006", "January 2, 2006", "Jan 02, 2006", "Jan 2, 2006"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return site.ApplyFilters(raw, []site.Filter{{Name: "parse_time"}})
}

func fetchAnimeZSeedingSize(ctx context.Context, hc *httpclient.Client, baseURL, profilePath, cookie string) int64 {
	activePath := strings.TrimRight(profilePath, "/") + "/active"
	total, pages := fetchAnimeZActivePage(ctx, hc, baseURL, activePath, cookie)
	if pages > 100 {
		pages = 100
	}
	for page := 2; page <= pages; page++ {
		size, _ := fetchAnimeZActivePage(ctx, hc, baseURL, activePath+"?page="+strconv.Itoa(page), cookie)
		total += size
	}
	return total
}

func fetchAnimeZActivePage(ctx context.Context, hc *httpclient.Client, baseURL, path, cookie string) (int64, int) {
	resp, err := hc.Get(ctx, joinURL(baseURL, path), cookie, true)
	if err != nil || resp == nil || resp.IsCloudflare() || resp.StatusCode >= 400 {
		return 0, 1
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return 0, 1
	}
	return animeZSeedingSize(doc), animeZActivePageCount(doc)
}

func animeZActivePageCount(doc *goquery.Document) int {
	maxPage := 1
	doc.Find(`a[href*="page="]`).Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if u, err := url.Parse(href); err == nil {
			if n, err := strconv.Atoi(u.Query().Get("page")); err == nil && n > maxPage {
				maxPage = n
			}
		}
	})
	return maxPage
}

func animeZSeedingSize(doc *goquery.Document) int64 {
	var total int64
	doc.Find("table tr").Each(func(_ int, row *goquery.Selection) {
		if !animeZIsSeedingRow(row) {
			return
		}
		for _, sel := range []string{
			"span.badge-extra.fa-database",
			"div.d-block span.text-yellow",
			"td[data-label='Size']",
			".torrent-size",
			".text-yellow",
		} {
			if size := site.ParseBytes(row.Find(sel).First().Text()); size > 0 {
				total += size
				return
			}
		}
		total += site.ParseBytes(row.Text())
	})
	return total
}

func animeZIsSeedingRow(row *goquery.Selection) bool {
	found := false
	row.Children().EachWithBreak(func(_ int, cell *goquery.Selection) bool {
		text := strings.ToLower(normalizeSpace(cell.Text()))
		if text == "seed" || text == "seeding" {
			found = true
			return false
		}
		return true
	})
	return found
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
