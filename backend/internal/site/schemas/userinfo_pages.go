package schemas

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func fetchUserInfoPages(ctx context.Context, a *BaseHTMLAdapter, def *site.SiteMetadata, baseURL, cookie string, u *site.UserInfo) error {
	if a == nil || def == nil || u == nil || len(def.UserInfo.Pages) == 0 {
		return nil
	}
	for _, page := range def.UserInfo.Pages {
		path, ok := renderUserInfoPath(page.Path, page.Params, u)
		if !ok || path == "" {
			continue
		}
		resp, err := fetchUserInfoPage(ctx, a, baseURL, path, cookie, page)
		if err != nil {
			continue
		}
		if resp.IsCloudflare() {
			return fmt.Errorf("blocked by cloudflare on %s", baseURL)
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return site.ErrNotLoggedIn
		}
		fields := userInfoPageFields(def, page)
		if len(fields) == 0 {
			continue
		}
		format := strings.ToLower(strings.TrimSpace(page.Format))
		if format == "json" || (format != "html" && site.LooksLikeJSON(resp.Body)) {
			site.ParseJSONUserInfoFields(resp.Body, fields, u)
			continue
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
		if err != nil {
			continue
		}
		if loggedOut(resp, doc, def.NoLogin) {
			return site.ErrNotLoggedIn
		}
		site.ParseUserInfoFields(doc, fields, u)
	}
	return nil
}

func fetchUserInfoPage(ctx context.Context, a *BaseHTMLAdapter, baseURL, path, cookie string, page site.UserInfoPageDef) (*httpclient.Response, error) {
	method := strings.ToUpper(strings.TrimSpace(page.Method))
	if method == "" || method == http.MethodGet {
		if len(page.Headers) > 0 {
			return a.hc.GetWithHeaders(ctx, joinURL(baseURL, path), cookie, page.Headers, true)
		}
		return a.hc.Get(ctx, joinURL(baseURL, path), cookie, true)
	}
	headers := map[string]string{}
	for k, v := range page.Headers {
		headers[k] = v
	}
	body := []byte(page.Body)
	if len(body) > 0 {
		headers["Content-Type"] = "application/json"
	}
	return a.hc.Do(ctx, method, joinURL(baseURL, path), body, headers, cookie, true)
}

func renderUserInfoPath(path string, params map[string]string, u *site.UserInfo) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	var ok bool
	path, ok = renderUserInfoVars(path, u)
	if !ok {
		return "", false
	}
	if len(params) == 0 {
		return path, true
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return "", false
	}
	q := parsed.Query()
	for k, v := range params {
		rv, ok := renderUserInfoVars(v, u)
		if !ok {
			return "", false
		}
		q.Set(k, rv)
	}
	parsed.RawQuery = q.Encode()
	return parsed.String(), true
}

func renderUserInfoVars(s string, u *site.UserInfo) (string, bool) {
	repl := map[string]string{
		"{id}":   u.ID,
		"$id$":   u.ID,
		"{uid}":  u.ID,
		"$uid$":  u.ID,
		"{name}": u.Name,
		"$name$": u.Name,
	}
	for key, value := range repl {
		if strings.Contains(s, key) {
			if value == "" {
				return "", false
			}
			s = strings.ReplaceAll(s, key, url.QueryEscape(value))
		}
	}
	return s, true
}

func userInfoPageFields(def *site.SiteMetadata, page site.UserInfoPageDef) map[string]site.ElementQuery {
	out := map[string]site.ElementQuery{}
	for _, key := range page.UseFields {
		if q, ok := page.Fields[key]; ok {
			out[key] = q
			continue
		}
		if q, ok := def.UserInfo.Fields[key]; ok {
			out[key] = q
			continue
		}
		if q, ok := def.UserInfo.DetailFields[key]; ok {
			out[key] = q
		}
	}
	for key, q := range page.Fields {
		out[key] = q
	}
	return out
}
