// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ---- 115 开放平台客户端（proapi.115.com/open/*，Bearer access_token） ----
//
// 契约来源：xhofe/115-sdk-go（alist drivers/115_open 依赖的官方 SDK）。与 web 契约
// （client115）并存：Token 存 access_token（非 cookie），类型 "115_open"。
// 上传：/open/upload/init（全文件 SHA1 + 前 128KB SHA1 双哈希秒传，status 6/7/8 二次
// 校验字节区间 SHA1）→ /open/upload/get_token 拿 OSS STS → PutObject 直传（callback 透传）。
// 直链：/open/ufile/downurl（pick_code form；UA 绑定——下载需同 UA）。

const api115Open = "https://proapi.115.com"

// 静态断言：client115Open 实现 Client + Writer。
var (
	_ Client = (*client115Open)(nil)
	_ Writer = (*client115Open)(nil)
)

type client115Open struct {
	drive Drive // Token = access_token 或 TokenPair JSON（Bearer）
	hc    httpDoer
	// onTokenRefresh 刷新成功后回写新令牌（chain 注入；nil=不回写，仍用新值重试）。
	onTokenRefresh func(tp TokenPair)
}

// SetTokenRefreshHook 实现 cloud.TokenRefresher（chain 注入持久化回写）。
func (c *client115Open) SetTokenRefreshHook(fn func(newToken string)) {
	c.onTokenRefresh = func(tp TokenPair) { fn(EncodeTokenPair(tp)) }
}

// do175 带令牌失效自愈的请求：401/99 时刷新一次重试。
func (c *client115Open) doHeal(ctx context.Context, method, u string, body []byte) (statusCode int, respBody []byte, err error) {
	resp, err := c.hc.Do(ctx, method, u, body, c.authHeaders(), "", true)
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode == 401 || (resp.StatusCode == 200 && authInvalid115Open(resp.Body)) {
		if rerr := c.tryRefresh(ctx); rerr == nil {
			resp, err = c.hc.Do(ctx, method, u, body, c.authHeaders(), "", true)
			if err != nil {
				return 0, nil, err
			}
		}
	}
	return resp.StatusCode, resp.Body, nil
}

// bearer 解析当前 access_token（JSON 双令牌或裸串）。
func (c *client115Open) bearer() string {
	return DecodeTokenPair(c.drive.Token).AccessToken
}

// tryRefresh 用 refresh_token 换新（有回写则更新 drive.Token）。
func (c *client115Open) tryRefresh(ctx context.Context) error {
	tp := DecodeTokenPair(c.drive.Token)
	if tp.RefreshToken == "" {
		return fmt.Errorf("115_open: token 失效且无 refresh_token")
	}
	np, err := Refresh115Open(ctx, c.hc, tp.RefreshToken)
	if err != nil {
		return err
	}
	c.drive.Token = EncodeTokenPair(*np)
	if c.onTokenRefresh != nil {
		c.onTokenRefresh(*np)
	}
	return nil
}

func (c *client115Open) Type() string { return "115_open" }

func (c *client115Open) authHeaders() map[string]string {
	h := jsonHeaders()
	if tok := c.bearer(); tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	return h
}

// authErr 401/令牌失效类响应（触发一次刷新重试）。
func authInvalid115Open(body []byte) bool {
	return strings.Contains(string(body), `"errno":401`) ||
		strings.Contains(string(body), `"code":99`) ||
		strings.Contains(string(body), "access_token")
}

// raw115OpenItem /open/ufile/files 条目。fc "0"=目录 "1"=文件。
type raw115OpenItem struct {
	FID  json.Number `json:"fid"`
	PID  json.Number `json:"pid"`
	Name string      `json:"fn"`
	FC   string      `json:"fc"`
	Size json.Number `json:"fs"`
	SHA1 string      `json:"sha1"`
	PC   string      `json:"pc"` // pick_code
}

func (it raw115OpenItem) isDir() bool { return it.FC == "0" }

// rawListOpen 分页列目录（limit=200，按 count 终止；200 页上限防失控）。
func (c *client115Open) rawList(ctx context.Context, cid string) ([]raw115OpenItem, error) {
	var out []raw115OpenItem
	limit := 200
	for offset := 0; ; offset += limit {
		u := fmt.Sprintf("%s/open/ufile/files?cid=%s&limit=%d&offset=%d&asc=1&o=file_name&show_dir=1",
			api115Open, url.QueryEscape(cid), limit, offset)
		sc, rb, err := c.doHeal(ctx, "GET", u, nil)
		if err != nil {
			return nil, err
		}
		if sc >= 300 {
			return nil, fmt.Errorf("115_open list: HTTP %d", sc)
		}
		var body struct {
			State bool             `json:"state"`
			Err   string           `json:"error"`
			Count int              `json:"count"`
			Data  []raw115OpenItem `json:"data"`
		}
		if err := json.Unmarshal(rb, &body); err != nil {
			return nil, fmt.Errorf("115_open list parse: %w", err)
		}
		if !body.State {
			return nil, fmt.Errorf("115_open list 失败: %s", body.Err)
		}
		out = append(out, body.Data...)
		if len(out) >= body.Count || len(body.Data) < limit {
			return out, nil
		}
		if offset/limit+1 >= 200 {
			return out, nil
		}
	}
}

func (c *client115Open) List(ctx context.Context, dir string) ([]File, error) {
	cid, err := c.resolvePath(ctx, dir)
	if err != nil {
		return nil, err
	}
	raw, err := c.rawList(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(raw))
	for _, it := range raw {
		out = append(out, File{
			Name: it.Name, Path: joinCloud(dir, it.Name),
			IsDir: it.isDir(), Size: numToInt64(it.Size), PickCode: it.PC,
		})
	}
	return out, nil
}

// resolvePath 根 cid="0" 逐级解析。
func (c *client115Open) resolvePath(ctx context.Context, dir string) (string, error) {
	cid := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, cid)
		if err != nil {
			return "", err
		}
		next := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				next = it.FID.String()
				break
			}
		}
		if next == "" {
			return "", fmt.Errorf("115_open 路径不存在: %s", seg)
		}
		cid = next
	}
	return cid, nil
}

func (c *client115Open) Test(ctx context.Context) (string, error) {
	raw, err := c.rawList(ctx, "0")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("115开放平台 · 根目录 %d 个条目", len(raw)), nil
}

func (c *client115Open) Quota(ctx context.Context) (*Quota, error) {
	// 开放平台配额端点未在 SDK 中暴露；best-effort 返零值（UI 显"—"）。
	return &Quota{}, nil
}

// MkdirP 逐级建目录（POST /open/folder/add form: pid + file_name——注意非 cname）。
func (c *client115Open) MkdirP(ctx context.Context, dir string) (string, error) {
	cid := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, cid)
		if err != nil {
			return "", err
		}
		found := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				found = it.FID.String()
				break
			}
		}
		if found != "" {
			cid = found
			continue
		}
		form := url.Values{"pid": {cid}, "file_name": {seg}}
		sc, rb, err := c.doHeal(ctx, "POST", api115Open+"/open/folder/add", []byte(form.Encode()))
		if err != nil {
			return "", fmt.Errorf("115_open mkdir %s: %w", seg, err)
		}
		if sc >= 300 {
			return "", fmt.Errorf("115_open mkdir: HTTP %d", sc)
		}
		var wrap struct {
			State bool   `json:"state"`
			Err   string `json:"error"`
			Data  struct {
				FileID json.Number `json:"file_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rb, &wrap); err != nil {
			return "", fmt.Errorf("115_open mkdir parse: %w", err)
		}
		if !wrap.State {
			return "", fmt.Errorf("115_open mkdir %s 失败: %s", seg, wrap.Err)
		}
		if id := wrap.Data.FileID.String(); id != "" {
			cid = id
			continue
		}
		again, err := c.rawList(ctx, cid)
		if err != nil {
			return "", err
		}
		ok := false
		for _, it := range again {
			if it.isDir() && it.Name == seg {
				cid, ok = it.FID.String(), true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("115_open mkdir %s: 无法确认新目录 id", seg)
		}
	}
	return cid, nil
}

// uploadInitOpen /open/upload/init 响应。
type uploadInitOpen struct {
	Status     int    `json:"status"` // 2=秒传 6/7/8=二次校验
	Bucket     string `json:"bucket"`
	Object     string `json:"object"`
	Callback   string `json:"callback"`
	CallbackVr string `json:"callback_var"`
	PickCode   string `json:"pick_code"`
	SignKey    string `json:"sign_key"`
	SignCheck  string `json:"sign_check"`
	Err        string `json:"statusmsg"`
}

func (c *client115Open) DownloadURL(ctx context.Context, cloudPath, ua string) (*DirectLink, error) {
	f, err := c.statCloud(ctx, cloudPath)
	if err != nil {
		return nil, err
	}
	if f.IsDir {
		return nil, fmt.Errorf("115_open 直链: %s 是目录", cloudPath)
	}
	if f.PickCode == "" {
		return nil, fmt.Errorf("115_open 直链: 无 pick_code")
	}
	form := url.Values{"pick_code": {f.PickCode}}
	sc, rb, err := c.doHeal(ctx, "POST", api115Open+"/open/ufile/downurl", []byte(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("115_open downurl: %w", err)
	}
	if sc >= 300 {
		return nil, fmt.Errorf("115_open downurl: HTTP %d", sc)
	}
	var wrap struct {
		State bool   `json:"state"`
		Err   string `json:"error"`
		Data  map[string]struct {
			URL struct {
				URL string `json:"url"`
			} `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rb, &wrap); err != nil {
		return nil, fmt.Errorf("115_open downurl parse: %w", err)
	}
	if !wrap.State {
		return nil, fmt.Errorf("115_open 直链解析失败: %s", wrap.Err)
	}
	for _, v := range wrap.Data {
		if v.URL.URL != "" {
			h := map[string]string{}
			if ua != "" {
				h["User-Agent"] = ua
			}
			return &DirectLink{URL: v.URL.URL, Header: h}, nil
		}
	}
	return nil, fmt.Errorf("115_open 直链: 响应无 url")
}

// statCloud 父目录列表按名找。
func (c *client115Open) statCloud(ctx context.Context, dst string) (File, error) {
	files, err := c.List(ctx, cloudDirOf(dst))
	if err != nil {
		return File{}, err
	}
	name := pathBase(dst)
	for _, f := range files {
		if f.Name == name {
			return f, nil
		}
	}
	return File{}, os.ErrNotExist
}
