package dl

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// —— 生命周期 ——

// Recheck → core.force_recheck。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	_, err := c.request(ctx, "core.force_recheck", []any{hashes})
	return err
}

// Reannounce → core.force_reannounce。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	_, err := c.request(ctx, "core.force_reannounce", []any{hashes})
	return err
}

// SetLocation → core.move_storage（单 hash，循环）。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	for _, h := range hashes {
		if _, err := c.request(ctx, "core.move_storage", []any{h, location}); err != nil {
			return err
		}
	}
	return nil
}

// Deluge 无 force-start 概念。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

// —— 分类 / 标签（Deluge label 为 Label 插件单值，无原生多标签/CRUD）——

// ListCategories 去重所有种子的 label（复用 ListTorrents）。
func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range list {
		if t.Category != "" && !seen[t.Category] {
			seen[t.Category] = true
			out = append(out, t.Category)
		}
	}
	return out, nil
}

func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	return nil, downloader.ErrNotImplemented
}
func (c *Client) SetTags(ctx context.Context, hashes []string, tags []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) DeleteCategories(ctx context.Context, names []string) error {
	return downloader.ErrNotImplemented
}

// —— 做种限制（core.set_torrent options）——
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	opts := map[string]any{}
	if limits.RatioLimit >= 0 {
		opts["stop_ratio"] = limits.RatioLimit
		opts["stop_at_ratio"] = true
	} else {
		opts["stop_at_ratio"] = false
	}
	_, err := c.request(ctx, "core.set_torrent", []any{hashes, opts})
	return err
}

// —— 队列优先级（core.queue_top/up/down/bottom）——
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	var method string
	switch action {
	case "top":
		method = "core.queue_top"
	case "up":
		method = "core.queue_up"
	case "down":
		method = "core.queue_down"
	case "bottom":
		method = "core.queue_bottom"
	default:
		return fmt.Errorf("invalid queue action: %s", action)
	}
	_, err := c.request(ctx, method, []any{hashes})
	return err
}

// —— 顺序下载 / 首尾优先（Deluge 有选项但 toggle 需读当前状态，暂不支持）——
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// —— 限速（KB/s）——
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	opts := map[string]any{
		"max_upload_speed":   bytesToKB(limits.UploadLimit),
		"max_download_speed": bytesToKB(limits.DownloadLimit),
	}
	_, err := c.request(ctx, "core.set_torrent", []any{hashes, opts})
	return err
}

// GetSpeedLimits 读 core.get_config 的 max_*_speed（KB → bytes/s；0=不限）。
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	raw, err := c.request(ctx, "core.get_config", nil)
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse get_config: %w", err)
	}
	return &downloader.Limits{
		UploadLimit:   kbToBytes(toInt64(cfg["max_upload_speed"])),
		DownloadLimit: kbToBytes(toInt64(cfg["max_download_speed"])),
	}, nil
}

// SetSpeedLimits 全局速度上限（core.set_config）。
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	_, err := c.request(ctx, "core.set_config", []any{map[string]any{
		"max_upload_speed":   bytesToKB(limits.UploadLimit),
		"max_download_speed": bytesToKB(limits.DownloadLimit),
	}})
	return err
}

// —— 文件优先级 ——

// ListFiles → core.get_torrent_status。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	raw, err := c.request(ctx, "core.get_torrent_status",
		[]any{hash, []string{"files", "file_progress", "file_priorities"}})
	if err != nil {
		return nil, err
	}
	var s struct {
		Files []struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		} `json:"files"`
		FileProgress   []float64 `json:"file_progress"`
		FilePriorities []int     `json:"file_priorities"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse torrent_status files: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(s.Files))
	for i, f := range s.Files {
		var prog float64
		if i < len(s.FileProgress) {
			prog = s.FileProgress[i]
		}
		var prio int
		if i < len(s.FilePriorities) {
			prio = s.FilePriorities[i]
		}
		out = append(out, downloader.TorrentFile{
			Index: i, Name: f.Path, Size: f.Size, Progress: prog, Priority: prio,
		})
	}
	return out, nil
}

// SetFilePriority 读当前 file_priorities，改指定 index，整体回写（core.set_torrent_file_priorities）。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	raw, err := c.request(ctx, "core.get_torrent_status",
		[]any{hash, []string{"file_priorities"}})
	if err != nil {
		return err
	}
	var s struct {
		FilePriorities []int `json:"file_priorities"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("parse file_priorities: %w", err)
	}
	prios := append([]int(nil), s.FilePriorities...)
	for _, idx := range indices {
		if idx >= 0 && idx < len(prios) {
			prios[idx] = priority
		}
	}
	_, err = c.request(ctx, "core.set_torrent_file_priorities", []any{hash, prios})
	return err
}

// —— Tracker（Deluge get/set_torrent_trackers）——

// ListTrackers → core.get_torrent_status trackers。
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	cur, err := c.trackerList(ctx, hash)
	if err != nil {
		return nil, err
	}
	out := make([]downloader.Tracker, 0, len(cur))
	for _, t := range cur {
		u, _ := t["url"].(string)
		tier := int(toInt64(t["tier"]))
		out = append(out, downloader.Tracker{URL: u, Tier: tier})
	}
	return out, nil
}

// AddTrackers：get → 追加缺失 → set。
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	cur, err := c.trackerList(ctx, hash)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, t := range cur {
		u, _ := t["url"].(string)
		have[u] = true
	}
	for _, u := range urls {
		if !have[u] {
			cur = append(cur, map[string]any{"url": u, "tier": 0})
		}
	}
	return c.setTrackers(ctx, hash, cur)
}

// RemoveTrackers：get → 过滤 → set。
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	cur, err := c.trackerList(ctx, hash)
	if err != nil {
		return err
	}
	rm := map[string]bool{}
	for _, u := range urls {
		rm[u] = true
	}
	var kept []map[string]any
	for _, t := range cur {
		u, _ := t["url"].(string)
		if !rm[u] {
			kept = append(kept, t)
		}
	}
	return c.setTrackers(ctx, hash, kept)
}

func (c *Client) trackerList(ctx context.Context, hash string) ([]map[string]any, error) {
	raw, err := c.request(ctx, "core.get_torrent_status", []any{hash, []string{"trackers"}})
	if err != nil {
		return nil, err
	}
	var s struct {
		Trackers []map[string]any `json:"trackers"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse trackers: %w", err)
	}
	return s.Trackers, nil
}

func (c *Client) setTrackers(ctx context.Context, hash string, trackers []map[string]any) error {
	_, err := c.request(ctx, "core.set_torrent_trackers", []any{hash, trackers})
	return err
}

// —— Peers ——

// ListPeers → core.get_torrent_status peers。
// 联调点：Deluge peers 元组 [ip, port, client, ...]，字段位置以实测为准。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	raw, err := c.request(ctx, "core.get_torrent_status", []any{hash, []string{"peers"}})
	if err != nil {
		return nil, err
	}
	var s struct {
		Peers [][]any `json:"peers"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse peers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(s.Peers))
	for _, p := range s.Peers {
		out = append(out, downloader.Peer{
			IP:     anyStr(atIdx(p, 0)),
			Client: anyStr(atIdx(p, 2)),
		})
	}
	return out, nil
}

// —— 小工具 ——

func atIdx(row []any, i int) any {
	if i >= len(row) {
		return nil
	}
	return row[i]
}

func anyStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// toInt64 把 any（float64/string/int）转 int64。
func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return int64(f)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
