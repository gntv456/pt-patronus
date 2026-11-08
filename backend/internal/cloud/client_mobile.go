package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// clientMobile — 中国移动云盘（原和彩云，caiyun-mobile.yunnan.cloud.10086.cn 体系，ylogin token）。
// 鉴权：accessToken（移动云盘开放平台/抓包授权获得），Authorization Bearer 头。
// 列表：POST /orchestration/personalCloud-repo/v1.0/browse 或开放平台 list 接口（parentCatalogId 逐级；根=-1）。
//
// TODO 联调：真实 token 联调确认分页 cursor 与直链字段；结构与官方文档对齐，mock 覆盖解析。
type clientMobile struct {
	drive Drive
	hc    httpDoer
}

func (c *clientMobile) Type() string { return "mobile" }

const apiMobile = "https://caiyun-mobile.yunnan.cloud.10086.cn"

func (c *clientMobile) authHeader() map[string]string {
	h := jsonHeaders()
	if c.drive.Token != "" {
		h["Authorization"] = "Bearer " + c.drive.Token
	}
	return h
}

// rawCMItem 移动云盘条目。ID 兼容数字/字符串双形态（历史接口字符串 id），
// 故用 any 接收再归一。
type rawCMItem struct {
	IDRaw   any         `json:"id"`
	Catalog bool        `json:"isCatalog"`
	Name    string      `json:"name"`
	Size    json.Number `json:"size"` // 历史接口可能返回字符串数字，json.Number 兼容
}

func (it rawCMItem) idStr() string {
	switch v := it.IDRaw.(type) {
	case string:
		return v
	case float64:
		return strconvFormat(v)
	}
	return ""
}

func strconvFormat(f float64) string {
	return fmt.Sprintf("%.0f", f)
}

func (c *clientMobile) rawList(ctx context.Context, parentID string) ([]rawCMItem, error) {
	var out []rawCMItem
	const limit = 100
	for pageNum := 1; ; pageNum++ {
		u := fmt.Sprintf("%s/orchestration/personalCloud-repo/v1.0/queryContentList?parentCatalogId=%s&numPerPage=%d&pageNum=%d&contentSort=0",
			apiMobile, url.QueryEscape(parentID), limit, pageNum)
		resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("mobile list: HTTP %d", resp.StatusCode)
		}
		var body struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				List []rawCMItem `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return nil, fmt.Errorf("mobile list parse: %w", err)
		}
		if body.Code != 0 {
			return nil, fmt.Errorf("移动云盘 list 失败: %s (code=%d)", body.Msg, body.Code)
		}
		out = append(out, body.Data.List...)
		if len(body.Data.List) < limit || pageNum >= 100 {
			return out, nil
		}
	}
}

func (c *clientMobile) List(ctx context.Context, dir string) ([]File, error) {
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
			IsDir: it.Catalog, Size: numToInt64(it.Size),
			PickCode: it.idStr(),
		})
	}
	return out, nil
}

// resolvePath 根目录 parentCatalogId=-1，逐段下钻。
func (c *clientMobile) resolvePath(ctx context.Context, dir string) (string, error) {
	parentID := "-1"
	for _, seg := range splitSegments(dir) {
		raw, err := c.rawList(ctx, parentID)
		if err != nil {
			return "", err
		}
		next := ""
		for _, it := range raw {
			if it.Catalog && it.Name == seg {
				next = it.idStr()
				break
			}
		}
		if next == "" {
			return "", fmt.Errorf("移动云盘路径不存在: %s", seg)
		}
		parentID = next
	}
	return parentID, nil
}

func (c *clientMobile) Test(ctx context.Context) (string, error) {
	raw, err := c.rawList(ctx, "-1")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("移动云盘 · 根目录 %d 个条目", len(raw)), nil
}

func (c *clientMobile) Quota(ctx context.Context) (*Quota, error) {
	u := apiMobile + "/orchestration/personalCloud-repo/v1.0/queryUserDiskInfo"
	resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
	if err != nil || resp.StatusCode >= 300 {
		return &Quota{}, nil
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			TotalSize json.Number `json:"totalCapacity"`
			UsedSize  json.Number `json:"usedCapacity"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &body) != nil {
		return &Quota{}, nil
	}
	return &Quota{Used: numToInt64(body.Data.UsedSize), Total: numToInt64(body.Data.TotalSize)}, nil
}

// DownloadURL 直链：开放平台 downloadUrl 接口（contentId=条目 id）。
func (c *clientMobile) DownloadURL(ctx context.Context, cloudPath string) (string, error) {
	segs := splitSegments(cloudPath)
	if len(segs) == 0 {
		return "", fmt.Errorf("移动云盘直链需要文件路径")
	}
	parent := "/" + strings.Join(segs[:len(segs)-1], "/")
	name := segs[len(segs)-1]
	parentID, err := c.resolvePath(ctx, parent)
	if err != nil {
		return "", err
	}
	raw, err := c.rawList(ctx, parentID)
	if err != nil {
		return "", err
	}
	contentID := ""
	for _, it := range raw {
		if !it.Catalog && it.Name == name {
			contentID = it.idStr()
			break
		}
	}
	if contentID == "" {
		return "", fmt.Errorf("移动云盘未找到文件: %s", cloudPath)
	}
	u := fmt.Sprintf("%s/orchestration/personalCloud-repo/v1.0/getFileDownloadUrl?contentId=%s", apiMobile, url.QueryEscape(contentID))
	resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("mobile download: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			DownloadURL string `json:"downloadUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return "", err
	}
	if body.Code != 0 || body.Data.DownloadURL == "" {
		return "", fmt.Errorf("移动云盘直链失败 (code=%d)", body.Code)
	}
	return body.Data.DownloadURL, nil
}

var _ Client = (*clientMobile)(nil)
