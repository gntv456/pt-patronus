package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// client123 — 123云盘官方开放 API（accessToken 鉴权，按 parentFileId 列目录）。
// 与 115/夸克不同：123 用 Authorization: Bearer 头（非 cookie）；code=0 成功。
//
// TODO 联调：端点/字段名基于官方 API 文档（api.123pan.com/api/v1/file/list），
// 真实 accessToken 联调确认 Type 判定（0=目录）、配额端点。单测用 mock 覆盖。
type client123 struct {
	drive Drive
	hc    httpDoer
}

func (c *client123) Type() string { return "123" }

const api123 = "https://api.123pan.com"

// authHeader 返回 Bearer token 头（123 用 token 非 cookie）。
func (c *client123) authHeader() map[string]string {
	h := jsonHeaders()
	if c.drive.Token != "" {
		h["Authorization"] = "Bearer " + c.drive.Token
	}
	return h
}

// raw123Item 123 /api/v1/file/list 返回条目。Type=0 目录，1 文件。
type raw123Item struct {
	FileID json.Number `json:"FileId"`
	Name   string      `json:"FileName"`
	Size   json.Number `json:"Size"`
	Type   json.Number `json:"Type"`
}

func (it raw123Item) isDir() bool { return it.Type.String() == "0" }

func (c *client123) rawList(ctx context.Context, parentID string) ([]raw123Item, error) {
	var out []raw123Item
	limit := 100
	for page := 1; ; page++ {
		u := fmt.Sprintf("%s/api/v1/file/list?parentFileId=%s&limit=%d&page=%d&OrderBy=file_id&OrderDirection=desc&trashed=false",
			api123, url.QueryEscape(parentID), limit, page)
		resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("123 list: HTTP %d", resp.StatusCode)
		}
		var body struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				InfoList []raw123Item `json:"InfoList"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return nil, fmt.Errorf("123 list parse: %w", err)
		}
		if body.Code != 0 {
			return nil, fmt.Errorf("123 list 失败: %s", body.Msg)
		}
		out = append(out, body.Data.InfoList...)
		if len(body.Data.InfoList) < limit {
			return out, nil
		}
	}
}

func (c *client123) List(ctx context.Context, dir string) ([]File, error) {
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
			IsDir: it.isDir(), Size: numToInt64(it.Size),
		})
	}
	return out, nil
}

// resolvePath 从根 parentFileId=0 按路径段逐级解析到目标 FileId。
func (c *client123) resolvePath(ctx context.Context, dir string) (string, error) {
	parentID := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, parentID)
		if err != nil {
			return "", err
		}
		next := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				next = it.FileID.String()
				break
			}
		}
		if next == "" {
			return "", fmt.Errorf("123 路径不存在: %s", seg)
		}
		parentID = next
	}
	return parentID, nil
}

func (c *client123) Test(ctx context.Context) (string, error) {
	raw, err := c.rawList(ctx, "0")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("123云盘 · 根目录 %d 个条目", len(raw)), nil
}

func (c *client123) Quota(ctx context.Context) (*Quota, error) {
	// TODO 联调：123 容量端点字段确认。best-effort，解析失败返零值。
	resp, err := c.hc.Do(ctx, "GET", api123+"/api/v1/user/info", nil, c.authHeader(), "", true)
	if err != nil || resp.StatusCode >= 300 {
		return &Quota{}, nil
	}
	var body struct {
		Data struct {
			UseSize   json.Number `json:"spaceUsed"`
			TotalSize json.Number `json:"spaceTotal"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &body) != nil {
		return &Quota{}, nil
	}
	return &Quota{Used: numToInt64(body.Data.UseSize), Total: numToInt64(body.Data.TotalSize)}, nil
}

var _ Client = (*client123)(nil)
