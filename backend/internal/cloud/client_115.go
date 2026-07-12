package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// client115 — 115网盘 web API（cookie 鉴权，按 cid 列目录）。
//
// TODO 联调：端点/字段名基于社区逆向契约（webapi.115.com/files），真实 cookie 联调确认
// 列目录的 count/offset 分页、文件夹判定（无 sha1 即目录）、配额端点字段。单测用 mock 契约覆盖。
type client115 struct {
	drive Drive
	hc    httpDoer
}

func (c *client115) Type() string { return "115" }

const api115 = "https://webapi.115.com"

// raw115Item 115 /files 返回的单个条目。文件项含 sha1+pickcode；目录项无。
type raw115Item struct {
	FileID   json.Number `json:"file_id"`
	CID      json.Number `json:"cid"` // 目录项：自身 cid；文件项：所在父 cid
	FID      json.Number `json:"fid"`
	Name     string      `json:"file_name"`
	Size     json.Number `json:"file_size"`
	SHA1     string      `json:"sha1"`
	PickCode string      `json:"pickcode"`
}

func (it raw115Item) isDir() bool { return it.SHA1 == "" } // 文件夹无 sha1
func (it raw115Item) id() string {
	if it.isDir() && it.CID.String() != "" {
		return it.CID.String()
	}
	if it.FID.String() != "" {
		return it.FID.String()
	}
	return it.FileID.String()
}

// rawList 按 cid 拉原始条目（自动翻页直到取尽）。返回原始项供路径解析与列表复用。
func (c *client115) rawList(ctx context.Context, cid string) ([]raw115Item, error) {
	var out []raw115Item
	limit := 100
	for offset := 0; ; offset += limit {
		u := fmt.Sprintf("%s/files?cid=%s&o=user_index&aid=1&cur_dir_mode=1&count=%d&offset=%d&show_dir=1",
			api115, url.QueryEscape(cid), limit, offset)
		resp, err := c.hc.Do(ctx, "GET", u, nil, jsonHeaders(), c.drive.Token, true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("115 list: HTTP %d", resp.StatusCode)
		}
		var body struct {
			State bool         `json:"state"`
			Err   string       `json:"error"`
			Data  []raw115Item `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return nil, fmt.Errorf("115 list parse: %w", err)
		}
		if !body.State {
			return nil, fmt.Errorf("115 list 失败: %s", body.Err)
		}
		out = append(out, body.Data...)
		if len(body.Data) < limit {
			return out, nil
		}
	}
}

func (c *client115) List(ctx context.Context, dir string) ([]File, error) {
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
			IsDir: it.isDir(), Size: numToInt64(it.Size), PickCode: it.PickCode,
		})
	}
	return out, nil
}

// resolvePath 从根 cid=0 按路径段逐级解析到目标 cid。
func (c *client115) resolvePath(ctx context.Context, dir string) (string, error) {
	cid := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, cid)
		if err != nil {
			return "", err
		}
		next := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				next = it.id()
				break
			}
		}
		if next == "" {
			return "", fmt.Errorf("115 路径不存在: %s", seg)
		}
		cid = next
	}
	return cid, nil
}

func (c *client115) Test(ctx context.Context) (string, error) {
	raw, err := c.rawList(ctx, "0")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("115 · 根目录 %d 个条目", len(raw)), nil
}

func (c *client115) Quota(ctx context.Context) (*Quota, error) {
	// TODO 联调：115 容量端点（webapi.115.com/user/info 或 proapi user/total_space）字段确认。
	// best-effort：解析失败返零值（UI 显"—"），不报错（配额仅展示）。
	resp, err := c.hc.Do(ctx, "GET", api115+"/user/info", nil, jsonHeaders(), c.drive.Token, true)
	if err != nil || resp.StatusCode >= 300 {
		return &Quota{}, nil
	}
	var body struct {
		Data struct {
			Space struct {
				SpaceTotal json.Number `json:"space_total"`
				SpaceUsed  json.Number `json:"space_used"`
			} `json:"space"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &body) != nil {
		return &Quota{}, nil
	}
	return &Quota{Used: numToInt64(body.Data.Space.SpaceUsed), Total: numToInt64(body.Data.Space.SpaceTotal)}, nil
}

// numToInt64 容错把 json.Number 转 int64（空/非法返 0）。
func numToInt64(n json.Number) int64 {
	if n == "" {
		return 0
	}
	v, _ := n.Int64()
	return v
}

// jsonHeaders 返回 JSON 接收头。
func jsonHeaders() map[string]string {
	return map[string]string{"Accept": "application/json"}
}

// 静态断言：client115 实现 Client。
var _ Client = (*client115)(nil)
