// Package httpclient 是统一抓取 HTTP 客户端：cookie 管理、重试、UA、
// Cloudflare 检测、SSRF 防护、可配置超时/跳过 TLS 校验。
package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
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

// defaultMaxBodyBytes 是 Do/PostWithCookie 读取响应体的默认上限（32MB）。
// 防止恶意/损坏的 RSS、webhook、CF 挑战页等返回超大响应体导致 OOM——RSS 是定时任务，
// 单条恶意源每个 cron 周期都会重复触发，无上限会拖垮整机。需要更大响应时用 Options.MaxBodyBytes 放宽。
const defaultMaxBodyBytes int64 = 32 << 20

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
	// UTLS 启用 uTLS（Chrome ClientHello）拨号，绕过按 TLS 指纹拦截非浏览器流量的 WAF/CDN
	// （如 SafeLine 雷池对签到端点的 468 拦截）。仅对需要的站点 opt-in；默认 false 用标准 TLS。
	UTLS bool
	// MaxBodyBytes 读取响应体的最大字节数；0 表示用 defaultMaxBodyBytes（32MB）。
	// 超额返回错误而非返回截断数据，防止超大响应体撑爆内存。
	MaxBodyBytes int64
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

// readCapped 读取最多 max 字节的响应体；超限返回错误（不返回截断数据）。
// LimitReader 读到 max+1 即止（不会读完整个超大响应），据此判定越限，防止 OOM。
func readCapped(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("response body exceeds %d bytes", max)
	}
	return b, nil
}

// Client 封装 *http.Client + cookie jar + 重试 + SSRF 守卫。
type Client struct {
	hc           *http.Client
	ua           string
	retries      int
	allowPrivate bool
	maxBody      int64
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
	if opts.UTLS {
		// uTLS（Chrome 指纹）拨号：仅 https 走 DialTLSContext 用 utls 握手，绕 TLS 指纹 WAF。
		// ALPN 强制 http/1.1（net/http 不能在自管 TLS 连接上说 h2）。见 utls_dial.go。
		transport.ForceAttemptHTTP2 = false
		transport.DialTLSContext = UTLSDialContext(opts.AllowPrivate, opts.InsecureTLS)
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
	maxBody := opts.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}
	return &Client{hc: hc, ua: opts.UserAgent, retries: opts.MaxRetries, allowPrivate: opts.AllowPrivate, maxBody: maxBody}, nil
}

// Do 发起请求。ssrf=true 时阻止内网目标（站点抓取用）；cookieStr 形如 "k=v; k2=v2"。
func (c *Client) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*Response, error) {

	if ssrf && !c.allowPrivate {
		if err := c.assertPublicWithRetry(rawURL); err != nil {
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
			// SSRF 拨号拒绝（私网/重定向到内网，ssrfDialContext 返回）是确定的拒绝，不重试。
			if errors.Is(err, ErrBlocked) {
				return nil, err
			}
			lastErr = err
			if attempt < maxRetries {
				c.backoff(attempt)
				continue
			}
			return nil, err
		}
		data, err := readCapped(resp.Body, c.maxBody)
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

// PostWithCookie 发 POST，cookie 以显式 Cookie 头发送，完全不经过 cookie jar。
//
// 专用于凭据校验（如好学 /api/verifyAccess.php）：必须只携带调用方本次提交的 cookie，
// 禁止复用共享 jar 中其他请求残留的 cookie。否则同一个共享 Client 上，先发生的真实
// 登录会把有效 cookie 种进 jar，之后任何人提交无效/空 cookie 的校验请求仍会带着残留
// 凭证打到目标站并通过 —— 构成认证绕过（越权登录为上一个被验证的用户）。
//
// 为此这里用 Jar=nil 的临时 http.Client（复用本 Client 的 transport 以保留 SSRF 拨号
// 守卫与连接池），并把 cookie 写入请求头；不带重试、不读写 jar。凭据校验目标始终是
// 公网站点，故默认启用 SSRF 守卫（AllowPrivate 的 client 例外，用于测试）。
func (c *Client) PostWithCookie(ctx context.Context, rawURL, cookieStr string) (*Response, error) {
	if !c.allowPrivate {
		if err := c.assertPublicWithRetry(rawURL); err != nil {
			return nil, fmt.Errorf("ssrf: %w", err)
		}
		ctx = context.WithValue(ctx, ssrfCtxKey{}, true)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if cookieStr != "" {
		req.Header.Set("Cookie", cookieStr) // 显式头，绝不读 jar
	}
	// Jar=nil：复用 transport（SSRF 拨号守卫 + 连接池），但不自动注入/持久化 cookie。
	noJar := &http.Client{Timeout: c.hc.Timeout, Transport: c.hc.Transport}
	resp, err := noJar.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := readCapped(resp.Body, c.maxBody)
	if err != nil {
		return nil, err
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       decodeBody(data, resp.Header.Get("Content-Type")),
		FinalURL:   resp.Request.URL.String(),
	}, nil
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

// maxPrecheckAttempts 是 SSRF 预检的重试总次数：1 次首检 + 2 次重试。
const maxPrecheckAttempts = 3

// retryAssert 重复执行 check，对其返回的临时性错误（非 [ErrBlocked]）按指数退避重试。
// 确定的 SSRF 拒绝（errors.Is(err, ErrBlocked)）立即返回、绝不重试。
//
// 抽出为独立方法以便测试注入 check 直接验证重试边界，不依赖真实 DNS。
func (c *Client) retryAssert(check func() error) error {
	var err error
	for attempt := 0; attempt < maxPrecheckAttempts; attempt++ {
		err = check()
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrBlocked) {
			return err // 确定的拒绝，立即返回
		}
		// 临时性解析错误，退避后重试。
		if attempt < maxPrecheckAttempts-1 {
			c.backoff(attempt)
		}
	}
	return err
}

// assertPublicWithRetry 校验目标为公网站点，对 DNS 解析超时 / NXDOMAIN / 网络瞬时错误做
// 有限重试；确定的 SSRF 拒绝（解析到私有地址等，errors.Is(err, ErrBlocked)）立即返回。
//
// 背景：原 Do/PostWithCookie 在请求重试循环【之外】做一次 AssertPublicURL 预检，DNS 一次
// 超时（容器内 Docker DNS 127.0.0.11 转发到不通的上游时极常见）即令整次站点抓取硬失败，
// 即便配了 MaxRetries 也不重试预检。这里把预检的重试从请求重试中分离：只对临时性解析
// 错误重试，安全拒绝一触即拒——既避免误杀正常抓取，也不给攻击者重试探针的机会。
func (c *Client) assertPublicWithRetry(rawURL string) error {
	return c.retryAssert(func() error { return AssertPublicURL(rawURL) })
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
			lastErr = fmt.Errorf("%w: %s", ErrBlocked, ipa.IP)
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
