package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// clientBaidu — 百度网盘开放平台（openapi.baidu.com/rest/2.0/xpan）。
// 鉴权：Bearer accessToken（在开放平台创建应用 + 用户授权后获得，TokenEnc 存该 token）。
// 文件列表：GET /rest/2.0/xpan/file?method=list（web 端同源接口，path 寻址无需逐级 FileId）。
//
// TODO 联调：downloadUrl（需申请 download 权限 scope）联调确认；列表/配额按官方文档字段实现，
// 单测以 mock 覆盖解析逻辑。
type clientBaidu struct {
	drive Drive
	hc    httpDoer
}

func (c *clientBaidu) Type() string { return "baidu" }

const apiBaidu = "https://pan.baidu.com/rest/2.0"

func (c *clientBaidu) authHeader() map[string]string {
	h := jsonHeaders()
	if c.drive.Token != "" {
		h["Authorization"] = "Bearer " + c.drive.Token
	}
	return h
}

// rawBaiduItem xpan file list 条目。isdir=1 目录；fs_id_download 提供下载起点。
type rawBaiduItem struct {
	FsID  json.Number `json:"fs_id"`
	Path  string      `json:"path"`
	Name  string      `json:"server_filename"`
	IsDir int         `json:"isdir"`
	Size  json.Number `json:"size"`
}

func (it rawBaiduItem) isDir() bool { return it.IsDir == 1 }

func (c *clientBaidu) rawList(ctx context.Context, dir string) ([]rawBaiduItem, error) {
	var out []rawBaiduItem
	const limit = 1000
	for start := 0; ; start += limit {
		u := fmt.Sprintf("%s/xpan/file?method=list&dir=%s&order=name&limit=%d&start=%d&web=1&channel=dubox",
			apiBaidu, url.QueryEscape(cleanBaiduDir(dir)), limit, start)
		resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("baidu list: HTTP %d", resp.StatusCode)
		}
		var body struct {
			Errno  int            `json:"errno"`
			List   []rawBaiduItem `json:"list"`
			Guides []struct{}     `json:"guides"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			return nil, fmt.Errorf("baidu list parse: %w", err)
		}
		if body.Errno != 0 {
			return nil, fmt.Errorf("baidu list 失败: errno=%d（-6=token 失效）", body.Errno)
		}
		out = append(out, body.List...)
		if len(body.List) < limit {
			return out, nil
		}
		if start >= 100*limit { // 上限 10 万条防异常翻页
			return out, nil
		}
	}
}

// cleanBaiduDir 百度 xpan 的 dir 参数不带尾斜杠、根目录传空。（"/" → ""）
func cleanBaiduDir(dir string) string {
	if dir == "/" {
		return ""
	}
	return strings.TrimSuffix(dir, "/")
}

func (c *clientBaidu) List(ctx context.Context, dir string) ([]File, error) {
	raw, err := c.rawList(ctx, dir)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(raw))
	for _, it := range raw {
		out = append(out, File{
			Name: it.Name, Path: it.Path,
			IsDir: it.isDir(), Size: numToInt64(it.Size),
			PickCode: it.FsID.String(),
		})
	}
	return out, nil
}

func (c *clientBaidu) Test(ctx context.Context) (string, error) {
	// uinfo 抽用户名 + 配额（一次调用双探活）。
	resp, err := c.hc.Do(ctx, "GET", apiBaidu+"/rest/2.0/xpan/nas?method=uinfo", nil, c.authHeader(), "", true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("baidu test: HTTP %d（检查 accessToken / scope）", resp.StatusCode)
	}
	var body struct {
		Errno     int    `json:"errno"`
		BaiduName string `json:"baidu_name"`
		Total     int64  `json:"total"`
		Used      int64  `json:"used"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return "", fmt.Errorf("baidu test parse: %w", err)
	}
	if body.Errno != 0 {
		return "", fmt.Errorf("baidu test 失败: errno=%d", body.Errno)
	}
	return fmt.Sprintf("百度网盘 · %s", body.BaiduName), nil
}

func (c *clientBaidu) Quota(ctx context.Context) (*Quota, error) {
	resp, err := c.hc.Do(ctx, "GET", apiBaidu+"/rest/2.0/xpan/nas?method=uinfo", nil, c.authHeader(), "", true)
	if err != nil || resp.StatusCode >= 300 {
		return &Quota{}, nil
	}
	var body struct {
		Total int64 `json:"total"`
		Used  int64 `json:"used"`
	}
	if json.Unmarshal(resp.Body, &body) != nil {
		return &Quota{}, nil
	}
	return &Quota{Used: body.Used, Total: body.Total}, nil
}

// DownloadURL xpan file?method=locatedownload 换 dlink（带 UA 绑定；302 前置）。
func (c *clientBaidu) DownloadURL(ctx context.Context, cloudPath string) (string, error) {
	// locatedownload 接受 fs_id（List 已回填 PickCode）；空 fs_id → 先按路径查一次。
	fsID := ""
	if segs := splitSegments(cloudPath); len(segs) > 0 {
		parent := "/" + strings.Join(segs[:len(segs)-1], "/")
		name := segs[len(segs)-1]
		raw, err := c.rawList(ctx, parent)
		if err != nil {
			return "", err
		}
		for _, it := range raw {
			if !it.isDir() && it.Name == name {
				fsID = it.FsID.String()
				break
			}
		}
	}
	if fsID == "" {
		return "", fmt.Errorf("百度网盘未找到文件: %s", cloudPath)
	}
	u := apiBaidu + "/rest/2.0/xpan/multifile?method=locatedownload&fsids=[" + url.QueryEscape(fsID) + "]"
	resp, err := c.hc.Do(ctx, "GET", u, nil, c.authHeader(), "", true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("baidu download: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Errno int `json:"errno"`
		List  []struct {
			Dlink string `json:"dlink"`
		} `json:"list"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return "", err
	}
	if body.Errno != 0 || len(body.List) == 0 || body.List[0].Dlink == "" {
		return "", fmt.Errorf("baidu download 失败: errno=%d（可能缺 download scope）", body.Errno)
	}
	return body.List[0].Dlink, nil
}

var _ Client = (*clientBaidu)(nil)
