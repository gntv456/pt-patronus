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
	"sync"
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
	// AllowPrivate 放行内网/环回目标（本地测试或内网下载器）；默认 false——站点抓取必须 SSRF 防护。
	AllowPrivate bool
	// UTLS 启用 uTLS（Chrome ClientHello）拨号，绕过按 TLS 指纹拦截非浏览器流量的 WAF/CDN
	// （如 SafeLine 雷池对签到端点的 468 拦截）。仅对需要的站点 opt-in；默认 false 用标准 TLS。
	UTLS bool
	// MaxBodyBytes 响应体上限；0 用 defaultMaxBodyBytes(32MB)。超额返回错误而非截断数据，防 OOM。
	MaxBodyBytes int64
	// DialContext 可选自定义拨号（仅测试）：接管拨号绕过 SSRF 守卫与 DNS，重定向到本地 testserver。
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
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
	if name != "gbk" && name != "gb2312" && name != "gb18030" {
		return data // 非 GBK 族原样返回
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

	// per-host 最小请求间隔限流（站点礼貌抓取，对标 MP 站点流控）。
	limMu   sync.Mutex
	limNext map[string]time.Time // host → 下一次允许请求的时间
	lims    map[string]time.Duration
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
		// SSRF 模式下解析主机并对每个 IP 做 IsPublicIP 校验再直连（拦截重定向到内网/元数据，
		// 消除「解析后校验、拨号时再解析」的 DNS 重绑定窗口）；非 SSRF 退化为普通拨号。
		DialContext: ssrfDialContext,
	}
	if opts.DialContext != nil { // 仅测试：完全接管拨号，绕过守卫与 DNS，重定向到本地 testserver。
		transport.DialContext = opts.DialContext
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
	return &Client{hc: hc, ua: opts.UserAgent, retries: opts.MaxRetries, allowPrivate: opts.AllowPrivate, maxBody: maxBody,
		limNext: map[string]time.Time{}, lims: map[string]time.Duration{}}, nil
}

// SetHostInterval 设置某 host 的最小请求间隔（站点抓取限流，LimitInterval 秒 → 此处）。
// host 传 URL 或纯域名均可（内部经 url.Parse 归一到 u.Host）。interval<=0 清除限流。
// 线程安全；未知 host 不受影响（默认不限）。
func (c *Client) SetHostInterval(host string, interval time.Duration) {
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Host
	}
	if host == "" {
		return
	}
	c.limMu.Lock()
	defer c.limMu.Unlock()
	if interval <= 0 {
		delete(c.lims, host)
		return
	}
	c.lims[host] = interval
}

// throttle 按 host 限流等待：距该 host 上次放行不足 interval 则补足睡眠。
func (c *Client) throttle(ctx context.Context, rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return
	}
	c.limMu.Lock()
	interval, ok := c.lims[u.Host]
	if !ok {
		c.limMu.Unlock()
		return
	}
	now := time.Now()
	next := c.limNext[u.Host]
	var wait time.Duration
	if next.After(now) {
		wait = next.Sub(now)
		c.limNext[u.Host] = next.Add(interval)
	} else {
		c.limNext[u.Host] = now.Add(interval)
	}
	c.limMu.Unlock()
	if wait <= 0 {
		return
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// Do 发起请求。ssrf=true 时阻止内网目标（站点抓取用）；cookieStr 形如 "k=v; k2=v2"。
func (c *Client) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*Response, error) {

	c.throttle(ctx, rawURL)
	if ssrf && !c.allowPrivate {
		if err := c.assertPublicWithRetry(rawURL); err != nil {
			// 区分两类失败：ErrBlocked=确定的 SSRF 策略拦截（私网/环回等）；其余为 DNS 解析失败/
			// 网络不可达（临时性）。前者标 ssrf，后者标 unreachable，避免把「站点域名解析不了」
			// 误报成 SSRF 拦截，误导排障（如 hhanclub.top/lemonhd.club 域名失效被显示为 ssrf）。
			if errors.Is(err, ErrBlocked) {
				return nil, fmt.Errorf("ssrf: %w", err)
			}
			return nil, fmt.Errorf("unreachable: %w", err)
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
	c.throttle(ctx, rawURL)
	if !c.allowPrivate {
		if err := c.assertPublicWithRetry(rawURL); err != nil {
			if errors.Is(err, ErrBlocked) {
				return nil, fmt.Errorf("ssrf: %w", err)
			}
			return nil, fmt.Errorf("unreachable: %w", err)
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
