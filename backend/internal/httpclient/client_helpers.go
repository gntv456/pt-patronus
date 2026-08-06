package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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
