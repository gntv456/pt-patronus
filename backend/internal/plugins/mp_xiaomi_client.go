// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type mpXiaomiClient struct {
	baseURL  string
	password string
	timeout  time.Duration
	hc       *http.Client
	token    string
}

func mpNewXiaomiClient(ctx context.Context, cfg map[string]any) (*mpXiaomiClient, error) {
	raw := strings.TrimSpace(pluginString(cfg["router_url"]))
	if raw == "" {
		return nil, errors.New("路由器地址不能为空")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("路由器地址无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("路由器地址仅支持 http/https")
	}
	if !asBool(cfg["allow_public_router"]) {
		if err := mpValidateRouterHost(ctx, u.Hostname()); err != nil {
			return nil, err
		}
	}
	password := pluginString(cfg["password"])
	if password == "" {
		return nil, errors.New("路由器登录密码不能为空")
	}
	timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 10)) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &mpXiaomiClient{
		baseURL:  strings.TrimRight(u.String(), "/"),
		password: password,
		timeout:  timeout,
		hc:       &http.Client{Timeout: timeout},
	}, nil
}

func (c *mpXiaomiClient) Status(ctx context.Context) (map[string]any, []map[string]any, error) {
	if err := c.login(ctx); err != nil {
		return nil, nil, err
	}
	var raw map[string]any
	if err := c.getJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/misystem/status", c.token), &raw); err != nil {
		return nil, nil, err
	}
	status := mpParseXiaomiStatus(raw)
	ports, _ := c.PortList(ctx)
	return status, ports, nil
}

func (c *mpXiaomiClient) PortList(ctx context.Context) ([]map[string]any, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := c.getJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/xqnetwork/portforward?ftype=1", c.token), &raw); err != nil {
		return nil, err
	}
	list, _ := raw["list"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func (c *mpXiaomiClient) PortAdd(ctx context.Context, name string, proto, sport int, ip string, dport int) (map[string]any, error) {
	if name == "" || proto == 0 || sport == 0 || ip == "" || dport == 0 {
		return nil, errors.New("端口映射参数不完整")
	}
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("name", name)
	form.Set("proto", fmt.Sprint(proto))
	form.Set("sport", fmt.Sprint(sport))
	form.Set("ip", ip)
	form.Set("dport", fmt.Sprint(dport))
	var out map[string]any
	err := c.postFormJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/xqnetwork/add_redirect", c.token), form, &out)
	return out, err
}

func (c *mpXiaomiClient) PortDelete(ctx context.Context, port, proto int) (map[string]any, error) {
	if port == 0 {
		return nil, errors.New("端口不能为空")
	}
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("port", fmt.Sprint(port))
	form.Set("proto", fmt.Sprint(proto))
	var out map[string]any
	err := c.postFormJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/xqnetwork/delete_redirect", c.token), form, &out)
	return out, err
}

func (c *mpXiaomiClient) QuickToggle(ctx context.Context, cfg map[string]any) (map[string]any, error) {
	name := pluginString(cfg["q_name"])
	proto := mpXiaomiProtoInt(cfg["q_proto"])
	sport := pluginInt(cfg["q_sport"], 0)
	ip := pluginString(cfg["q_ip"])
	dport := pluginInt(cfg["q_dport"], 0)
	if name == "" || proto == 0 || sport == 0 || ip == "" || dport == 0 {
		return nil, errors.New("快捷预设不完整")
	}
	ports, _ := c.PortList(ctx)
	for _, p := range ports {
		if pluginInt(p["srcport"], -1) == sport {
			res, err := c.PortDelete(ctx, sport, pluginInt(p["proto"], proto))
			return map[string]any{"action": "delete", "port": sport, "result": res}, err
		}
	}
	res, err := c.PortAdd(ctx, name, proto, sport, ip, dport)
	return map[string]any{"action": "add", "port": sport, "result": res}, err
}

func (c *mpXiaomiClient) login(ctx context.Context) error {
	if c.token != "" {
		return nil
	}
	var init map[string]any
	if err := c.getJSON(ctx, "/cgi-bin/luci/api/xqsystem/init_info", &init); err != nil {
		return fmt.Errorf("获取 init_info 失败: %w", err)
	}
	key := pluginString(init["key"])
	if key == "" {
		key = pluginString(init["salt"])
	}
	if key == "" {
		key = pluginString(init["pwd"])
	}
	if key == "" {
		key = c.fetchLoginKey(ctx)
	}
	deviceID := strings.NewReplacer(":", "", "-", "").Replace(pluginString(init["routerId"]))
	if deviceID == "" {
		deviceID = strings.NewReplacer(":", "", "-", "").Replace(pluginString(init["id"]))
	}
	if deviceID == "" {
		deviceID = strings.NewReplacer(":", "", "-", "").Replace(pluginString(init["deviceId"]))
	}
	nonce := fmt.Sprintf("0_%s_%d_%d", deviceID, time.Now().Unix(), rand.Intn(10000))
	hashed := mpSHA1Hex(nonce + mpSHA1Hex(c.password+key))
	if asBool(init["newEncryptMode"]) || pluginInt(init["newEncryptMode"], 0) == 1 {
		hashed = mpSHA256Hex(nonce + mpSHA256Hex(c.password+key))
	}
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", hashed)
	form.Set("logtype", "2")
	form.Set("nonce", nonce)
	var out map[string]any
	if err := c.postFormJSON(ctx, "/cgi-bin/luci/api/xqsystem/login", form, &out); err != nil {
		return err
	}
	if pluginInt(out["code"], -1) != 0 || pluginString(out["token"]) == "" {
		return fmt.Errorf("登录失败: %v", out)
	}
	c.token = pluginString(out["token"])
	return nil
}

func (c *mpXiaomiClient) fetchLoginKey(ctx context.Context) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/cgi-bin/luci/web", nil)
	res, err := c.hc.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	text := string(body)
	for _, pattern := range []string{`key\s*[:=]\s*"([A-Za-z0-9]+)"`, `key\s*[:=]\s*'([A-Za-z0-9]+)'`, `Encrypt\([^\)]*["']([A-Za-z0-9]+)["']`} {
		m := regexp.MustCompile(pattern).FindStringSubmatch(text)
		if len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

func (c *mpXiaomiClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", c.baseURL+"/cgi-bin/luci/web")
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *mpXiaomiClient) postFormJSON(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Referer", c.baseURL+"/cgi-bin/luci/web")
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
