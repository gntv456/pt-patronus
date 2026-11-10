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

// clientTianyi — 天翼云盘个人版（开放 API api.189.cn 体系，家庭云/个人云 xpan 同源）。
// 鉴权：accessToken（天翼开放平台授权获得），Query 参数 accessToken 传递（部分网关不认 Header）。
// 列表：GET /api/file/listFiles（familyId 缺省=个人云，按 parentFolderId 逐级；根=空字符串）。
//
// TODO 联调：分页 marker 与 downloadUrl（fileDownloadUrl 需会话）按真实账号联调微调。
type clientTianyi struct {
	drive Drive
	hc    httpDoer
}

func (c *clientTianyi) Type() string { return "tianyi" }

const apiTianyi = "https://api.189.cn"

// tianyiURL 拼 endpoint + accessToken query。
func (c *clientTianyi) tianyiURL(path string, params map[string]string) string {
	u := apiTianyi + path
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	if c.drive.Token != "" {
		q.Set("accessToken", c.drive.Token)
	}
	return u + "?" + q.Encode()
}

// rawTYItem 189 file list 条目。
type rawTYItem struct {
	ID       json.Number `json:"id"`
	FileID   json.Number `json:"fileId"`
	Name     string      `json:"name"`
	IsFolder bool        `json:"isFolder"`
	Size     int64       `json:"size"`
	Url      string      `json:"url"` // 文件时为下载短链（可能限时）
}

func (it rawTYItem) idStr() string {
	if it.FileID.String() != "" {
		return it.FileID.String()
	}
	return it.ID.String()
}

func (c *clientTianyi) rawList(ctx context.Context, parentID string) ([]rawTYItem, error) {
	var out []rawTYItem
	const limit = 100
	for pageNum := 1; ; pageNum++ {
		u := c.tianyiURL("/api/file/listFiles.action", map[string]string{
			"parentFolderId": parentID,
			"pageNum":        fmt.Sprint(pageNum),
			"pageSize":       fmt.Sprint(limit),
			"mediaAttr":      "1",
			"iconOption":     "5",
		})
		resp, err := c.hc.Do(ctx, "GET", u, nil, jsonHeaders(), "", true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("tianyi list: HTTP %d", resp.StatusCode)
		}
		var body struct {
			ResCode    int `json:"res_code"`
			FileListAO struct {
				FileList   []rawTYItem `json:"fileList"`
				FolderList []rawTYItem `json:"folderList"`
			} `json:"fileListAO"`
			// 旧接口形态：顶层 fileArray
			FileArray []rawTYItem `json:"fileArray"`
			Count     int         `json:"count"`
			RecordCnt int         `json:"recordCnt"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return nil, fmt.Errorf("tianyi list parse: %w", err)
		}
		if body.ResCode != 0 {
			return nil, fmt.Errorf("tianyi list 失败: res_code=%d（token 失效/参数错）", body.ResCode)
		}
		batch := append(body.FileListAO.FolderList, body.FileListAO.FileList...)
		if len(batch) == 0 {
			batch = body.FileArray
		}
		out = append(out, batch...)
		total := body.Count
		if body.RecordCnt > total {
			total = body.RecordCnt
		}
		if len(batch) < limit || (total > 0 && len(out) >= total) || pageNum >= 100 {
			return out, nil
		}
	}
}

func (c *clientTianyi) List(ctx context.Context, dir string) ([]File, error) {
	parentID, err := c.resolvePath(ctx, dir)
	if err != nil {
		return nil, err
	}
	raw, err := c.rawList(ctx, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(raw))
	for _, it := range raw {
		out = append(out, File{
			Name: it.Name, Path: joinCloud(dir, it.Name),
			IsDir: it.IsFolder, Size: it.Size,
			PickCode: it.idStr(),
		})
	}
	return out, nil
}

// resolvePath 从根（parentFolderId=""）逐段解析到目标目录 id。
func (c *clientTianyi) resolvePath(ctx context.Context, dir string) (string, error) {
	parentID := ""
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, parentID)
		if err != nil {
			return "", err
		}
		next := ""
		for _, it := range raw {
			if it.IsFolder && it.Name == seg {
				next = it.idStr()
				break
			}
		}
		if next == "" {
			return "", fmt.Errorf("天翼云盘路径不存在: %s", seg)
		}
		parentID = next
	}
	return parentID, nil
}

func (c *clientTianyi) Test(ctx context.Context) (string, error) {
	raw, err := c.rawList(ctx, "")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("天翼云盘 · 根目录 %d 个条目", len(raw)), nil
}

func (c *clientTianyi) Quota(ctx context.Context) (*Quota, error) {
	u := c.tianyiURL("/api/getUserDiskInfo.action", nil)
	resp, err := c.hc.Do(ctx, "GET", u, nil, jsonHeaders(), "", true)
	if err != nil || resp.StatusCode >= 300 {
		return &Quota{}, nil
	}
	var body struct {
		ResCode int `json:"res_code"`
		Disk    struct {
			TotalSize json.Number `json:"totalSize"`
			UsedSize  json.Number `json:"usedSize"`
		} `json:"disk"`
	}
	if json.Unmarshal(resp.Body, &body) != nil {
		return &Quota{}, nil
	}
	return &Quota{Used: numToInt64(body.Disk.UsedSize), Total: numToInt64(body.Disk.TotalSize)}, nil
}

// DownloadURL 列表项自带 url（限时短链）；按路径定位文件取 url。
func (c *clientTianyi) DownloadURL(ctx context.Context, cloudPath string) (string, error) {
	segs := splitSegments(cloudPath)
	if len(segs) == 0 {
		return "", fmt.Errorf("天翼云盘直链需要文件路径")
	}
	parent := "/" + strings.Join(segs[:len(segs)-1], "/")
	name := segs[len(segs)-1]
	raw, err := c.rawList(ctx, parent)
	if err != nil {
		return "", err
	}
	for _, it := range raw {
		if !it.IsFolder && it.Name == name {
			if it.Url == "" {
				return "", fmt.Errorf("天翼云盘条目无下载短链")
			}
			return it.Url, nil
		}
	}
	return "", fmt.Errorf("天翼云盘未找到文件: %s", cloudPath)
}

var _ Client = (*clientTianyi)(nil)
