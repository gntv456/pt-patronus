// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func plHTTPRequest(ctx context.Context, method, rawURL string, body io.Reader, headers map[string]string, cookie string, timeout time.Duration) (int, http.Header, []byte, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "PTPatronus-Plugin/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if strings.TrimSpace(cookie) != "" {
		req.Header.Set("Cookie", cookie)
	}
	client := &http.Client{Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if err != nil {
		return res.StatusCode, res.Header, nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 400 {
		return res.StatusCode, res.Header, data, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return res.StatusCode, res.Header, data, nil
}

func plValidateHTTPURL(ctx context.Context, rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return errors.New("URL 无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("仅支持 HTTP/HTTPS")
	}
	if !allowPrivate {
		return pluginValidatePublicHost(ctx, u.Hostname())
	}
	return nil
}

func plValidateRouterHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if pluginIsPrivateIP(ip) || ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("已阻止公网路由器地址 %s；如确认需要请开启允许公网地址", host)
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		if !(pluginIsPrivateIP(addr.IP) || addr.IP.IsLoopback()) {
			return fmt.Errorf("已阻止解析到公网 IP %s；如确认需要请开启允许公网地址", addr.IP)
		}
	}
	return nil
}

func plJoinURL(baseURL, endpoint string) (string, error) {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = "/"
	}
	b, err := url.Parse(baseURL)
	if err != nil || b.Hostname() == "" {
		return "", errors.New("站点地址无效")
	}
	e, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(e).String(), nil
}

func plHTMLToText(html string) string {
	html = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<br\s*/?>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?is)</(tr|div|p|li|td)>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(html, " ")
	html = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'").Replace(html)
	return html
}

func plTopLines(text string, limit int) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out
}
