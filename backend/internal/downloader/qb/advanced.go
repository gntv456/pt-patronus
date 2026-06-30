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
	status, _, err := c.doBody(ctx, http.MethodPost, "/api/v2/torrents/add",
		func() io.Reader { return bytes.NewReader(data) }, ct)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("add(file) status %d", status)
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

// AddTags 追加标签（tags 逗号分隔）。
func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("tags", strings.Join(tags, ","))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/addTags",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("addTags status %d", status)
	}
	return nil
}

// RemoveTags 移除标签。
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("tags", strings.Join(tags, ","))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/removeTags",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("removeTags status %d", status)
	}
	return nil
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

// —— 文件优先级 ——

// ListFiles 列出某任务内文件。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/torrents/files?hash="+url.QueryEscape(hash), "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("files status %d", status)
	}
	var fs []struct {
		Index    int     `json:"index"`
		Name     string  `json:"name"`
		Size     int64   `json:"size"`
		Progress float64 `json:"progress"`
		Priority int     `json:"priority"`
	}
	if err := json.Unmarshal(b, &fs); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(fs))
	for _, f := range fs {
		out = append(out, downloader.TorrentFile{
			Index: f.Index, Name: f.Name, Size: f.Size, Progress: f.Progress, Priority: f.Priority,
		})
	}
	return out, nil
}

// SetFilePriority 设置文件优先级。qB 字段名为 id（值为逗号分隔 file index），priority ∈ {0,1,2,7}。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	ids := make([]string, 0, len(indices))
	for _, i := range indices {
		ids = append(ids, strconv.Itoa(i))
	}
	form := url.Values{}
	form.Set("hash", hash)
	form.Set("id", strings.Join(ids, ","))
	form.Set("priority", strconv.Itoa(priority))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/filePrio",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("filePrio status %d", status)
	}
	return nil
}

// —— Tracker ——

// ListTrackers 列出某任务的 tracker（过滤 qB 返回的 ** [DHT]/** [PEX]/** [LSD] 虚拟项）。
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/torrents/trackers?hash="+url.QueryEscape(hash), "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("trackers status %d", status)
	}
	var raw []struct {
		URL    string `json:"url"`
		Tier   int    `json:"tier"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse trackers: %w", err)
	}
	out := make([]downloader.Tracker, 0, len(raw))
	for _, t := range raw {
		if strings.HasPrefix(t.URL, "**") {
			continue
		}
		out = append(out, downloader.Tracker{URL: t.URL, Tier: t.Tier, Status: trackerStatus(t.Status)})
	}
	return out, nil
}

func trackerStatus(s int) string {
	switch s {
	case 0:
		return "disabled"
	case 1:
		return "not_contacted"
	case 2:
		return "working"
	case 3:
		return "updating"
	case 4:
		return "not_working"
	default:
		return "unknown"
	}
}

// AddTrackers 追加 tracker（urls 换行分隔）。
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	form := url.Values{}
	form.Set("hash", hash)
	form.Set("urls", strings.Join(urls, "\n"))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/addTrackers",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("addTrackers status %d", status)
	}
	return nil
}

// RemoveTrackers 移除 tracker（urls 换行分隔）。
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	form := url.Values{}
	form.Set("hash", hash)
	form.Set("urls", strings.Join(urls, "\n"))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/removeTrackers",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("removeTrackers status %d", status)
	}
	return nil
}

// —— 做种限制 ——

// SetShareLimits 设置分享比率/做种时长上限。
// qB setShareLimits：ratioLimit/seedingTimeLimit 均支持 -2=无限 -1=全局默认 >=0=具体值。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("ratioLimit", strconv.FormatFloat(limits.RatioLimit, 'f', -1, 64))
	form.Set("seedingTimeLimit", strconv.Itoa(limits.SeedingTimeLimit))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/setShareLimits",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setShareLimits status %d", status)
	}
	return nil
}

// —— 队列优先级 ——

// SetQueuePriority 调整队列优先级（top=置顶 up=上移 down=下移 bottom=置底）。
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	path, ok := map[string]string{
		"top":    "/api/v2/torrents/topPriority",
		"up":     "/api/v2/torrents/increasePrio",
		"down":   "/api/v2/torrents/decreasePrio",
		"bottom": "/api/v2/torrents/bottomPriority",
	}[action]
	if !ok {
		return fmt.Errorf("invalid queue action: %s", action)
	}
	return c.postHashes(ctx, path, hashes)
}

// —— 强制开始 ——

// SetForceStart 强制开始/取消强制。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("value", strconv.FormatBool(enable))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/setForceStart",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setForceStart status %d", status)
	}
	return nil
}

// —— 顺序下载 / 首尾优先（toggle）——

// ToggleSequentialDownload 切换顺序下载。
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "/api/v2/torrents/toggleSequentialDownload", hashes)
}

// ToggleFirstLastPiece 切换首尾优先。
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "/api/v2/torrents/toggleFirstLastPiecePrio", hashes)
}

// —— Peers ——

// ListPeers 列出某任务当前连接的对等体（/sync/torrentPeers）。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/sync/torrentPeers?hash="+url.QueryEscape(hash), "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("torrentPeers status %d", status)
	}
	var resp struct {
		Peers map[string]struct {
			Client   string  `json:"client"`
			Progress float64 `json:"progress"`
			DLSpeed  int64   `json:"dl_speed"`
			UPSpeed  int64   `json:"up_speed"`
			Flags    string  `json:"flags"`
			Country  string  `json:"country"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("parse torrentPeers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(resp.Peers))
	for ip, p := range resp.Peers {
		out = append(out, downloader.Peer{
			IP:       ip,
			Client:   p.Client,
			Progress: p.Progress,
			DLRate:   p.DLSpeed,
			UPRate:   p.UPSpeed,
			Flags:    p.Flags,
		})
	}
	return out, nil
}

// —— 分类增删 ——

// CreateCategory 新建分类（可绑定保存路径）。
func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	form := url.Values{}
	form.Set("category", name)
	form.Set("savePath", savePath)
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/createCategory",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("createCategory status %d", status)
	}
	return nil
}

// EditCategory 编辑分类保存路径。
func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	form := url.Values{}
	form.Set("category", name)
	form.Set("savePath", savePath)
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/editCategory",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("editCategory status %d", status)
	}
	return nil
}

// DeleteCategories 删除分类（categories 换行分隔）。
func (c *Client) DeleteCategories(ctx context.Context, names []string) error {
	form := url.Values{}
	form.Set("categories", strings.Join(names, "\n"))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/removeCategories",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("removeCategories status %d", status)
	}
	return nil
}
