package qb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// CachedStats 从已同步的 ServerState 直接构造全局速率（不触发新 Sync 请求）。
// broadcaster 在 aggregate 里已调 ListTorrents→Sync，复用此缓存零额外开销。
func (c *Client) CachedStats() *downloader.GlobalStats {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return &downloader.GlobalStats{
		DLRate: num(c.state.ServerState["dl_info_speed"]),
		UPRate: num(c.state.ServerState["up_info_speed"]),
	}
}

// AddTorrentFile 上传 .torrent 文件添加种子（multipart/form-data）。
// qB 的 /api/v2/torrents/add 同一端点同时支持 urls 与 multipart 的 torrents 字段。
func (c *Client) AddTorrentFile(ctx context.Context, opts downloader.AddOptions) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("torrents", safeFileName(opts.FileName))
	if err != nil {
		return err
	}
	if _, err := fw.Write(opts.FileContent); err != nil {
		return err
	}
	if opts.Category != "" {
		_ = w.WriteField("category", opts.Category)
	}
	if opts.SavePath != "" {
		_ = w.WriteField("savepath", opts.SavePath)
	}
	if len(opts.Tags) > 0 {
		_ = w.WriteField("tags", strings.Join(opts.Tags, ","))
	}
	if opts.Paused {
		if c.ge5() {
			_ = w.WriteField("stopped", "true")
		} else {
			_ = w.WriteField("paused", "true")
		}
	}
	if opts.UploadLimit > 0 {
		_ = w.WriteField("upLimit", strconv.FormatInt(opts.UploadLimit, 10))
	}
	if opts.DownloadLimit > 0 {
		_ = w.WriteField("dlLimit", strconv.FormatInt(opts.DownloadLimit, 10))
	}
	if err := w.Close(); err != nil {
		return err
	}
	data := buf.Bytes()
	ct := w.FormDataContentType()
	status, b, err := c.doBody(ctx, http.MethodPost, "/api/v2/torrents/add",
		func() io.Reader { return bytes.NewReader(data) }, ct)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("add(file) status %d", status)
	}
	if addFailed(b) {
		return fmt.Errorf("add(file) failed: 下载器拒绝添加（重复种子/无效文件/保存路径不存在？）")
	}
	return nil
}

func safeFileName(s string) string {
	if s == "" {
		return "file.torrent"
	}
	// 剥离路径分隔符（/ 与 \，防 "../../../etc/passwd" 或 "..\..\evil"）+
	// 去控制字符（防 CRLF 污染 multipart filename 头）。不用 filepath.Base：
	// 文件名来自用户上传，可能含跨平台混合分隔符，手动剥两者更稳。
	if i := strings.LastIndexAny(s, `/\\`); i >= 0 {
		s = s[i+1:]
	}
	name := strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
	if name == "" || name == "." {
		return "file.torrent"
	}
	return name
}

// —— 分类 / 标签 ——

// ListCategories 返回同步状态里的分类名（零额外请求）。
func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	if err := c.Sync(ctx); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	out := make([]string, 0, len(c.state.Categories))
	for name := range c.state.Categories {
		out = append(out, name)
	}
	return out, nil
}

// ListTags 返回同步状态里的标签（零额外请求）。
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	if err := c.Sync(ctx); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	out := make([]string, len(c.state.Tags))
	copy(out, c.state.Tags)
	return out, nil
}

// checkWriteOK qB 的部分写操作（addTags / removeTags）HTTP 恒 200，成败只在响应正文里回
// "ok." / "fail."；只看状态码就会把「下载器根本没改」报成成功（标签是刷流/删种/限流的分流依据）。
// 另外 sid 过期时 qB 会返回 200 + 登录页 HTML，同样必须算失败。空正文（多数写端点）照旧算成功。
func checkWriteOK(op string, body []byte) error {
	s := strings.TrimSpace(string(body))
	switch {
	case s == "", strings.EqualFold(s, "ok"), strings.EqualFold(s, "ok."):
		return nil
	default:
		preview := s
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		return fmt.Errorf("%s: 下载器未确认成功，返回 %q（fail.=标签/哈希无效；HTML=会话已过期，需重新登录 WebUI）", op, preview)
	}
}

// AddTags 追加标签（tags 逗号分隔）。
func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("tags", strings.Join(tags, ","))
	status, body, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/addTags",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("addTags status %d", status)
	}
	return checkWriteOK("addTags", body)
}

// RemoveTags 移除标签。
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("tags", strings.Join(tags, ","))
	status, body, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/removeTags",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("removeTags status %d", status)
	}
	return checkWriteOK("removeTags", body)
}

// SetTags 覆盖式设置标签。qB 无原生覆盖接口，按 diff 实现：
// 取每个任务当前标签 → 移除「旧有新无」→ 追加「新有旧无」。hashes 为空（=all）时退化为单纯追加。
func (c *Client) SetTags(ctx context.Context, hashes []string, tags []string) error {
	if len(hashes) == 0 {
		return c.AddTags(ctx, nil, tags)
	}
	newSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		newSet[t] = true
	}
	for _, h := range hashes {
		old, err := c.tagsOf(ctx, h)
		if err != nil {
			return err
		}
		var remove, add []string
		oldSet := make(map[string]bool, len(old))
		for _, t := range old {
			oldSet[t] = true
			if !newSet[t] {
				remove = append(remove, t)
			}
		}
		for _, t := range tags {
			if !oldSet[t] {
				add = append(add, t)
			}
		}
		if len(remove) > 0 {
			if err := c.RemoveTags(ctx, []string{h}, remove); err != nil {
				return err
			}
		}
		if len(add) > 0 {
			if err := c.AddTags(ctx, []string{h}, add); err != nil {
				return err
			}
		}
	}
	return nil
}

// tagsOf 取单个任务当前标签（从同步状态）。
func (c *Client) tagsOf(ctx context.Context, hash string) ([]string, error) {
	if err := c.Sync(ctx); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	m := c.state.Torrents[hash]
	if m == nil {
		return nil, nil
	}
	return splitTags(str(m["tags"])), nil
}

// —— 生命周期 ——

// Recheck 强制重新校验。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "/api/v2/torrents/recheck", hashes)
}

// Reannounce 重新汇报。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "/api/v2/torrents/reannounce", hashes)
}

// SetLocation 移动保存路径。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("location", location)
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/setLocation",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setLocation status %d", status)
	}
	return nil
}

// —— 限速（bytes/s，0=不限）——

// SetLimits 设置单个/多个任务上下行限速。qB 的 0 即取消限制，故始终发送。
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	h := joinHashes(hashes)
	for _, op := range []struct {
		path  string
		value int64
	}{
		{"/api/v2/torrents/setUploadLimit", limits.UploadLimit},
		{"/api/v2/torrents/setDownloadLimit", limits.DownloadLimit},
	} {
		form := url.Values{}
		form.Set("hashes", h)
		form.Set("limit", strconv.FormatInt(op.value, 10))
		status, _, err := c.do(ctx, http.MethodPost, op.path,
			form.Encode(), "application/x-www-form-urlencoded")
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("%s status %d", op.path, status)
		}
	}
	return nil
}

// GetSpeedLimits 读取全局速度上限（来自 app/preferences 的 dl_limit/up_limit，bytes/s）。
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/app/preferences", "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("preferences status %d", status)
	}
	var p struct {
		DLLimit int64 `json:"dl_limit"`
		UPLimit int64 `json:"up_limit"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse preferences: %w", err)
	}
	return &downloader.Limits{DownloadLimit: p.DLLimit, UploadLimit: p.UPLimit}, nil
}

// SetSpeedLimits 设置全局速度上限（bytes/s，0=不限）。
// qB transfer 端点单位为 bytes/s（与单种子一致），无需换算。
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	for _, op := range []struct {
		path  string
		value int64
	}{
		{"/api/v2/transfer/setUploadLimit", limits.UploadLimit},
		{"/api/v2/transfer/setDownloadLimit", limits.DownloadLimit},
	} {
		form := url.Values{}
		form.Set("limit", strconv.FormatInt(op.value, 10))
		status, _, err := c.do(ctx, http.MethodPost, op.path,
			form.Encode(), "application/x-www-form-urlencoded")
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("%s status %d", op.path, status)
		}
	}
	return nil
}
