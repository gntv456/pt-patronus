package schemas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Avistaz 是 Avistaz 网络适配器（avistaz/cinemaz/exoticaz/privatehd）。
// from-scratch JWT token API：POST /api/v1/jackett/auth（username+password+pid）→ Bearer token，
// GET /api/v1/jackett/torrents。UserInfo 按站规禁用（PT-depiler passParse），只返 {Name: username}。
// 凭据由 chain.credential() 打包成 JSON {username,password,pid} 单串传入（cookie 参数位）。
// 契约来源：PT-depiler AvistazNetwork.ts。
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
	var m map[string]string
	if err := json.Unmarshal([]byte(cred), &m); err == nil {
		return m["username"], m["password"], m["pid"]
	}
	return "", "", ""
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

	def = site.EnsureDefaults(def)
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

// FetchUserInfo 按站规禁用用户数据，只返 {Name: username}（PT-depiler passParse 状态）。
// 绝不报错、绝不访问 /profile。
func (a *Avistaz) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cred string) (*site.UserInfo, error) {
	u, _, _ := parseAvistazCred(cred)
	return &site.UserInfo{Name: u}, nil
}
