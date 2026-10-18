package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// GenericJSON 是通用 JSON-API 适配器，服务命名 JSON schema（tnode/yemapt/f3nix/tbdev）。
// schema 参数化实例。选择器（JSON 点分路径）由 definition 驱动，走 site.ParseJSONSearch。
// cookie 鉴权（bearer-token 站需 per-schema credential() 扩展，留 M6）。
type GenericJSON struct {
	hc     *httpclient.Client
	schema string
}

func NewGenericJSON(hc *httpclient.Client, schema string) *GenericJSON {
	return &GenericJSON{hc: hc, schema: schema}
}

func (g *GenericJSON) Schema() string { return g.schema }
func (g *GenericJSON) Name() string   { return "site.generic_json." + g.schema }
func (g *GenericJSON) Priority() int  { return 200 }

// Search GET 搜索端点（BuildJSONSearchURL 处理路径占位/query 参数）→ ParseJSONSearch。
func (g *GenericJSON) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	def = site.EnsureDefaults(def)
	if def.ID == "sunnypt" {
		return g.SearchSunnypt(ctx, def, baseURL, cookie, keyword, category, page)
	}
	if def.ID == "yemapt" || g.schema == "yemapt" {
		return g.searchYemaPT(ctx, def, baseURL, cookie, keyword, page)
	}
	if def.ID == "beyondhd" || g.schema == "f3nix" {
		return g.searchF3NIX(ctx, def, baseURL, cookie, keyword, page)
	}
	if def.ID == "fsm" || g.schema == "fsm" {
		return g.searchFSM(ctx, def, baseURL, cookie, keyword, page)
	}
	target := site.BuildJSONSearchURL(baseURL, def, keyword, page)
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
	return site.ParseJSONSearch(resp.Body, def, baseURL)
}

func (g *GenericJSON) searchFSM(ctx context.Context, def *site.SiteMetadata, baseURL, credRaw, keyword string, page int) ([]site.Torrent, error) {
	cred := parseFSMCredential(credRaw)
	if cred.Token == "" {
		return nil, fmt.Errorf("fsm api token required")
	}
	target := site.BuildJSONSearchURL(fsmAPIBaseURL(baseURL), def, keyword, page)
	resp, err := g.hc.GetWithHeaders(ctx, target, cred.Cookie, map[string]string{
		"APITOKEN": cred.Token,
		"Accept":   "application/json",
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
	var probe struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(resp.Body, &probe); err == nil && !probe.Success {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseJSONSearch(resp.Body, def, baseURL)
}

func (g *GenericJSON) searchF3NIX(ctx context.Context, def *site.SiteMetadata, baseURL, credRaw, keyword string, page int) ([]site.Torrent, error) {
	cred := parseF3NIXCredential(credRaw)
	if cred.APIKey == "" {
		return nil, fmt.Errorf("f3nix api key required")
	}
	if page < 1 {
		page = 1
	}
	body, _ := json.Marshal(map[string]any{
		"action": "search",
		"search": keyword,
		"rsskey": cred.RSSKey,
		"page":   page,
	})
	target := joinURL(baseURL, strings.TrimRight(def.SearchEntry, "/")+"/"+url.PathEscape(cred.APIKey))
	resp, err := g.hc.Do(ctx, http.MethodPost, target, body, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, cred.Cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	var probe struct {
		Success    bool `json:"success"`
		StatusCode int  `json:"status_code"`
	}
	if err := json.Unmarshal(resp.Body, &probe); err == nil && (!probe.Success || probe.StatusCode != 1) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseJSONSearch(resp.Body, def, baseURL)
}

func (g *GenericJSON) searchYemaPT(ctx context.Context, def *site.SiteMetadata, baseURL, cookie, keyword string, page int) ([]site.Torrent, error) {
	if page < 1 {
		page = 1
	}
	body, _ := json.Marshal(map[string]any{
		"keyword": keyword,
		"pageParam": map[string]any{
			"current":  page,
			"pageSize": 40,
			"total":    1000,
		},
		"sorter": map[string]string{
			"order": "descend",
			"field": "listingTime",
		},
	})
	target := joinURL(baseURL, def.SearchEntry)
	resp, err := g.hc.Do(ctx, "POST", target, body, withYemaPTAuth(map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, cookie), cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseJSONSearch(resp.Body, def, baseURL)
}

// FetchUserInfo JSON 站 YAML 普遍无 user_info → stub（不报错）。
func (g *GenericJSON) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {
	def = site.EnsureDefaults(def)
	if def.ID == "sunnypt" {
		return g.FetchSunnyptUserInfo(ctx, def, baseURL, cookie)
	}
	if def.ID == "beyondhd" || g.schema == "f3nix" {
		cookie = parseF3NIXCredential(cookie).Cookie
	}
	var fsmCred fsmCredential
	if def.ID == "fsm" || g.schema == "fsm" {
		fsmCred = parseFSMCredential(cookie)
		cookie = fsmCred.Cookie
		baseURL = fsmAPIBaseURL(baseURL)
	}
	u := &site.UserInfo{}
	for _, page := range def.UserInfo.Pages {
		path, ok := renderUserInfoPath(page.Path, page.Params, u)
		if !ok || path == "" {
			continue
		}
		pageReq := page
		if def.ID == "yemapt" || g.schema == "yemapt" {
			pageReq.Headers = withYemaPTAuth(page.Headers, cookie)
		}
		if def.ID == "fsm" || g.schema == "fsm" {
			pageReq.Headers = withFSMAuth(page.Headers, fsmCred.Token)
		}
		resp, err := fetchUserInfoPage(ctx, &BaseHTMLAdapter{hc: g.hc}, baseURL, path, cookie, pageReq)
		if err != nil {
			continue
		}
		if resp.IsCloudflare() {
			return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, site.ErrNotLoggedIn
		}
		fields := userInfoPageFields(def, page)
		if len(fields) == 0 {
			continue
		}
		format := strings.ToLower(strings.TrimSpace(page.Format))
		if format == "html" || (format != "json" && !site.LooksLikeJSON(resp.Body)) {
			doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
			if err != nil {
				continue
			}
			site.ParseUserInfoFields(doc, fields, u)
			continue
		}
		site.ParseJSONUserInfoFields(resp.Body, fields, u)
	}
	return u, nil
}

type fsmCredential struct {
	Token   string
	Passkey string
	Cookie  string
}

func parseFSMCredential(raw string) fsmCredential {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fsmCredential{}
	}
	if strings.HasPrefix(raw, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			return fsmCredential{
				Token:   firstNonEmptyF3NIX(m["token"], m["api_token"], m["apiKey"], m["api_key"], m["apikey"]),
				Passkey: firstNonEmptyF3NIX(m["passkey"], m["rsskey"]),
				Cookie:  m["cookie"],
			}
		}
	}
	return fsmCredential{Token: raw, Cookie: raw}
}

func withFSMAuth(headers map[string]string, token string) map[string]string {
	out := map[string]string{}
	for k, v := range headers {
		out[k] = v
	}
	if token != "" {
		out["APITOKEN"] = token
	}
	out["Accept"] = "application/json"
	return out
}

func fsmAPIBaseURL(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return baseURL
	}
	host := u.Hostname()
	if strings.HasPrefix(host, "api.") || host == "localhost" || net.ParseIP(host) != nil {
		return strings.TrimRight(baseURL, "/")
	}
	port := u.Port()
	u.Host = "api." + host
	if port != "" {
		u.Host += ":" + port
	}
	return strings.TrimRight(u.String(), "/")
}

type f3nixCredential struct {
	APIKey string
	RSSKey string
	Cookie string
}

func parseF3NIXCredential(raw string) f3nixCredential {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return f3nixCredential{}
	}
	if strings.HasPrefix(raw, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			return f3nixCredential{
				APIKey: firstNonEmptyF3NIX(m["api_key"], m["apikey"], m["apiKey"]),
				RSSKey: firstNonEmptyF3NIX(m["rss_key"], m["rsskey"], m["rssKey"], m["passkey"]),
				Cookie: m["cookie"],
			}
		}
	}
	return f3nixCredential{APIKey: raw, Cookie: raw}
}

func firstNonEmptyF3NIX(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func withYemaPTAuth(headers map[string]string, token string) map[string]string {
	out := map[string]string{}
	for k, v := range headers {
		out[k] = v
	}
	token = strings.TrimSpace(token)
	if token != "" {
		out["Authorization"] = token
	}
	return out
}
