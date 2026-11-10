// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// clientOneDrive — Microsoft OneDrive 个人版（MS Graph API v1.0，Bearer accessToken）。
// 不同于 115/夸克 cookie 模式：Graph 用 Authorization: Bearer 头 + drive-scoped 路径寻址
// （/me/drive/root:/path:/children），无需逐级解析 FileId。
//
// 凭据：TokenEnc 存 accessToken（不含 refresh 语义——由用户在 Azure 应用注册后定期换发，
// 或配合第三方长期 token 工具使用；后续可扩展 refresh_token 自动续期）。
type clientOneDrive struct {
	drive Drive
	hc    httpDoer
}

func (c *clientOneDrive) Type() string { return "onedrive" }

const apiGraph = "https://graph.microsoft.com/v1.0"

func (c *clientOneDrive) authHeader() map[string]string {
	h := jsonHeaders()
	if c.drive.Token != "" {
		h["Authorization"] = "Bearer " + c.drive.Token
	}
	return h
}

// childrenPath 把云盘路径映射为 Graph children URL（根 children / 路径 children）。
// 路径段逐段 PathEscape（整段 escape 会把 / 也编码成 %2F，Graph 不认）。
func childrenPath(dir string) string {
	segs := splitSegments(dir)
	if len(segs) == 0 {
		return apiGraph + "/me/drive/root/children"
	}
	return apiGraph + "/me/drive/root:/" + escapeSegments(segs) + ":/children"
}

// escapeSegments 逐段转义后用 / 重接（保留路径分隔符）。
func escapeSegments(segs []string) string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = url.PathEscape(s)
	}
	return strings.Join(out, "/")
}

// rawODItem Graph driveItem。
type rawODItem struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Size   int64     `json:"size"`
	Folder *struct{} `json:"folder"`
	File   *struct{} `json:"file"`
	Parent *struct {
		Path string `json:"path"`
	} `json:"parentReference"`
	DownloadURL string `json:"@microsoft.graph.downloadUrl"`
}

func (it rawODItem) isDir() bool { return it.Folder != nil }

func (c *clientOneDrive) List(ctx context.Context, dir string) ([]File, error) {
	var out []File
	next := childrenPath(dir)
	for page := 1; next != ""; page++ {
		resp, err := c.hc.Do(ctx, "GET", next, nil, c.authHeader(), "", true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("OneDrive 路径不存在: %s", dir)
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("onedrive list: HTTP %d", resp.StatusCode)
		}
		var body struct {
			Value []rawODItem `json:"value"`
			Next  string      `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return nil, fmt.Errorf("onedrive list parse: %w", err)
		}
		for _, it := range body.Value {
			out = append(out, File{
				Name: it.Name, Path: joinCloud(dir, it.Name),
				IsDir: it.isDir(), Size: it.Size,
			})
		}
		next = body.Next
		// 上限 200 页：防异常 nextLink 环。
		if page >= 200 {
			break
		}
	}
	return out, nil
}

func (c *clientOneDrive) Test(ctx context.Context) (string, error) {
	resp, err := c.hc.Do(ctx, "GET", apiGraph+"/me/drive", nil, c.authHeader(), "", true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("onedrive test: HTTP %d（检查 token 有效期/Scopes）", resp.StatusCode)
	}
	var body struct {
		Owner struct {
			User struct {
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"owner"`
		Quota struct {
			Used  int64 `json:"used"`
			Total int64 `json:"total"`
		} `json:"quota"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return "", fmt.Errorf("onedrive test parse: %w", err)
	}
	return fmt.Sprintf("OneDrive · %s", body.Owner.User.DisplayName), nil
}

func (c *clientOneDrive) Quota(ctx context.Context) (*Quota, error) {
	resp, err := c.hc.Do(ctx, "GET", apiGraph+"/me/drive", nil, c.authHeader(), "", true)
	if err != nil || resp.StatusCode >= 300 {
		return &Quota{}, nil
	}
	var body struct {
		Quota struct {
			Used  int64 `json:"used"`
			Total int64 `json:"total"`
		} `json:"quota"`
	}
	if json.Unmarshal(resp.Body, &body) != nil {
		return &Quota{}, nil
	}
	return &Quota{Used: body.Quota.Used, Total: body.Quota.Total}, nil
}

// DownloadURL Graph downloadUrl 直链（预签名 URL，限时；无需 Authorization 头）。
func (c *clientOneDrive) DownloadURL(ctx context.Context, cloudPath string) (string, error) {
	segs := splitSegments(cloudPath)
	if len(segs) == 0 {
		return "", fmt.Errorf("OneDrive 直链需要文件路径")
	}
	u := apiGraph + "/me/drive/root:/" + escapeSegments(segs)
	resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("onedrive download: HTTP %d", resp.StatusCode)
	}
	var body rawODItem
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return "", err
	}
	if body.DownloadURL == "" {
		return "", fmt.Errorf("OneDrive 条目无 downloadUrl")
	}
	return body.DownloadURL, nil
}

var _ Client = (*clientOneDrive)(nil)
