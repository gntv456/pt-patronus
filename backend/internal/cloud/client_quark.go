package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// clientQuark — 夸克网盘 web API（cookie 鉴权，按 pdir_fid 列目录）。
//
// TODO 联调：端点/字段名基于社区逆向契约（drive-pc.quark.cn/1/clouddrive/file/sort），
// 真实 cookie 联调确认 file_type 判定（0=目录）、分页 _page/_size、配额端点。单测用 mock 覆盖。
type clientQuark struct {
	drive Drive
	hc    httpDoer
}

func (c *clientQuark) Type() string { return "quark" }

const apiQuark = "https://drive-pc.quark.cn/1/clouddrive"

// rawQuarkItem 夸克 file/sort 返回条目。file_type=0 目录，1 文件。
type rawQuarkItem struct {
	FID      string      `json:"fid"`
	Name     string      `json:"file_name"`
	Size     json.Number `json:"size"`
	FileType json.Number `json:"file_type"`
	Category json.Number `json:"category"`
}

func (it rawQuarkItem) isDir() bool { return it.FileType.String() == "0" }

func (c *clientQuark) rawList(ctx context.Context, fid string) ([]rawQuarkItem, error) {
	var out []rawQuarkItem
	size := 100
	for page := 1; ; page++ {
		u := fmt.Sprintf("%s/file/sort?pr=ucpro&fr=pc&pdir_fid=%s&_page=%d&_size=%d&_fetch_total=1&_fetch_sub_dirs=0&_sort=file_type:asc,updated_at:desc",
			apiQuark, url.QueryEscape(fid), page, size)
		resp, err := c.hc.Do(ctx, "GET", u, nil, jsonHeaders(), c.drive.Token, true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("quark list: HTTP %d", resp.StatusCode)
		}
		var body struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				List []rawQuarkItem `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return nil, fmt.Errorf("quark list parse: %w", err)
		}
		if body.Code != 0 && body.Code != 200 {
			return nil, fmt.Errorf("quark list 失败: %s", body.Msg)
		}
		out = append(out, body.Data.List...)
		if len(body.Data.List) < size {
			return out, nil
		}
	}
}

func (c *clientQuark) List(ctx context.Context, dir string) ([]File, error) {
	fid, err := c.resolvePath(ctx, dir)
	if err != nil {
		return nil, err
	}
	raw, err := c.rawList(ctx, fid)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(raw))
	for _, it := range raw {
		out = append(out, File{
			Name: it.Name, Path: joinCloud(dir, it.Name),
			IsDir: it.isDir(), Size: numToInt64(it.Size), PickCode: it.FID, // 夸克下载需 fid
		})
	}
	return out, nil
}

// resolvePath 从根 fid="0" 按路径段逐级解析到目标 fid。
func (c *clientQuark) resolvePath(ctx context.Context, dir string) (string, error) {
	fid := "0"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, fid)
		if err != nil {
			return "", err
		}
		next := ""
		for _, it := range raw {
			if it.isDir() && it.Name == seg {
				next = it.FID
				break
			}
		}
		if next == "" {
			return "", fmt.Errorf("quark 路径不存在: %s", seg)
		}
		fid = next
	}
	return fid, nil
}

func (c *clientQuark) Test(ctx context.Context) (string, error) {
	raw, err := c.rawList(ctx, "0")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("夸克 · 根目录 %d 个条目", len(raw)), nil
}

func (c *clientQuark) Quota(ctx context.Context) (*Quota, error) {
	// TODO 联调：夸克容量端点（/1/clouddrive/capacity）字段确认。best-effort，解析失败返零值。
	resp, err := c.hc.Do(ctx, "GET", apiQuark+"/capacity?pr=ucpro&fr=pc", nil, jsonHeaders(), c.drive.Token, true)
	if err != nil || resp.StatusCode >= 300 {
		return &Quota{}, nil
	}
	var body struct {
		Data struct {
			UseCapacity   json.Number `json:"use_capacity"`
			TotalCapacity json.Number `json:"total_capacity"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &body) != nil {
		return &Quota{}, nil
	}
	return &Quota{Used: numToInt64(body.Data.UseCapacity), Total: numToInt64(body.Data.TotalCapacity)}, nil
}

var _ Client = (*clientQuark)(nil)
