package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type httpProbePlugin struct{}

func NewHTTPProbePlugin() Plugin { return httpProbePlugin{} }

func (httpProbePlugin) Manifest() Manifest {
	return Manifest{
		ID:          "http-probe",
		Name:        "HTTP 站点探测",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "探测站点或 API 的状态码、响应时间和内容片段，适合临时检查服务是否可用。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "probe",
				Label:       "探测 URL",
				Description: "发起一次 HTTP/HTTPS 请求，返回状态码、耗时、内容类型和响应片段。",
				InputSchema: []ConfigField{
					{Key: "url", Label: "URL", Type: "text", Required: true, Default: "https://example.com"},
					{Key: "method", Label: "方法", Type: "select", Default: http.MethodGet, Options: []ConfigOption{
						{Label: "GET", Value: http.MethodGet},
						{Label: "HEAD", Value: http.MethodHead},
						{Label: "POST", Value: http.MethodPost},
					}},
					{Key: "expected_status", Label: "期望状态码", Type: "number", Default: 200},
					{Key: "headers", Label: "请求头", Type: "textarea", Help: "每行一个 Header: Value，可留空。"},
					{Key: "body", Label: "请求体", Type: "textarea", Help: "POST 时使用，可留空。"},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "timeout_seconds", Label: "超时时间(秒)", Type: "number", Default: 10},
			{Key: "follow_redirects", Label: "跟随重定向", Type: "switch", Default: true},
			{Key: "allow_private_network", Label: "允许内网地址", Type: "switch", Default: false, Help: "默认阻止本机、内网和链路本地地址，避免误探测敏感服务。"},
			{Key: "max_body_bytes", Label: "响应片段字节", Type: "number", Default: 2048},
			{Key: "default_headers", Label: "默认请求头", Type: "textarea", Default: "User-Agent: PTPatronus-PluginProbe/1.0"},
		},
	}
}

func (httpProbePlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"timeout_seconds":       10,
		"follow_redirects":      true,
		"allow_private_network": false,
		"max_body_bytes":        2048,
		"default_headers":       "User-Agent: PTPatronus-PluginProbe/1.0",
	}
}

func (httpProbePlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (httpProbePlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	if action != "probe" {
		return nil, fmt.Errorf("unknown action %s", action)
	}
	cfg := rt.Config()
	rawURL := strings.TrimSpace(pluginString(input["url"]))
	if rawURL == "" {
		return nil, errors.New("URL 不能为空")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("解析 URL 失败: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("仅支持 HTTP/HTTPS URL")
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("URL 缺少主机名")
	}
	if !asBool(cfg["allow_private_network"]) {
		// 预检提供早期清晰错误（主机名非法/字面内网 IP）；实际连接由 pluginSSRFDialContext
		// 在拨号时原子「解析→校验→直连」，消除预检与连接间的 DNS 重绑定窗口（TOCTOU）。
		if err := pluginValidatePublicHost(ctx, parsed.Hostname()); err != nil {
			return nil, err
		}
	}
	method := strings.ToUpper(strings.TrimSpace(pluginString(input["method"])))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
		return nil, errors.New("仅支持 GET、HEAD、POST")
	}
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(pluginString(input["body"]))
	}
	timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 10)) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	maxBody := pluginInt(cfg["max_body_bytes"], 2048)
	if maxBody <= 0 || maxBody > 65536 {
		maxBody = 2048
	}
	client := &http.Client{Timeout: timeout}
	if !asBool(cfg["follow_redirects"]) {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	// 公网模式下用拨号时校验的 transport，把 SSRF 校验绑定到实际连接（消除 DNS 重绑定）；
	// allow_private_network 模式放行内网，沿用默认 transport。
	if !asBool(cfg["allow_private_network"]) {
		client.Transport = &http.Transport{DialContext: pluginSSRFDialContext}
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return nil, err
	}
	for k, v := range pluginParseHeaders(pluginString(cfg["default_headers"])) {
		req.Header.Set(k, v)
	}
	for k, v := range pluginParseHeaders(pluginString(input["headers"])) {
		req.Header.Set(k, v)
	}
	start := time.Now()
	res, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return map[string]any{
			"ok":         false,
			"error":      err.Error(),
			"latency_ms": latency.Milliseconds(),
		}, nil
	}
	defer res.Body.Close()
	snippetRaw, _ := io.ReadAll(io.LimitReader(res.Body, int64(maxBody)+1))
	truncated := len(snippetRaw) > maxBody
	if truncated {
		snippetRaw = snippetRaw[:maxBody]
	}
	expected := pluginInt(input["expected_status"], 200)
	healthy := expected <= 0 || res.StatusCode == expected
	out := map[string]any{
		"ok":              true,
		"healthy":         healthy,
		"status_code":     res.StatusCode,
		"expected_status": expected,
		"latency_ms":      latency.Milliseconds(),
		"final_url":       res.Request.URL.String(),
		"content_type":    res.Header.Get("Content-Type"),
		"body_snippet":    string(snippetRaw),
		"body_truncated":  truncated,
	}
	rt.Log(logInfo, "probe", "http probe completed", map[string]any{
		"url":         rawURL,
		"status_code": res.StatusCode,
		"latency_ms":  latency.Milliseconds(),
		"healthy":     healthy,
	})
	return out, nil
}

func pluginValidatePublicHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return errors.New("主机名不能为空")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || strings.HasSuffix(lower, ".local") {
		return fmt.Errorf("已阻止内网/本机地址 %s；如需探测请在插件配置中开启允许内网地址", host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if pluginIsPrivateIP(ip) {
			return fmt.Errorf("已阻止内网/本机地址 %s；如需探测请在插件配置中开启允许内网地址", host)
		}
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("解析主机失败: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("主机 %s 没有可用 IP", host)
	}
	for _, addr := range addrs {
		if pluginIsPrivateIP(addr.IP) {
			return fmt.Errorf("已阻止解析到内网/本机地址 %s；如需探测请在插件配置中开启允许内网地址", addr.IP.String())
		}
	}
	return nil
}

func pluginIsPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// pluginZeroDialer 复用同一零配置拨号器（无 KeepAlive 干扰，按需建连）。
var pluginZeroDialer = &net.Dialer{}

// pluginSSRFDialContext 在拨号时原子地「解析主机→逐 IP 校验公网→直连已校验 IP」，
// 消除 pluginValidatePublicHost 预检与实际连接之间的 DNS 重绑定窗口（TOCTOU）：
// 预检与请求各做一次 DNS 解析，攻击者可在两次解析间令记录从公网 IP 翻转至内网 IP。
// 此处把校验下沉到拨号点，解析得到的 IP 直接用于建连，不再二次解析。
// 每次拨号（含重定向新主机）都独立校验，首跳与后续跳一视同仁。
// 参考 httpclient.ssrfDialContext。
func pluginSSRFDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	// 字面 IP：直接校验，无需解析（无重绑定窗口）。
	if ip := net.ParseIP(host); ip != nil {
		if pluginIsPrivateIP(ip) {
			return nil, fmt.Errorf("已阻止内网/本机地址 %s", host)
		}
		return pluginZeroDialer.DialContext(ctx, network, addr)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, a := range addrs {
		if pluginIsPrivateIP(a.IP) {
			lastErr = fmt.Errorf("已阻止解析到内网/本机地址 %s", a.IP.String())
			continue
		}
		conn, derr := pluginZeroDialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("主机 %s 没有可用的公网 IP", host)
	}
	return nil, lastErr
}
