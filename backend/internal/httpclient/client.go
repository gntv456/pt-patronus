// Package httpclient 是统一抓取 HTTP 客户端：cookie 管理、重试、UA、
// Cloudflare 检测、SSRF 防护、可配置超时/跳过 TLS 校验。
package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/text/transform"
)

const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Options 配置一个 Client。
type Options struct {
	Timeout        time.Duration
	UserAgent      string
	MaxRetries     int
	InsecureTLS    bool
	FollowRedirect bool
	// AllowPrivate 放行内网/环回目标（用于本地测试或内网下载器）；
	// 默认 false——站点抓取必须 SSRF 防护。
	AllowPrivate bool
}

// Response 是请求结果。
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	FinalURL   string
}

// IsCloudflare 启发式判定是否被 Cloudflare 拦截。
func (r *Response) IsCloudflare() bool {
	if r == nil {
		return false
	}
	if v := r.Header.Get("Server"); strings.Contains(strings.ToLower(v), "cloudflare") {
		return r.StatusCode == 403 || r.StatusCode == 503
	}
	body := strings.ToLower(string(r.Body))
	if r.StatusCode == 503 && (strings.Contains(body, "cf-chl") ||
		strings.Contains(body, "just a moment") ||
		strings.Contains(body, "cloudflare")) {
		return true
	}
	return false
}

// decodeBody 把响应体按声明的字符集转码为 UTF-8。NexusPHP 中文站常为 GBK/gb18030，
// 不转码则 goquery/strings 按字节(UTF-8)读取得到乱码，且中文关键字判定（如"成功"）
// 全部失效。仅对 GBK 族转码（gb18030 是 GBK/GB2312 超集）；UTF-8/未判定/二进制原样返回。
// 用 golang.org/x/net/html/charset 的 DetermineEncoding 综合 Content-Type 与 <meta charset>
// 判定，仅 certain=true 且属 GBK 族才转码——避免误伤 UTF-8 站与图片/种子等二进制。
func decodeBody(data []byte, contentType string) []byte {
	if len(data) == 0 {
		return data
	}
	e, name, certain := charset.DetermineEncoding(data, contentType)
	if e == nil || !certain {
		return data
	}
	switch name {
	case "gbk", "gb2312", "gb18030":
		// GBK 族，转码到 UTF-8。
	default:
		return data
	}
	decoded, err := io.ReadAll(transform.NewReader(bytes.NewReader(data), e.NewDecoder()))
	if err != nil || len(decoded) == 0 {
		return data // 转码失败则退回原始字节，宁可不译不可乱译
	}
	return decoded
}

// Client 封装 *http.Client + cookie jar + 重试 + SSRF 守卫。
type Client struct {
	hc           *http.Client
	ua           string
	retries      int
	allowPrivate bool
}

// New 构造一个 Client。
func New(opts Options) (*Client, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = defaultUA
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.InsecureTLS},
		Proxy:           http.ProxyFromEnvironment,
		// 自定义拨号：SSRF 模式下解析主机并对每个 IP 做 IsPublicIP 校验，再直连已校验的
		// IP。既拦截重定向到内网/元数据的后续跳（每跳都触发新拨号且复用同一 ctx），又消除
		// 「解析后校验、拨号时再解析」的 DNS 重绑定窗口。非 SSRF 调用退化为普通拨号。
		DialContext: ssrfDialContext,
	}
	hc := &http.Client{
		Timeout:   opts.Timeout,
		Jar:       jar,
		Transport: transport,
	}
	if !opts.FollowRedirect {
		hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 {
				return http.ErrUseLastResponse
			}
			return nil
		}
	}
	return &Client{hc: hc, ua: opts.UserAgent, retries: opts.MaxRetries, allowPrivate: opts.AllowPrivate}, nil
}

// Do 发起请求。ssrf=true 时阻止内网目标（站点抓取用）；cookieStr 形如 "k=v; k2=v2"。
func (c *Client) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*Response, error) {

	if ssrf && !c.allowPrivate {
		if err := AssertPublicURL(rawURL); err != nil {
			return nil, fmt.Errorf("ssrf: %w", err)
		}
		// 标记本次请求（含重定向后续跳）走 SSRF 拨号校验。
		ctx = context.WithValue(ctx, ssrfCtxKey{}, true)
	}
	if err := c.seedCookies(rawURL, cookieStr); err != nil {
		return nil, err
	}

	maxRetries := c.retries
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.hc.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				c.backoff(attempt)
				continue
			}
			return nil, err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				c.backoff(attempt)
				continue
			}
			return nil, err
		}
		out := &Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       decodeBody(data, resp.Header.Get("Content-Type")),
			FinalURL:   resp.Request.URL.String(),
		}
		// 429 / 5xx 重试
		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < maxRetries {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			c.backoff(attempt)
			continue
		}
		return out, nil
	}
	return nil, lastErr
}

// Get 是 Do 的便捷封装。
func (c *Client) Get(ctx context.Context, rawURL, cookieStr string, ssrf bool) (*Response, error) {
	return c.Do(ctx, http.MethodGet, rawURL, nil, nil, cookieStr, ssrf)
}

// Cookies 返回 jar 中 rawURL 的当前 cookie（形如 "k=v; k2=v2"）。
// 用于 cookie 保活：请求后服务端 Set-Cookie 会自动落入 jar，据此捕获轮换后的最新 cookie。
func (c *Client) Cookies(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	cookies := c.hc.Jar.Cookies(u)
	parts := make([]string, 0, len(cookies))
	for _, ck := range cookies {
		if ck.Name == "" {
			continue
		}
		parts = append(parts, ck.Name+"="+ck.Value)
	}
	return strings.Join(parts, "; "), nil
}

// PostForm 以 application/x-www-form-urlencoded 提交。
func (c *Client) PostForm(ctx context.Context, rawURL, cookieStr, formBody string, ssrf bool) (*Response, error) {
	return c.Do(ctx, http.MethodPost, rawURL, []byte(formBody),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, cookieStr, ssrf)
}

// PostFormAjax 同 PostForm，并附加 AJAX 来源头：X-Requested-With: XMLHttpRequest + Referer。
// 供 NexusPHP 等用 checkAjaxReferer() 校验来源的端点（魔力刮刮乐 / 勋章合成 / 财神刮刮乐）使用——
// 缺这两个头时站点返回 HTML 登录页或「安全验证失败」，而非 JSON。referer 一般传站点 baseURL。
func (c *Client) PostFormAjax(ctx context.Context, rawURL, cookieStr, formBody, referer string, ssrf bool) (*Response, error) {
	return c.PostFormAjaxWith(ctx, rawURL, cookieStr, formBody, referer, nil, ssrf)
}

// PostFormAjaxWith 同 PostFormAjax，并追加任意额外头（如勋章合成所需的 X-CSRF-TOKEN）。
func (c *Client) PostFormAjaxWith(ctx context.Context, rawURL, cookieStr, formBody, referer string, extra map[string]string, ssrf bool) (*Response, error) {
	headers := map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
	}
	if referer != "" {
		headers["Referer"] = referer
	}
	for k, v := range extra {
		headers[k] = v
	}
	return c.Do(ctx, http.MethodPost, rawURL, []byte(formBody), headers, cookieStr, ssrf)
}

// GetWithHeaders 发带额外头的 GET（如勋章合成取 csrf 页面需 Referer）。
func (c *Client) GetWithHeaders(ctx context.Context, rawURL, cookieStr string, headers map[string]string, ssrf bool) (*Response, error) {
	return c.Do(ctx, http.MethodGet, rawURL, nil, headers, cookieStr, ssrf)
}

// seedCookies 解析 "k=v; k2=v2" 并注入 cookie jar（作用于 rawURL 的 host）。
func (c *Client) seedCookies(rawURL, cookieStr string) error {
	if cookieStr == "" {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	var cookies []*http.Cookie
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:  strings.TrimSpace(kv[0]),
			Value: strings.TrimSpace(kv[1]),
		})
	}
	if len(cookies) > 0 {
		c.hc.Jar.SetCookies(u, cookies)
	}
	return nil
}

func (c *Client) backoff(attempt int) {
	// 指数退避：200ms, 400ms, 800ms … 上限 2s
	d := time.Duration(200<<attempt) * time.Millisecond
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	time.Sleep(d)
}

// ssrfCtxKey 标记请求需在拨号时做 SSRF 校验（由 Do 在 ssrf && !allowPrivate 时注入）。
type ssrfCtxKey struct{}

var zeroDialer = &net.Dialer{}

// ssrfDialContext 在 SSRF 模式下解析主机并对每个 IP 做 IsPublicIP 校验，再直连已校验的
// IP（避免二次解析导致的 DNS 重绑定）；非 SSRF 调用退化为普通拨号，放行内网（测试/下载器）。
// 每次重定向都触发新的拨号且复用同一 ctx，因此首跳与后续跳一视同仁地受校验。
func ssrfDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if v, _ := ctx.Value(ssrfCtxKey{}).(bool); !v {
		return zeroDialer.DialContext(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ipa := range ips {
		if !IsPublicIP(ipa.IP) {
			lastErr = fmt.Errorf("ssrf: blocked private/internal address %s", ipa.IP)
			continue
		}
		conn, err := zeroDialer.DialContext(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ssrf: no dialable public address for %s", host)
	}
	return nil, lastErr
}
