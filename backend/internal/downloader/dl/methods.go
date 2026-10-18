package dl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/downloader"

	"log/slog"
)

// listFields 是 get_torrents_status 请求的字段集。
var listFields = []string{
	"hash", "name", "progress", "ratio", "time_added", "completed_time", "save_path",
	"label", "state", "total_size", "upload_payload_rate", "download_payload_rate",
	"total_uploaded", "total_done", "num_seeds", "num_peers", "total_seeds", "total_peers",
}

// ListTorrents 列出全部任务（core.get_torrents_status 空过滤）。
func (c *Client) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	raw, err := c.request(ctx, "core.get_torrents_status", []any{map[string]any{}, listFields})
	if err != nil {
		return nil, err
	}
	var m map[string]dlTorrent
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse torrents: %w", err)
	}
	out := make([]downloader.Torrent, 0, len(m))
	for _, t := range m {
		out = append(out, toTorrent(t))
	}
	return out, nil
}

// AddTorrent 添加种子（链接 或 .torrent 字节）。
// add 后若指定 Category，解析返回 hash 调 label.set_torrent（需 Label 插件，失败降级忽略）。
func (c *Client) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	dopts := map[string]any{"add_paused": opts.Paused}
	if opts.SavePath != "" {
		dopts["download_location"] = opts.SavePath
	}
	if opts.UploadLimit > 0 {
		dopts["max_upload_speed"] = bytesToKB(opts.UploadLimit)
	}
	if opts.DownloadLimit > 0 {
		dopts["max_download_speed"] = bytesToKB(opts.DownloadLimit)
	}
	var method string
	var params []any
	if len(opts.FileContent) > 0 && opts.URL == "" {
		method = "core.add_torrent_file"
		params = []any{"", base64.StdEncoding.EncodeToString(opts.FileContent), dopts}
	} else {
		method = "core.add_torrent_url"
		params = []any{opts.URL, dopts}
	}
	raw, err := c.request(ctx, method, params)
	if err != nil {
		return err
	}
	// add 失败时 Deluge 返回 [false,"错误原因"]（如 "Duplicate torrent"）——此前只取
	// hash 不校验 ok → 失败被当成功（RSS 标记已下载、辅种记 added，静默丢种不再重试）。
	hash, addErr := parseAddResult(raw)
	if addErr != nil {
		return addErr
	}
	if opts.Category != "" && hash != "" {
		// 二轮 P2-3：Label 插件缺失时 label.set_torrent 报 "Unknown method"——
		// 此前吞错导致 Category 静默丢失 → 整理白名单失配。日志可见（不 fail：
		// 种子已加上，中断反而更糟），给用户明确的修复指引。
		if _, lerr := c.request(ctx, "label.set_torrent", []any{hash, opts.Category}); lerr != nil {
			slog.Warn("deluge: set label failed (Label plugin installed?) — organize whitelist may miss",
				"hash", hash, "label", opts.Category, "err", lerr)
		}
	}
	return nil
}

// parseAddResult 解析 add_torrent_file/url 的返回 [ok, hashOrError]：
// ok=true 时第二元素为 hash；ok=false 时第二元素为错误原因。
func parseAddResult(raw json.RawMessage) (hash string, err error) {
	var r []json.RawMessage
	if e := json.Unmarshal(raw, &r); e != nil || len(r) < 2 {
		return "", fmt.Errorf("deluge add: 响应格式异常: %s", string(raw))
	}
	var ok bool
	if e := json.Unmarshal(r[0], &ok); e != nil {
		return "", fmt.Errorf("deluge add: 响应格式异常: %s", string(raw))
	}
	if !ok {
		var reason string
		_ = json.Unmarshal(r[1], &reason)
		if reason == "" {
			reason = "unknown"
		}
		return "", fmt.Errorf("deluge add failed: %s", reason)
	}
	var s string
	_ = json.Unmarshal(r[1], &s)
	return s, nil
}

// Pause → core.pause_torrent（接受 hash 列表）。
func (c *Client) Pause(ctx context.Context, hashes []string) error {
	_, err := c.request(ctx, "core.pause_torrent", []any{hashes})
	return err
}

// Resume → core.resume_torrent。
func (c *Client) Resume(ctx context.Context, hashes []string) error {
	_, err := c.request(ctx, "core.resume_torrent", []any{hashes})
	return err
}

// Delete → core.remove_torrent（单 hash，循环）。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	for _, h := range hashes {
		if _, err := c.request(ctx, "core.remove_torrent", []any{h, deleteFiles}); err != nil {
			return err
		}
	}
	return nil
}

// SetCategory → label.set_torrent（需 Label 插件）。
func (c *Client) SetCategory(ctx context.Context, hashes []string, category string) error {
	for _, h := range hashes {
		if _, err := c.request(ctx, "label.set_torrent", []any{h, category}); err != nil {
			return err
		}
	}
	return nil
}

// GlobalStats → core.get_session_status。
func (c *Client) GlobalStats(ctx context.Context) (*downloader.GlobalStats, error) {
	raw, err := c.request(ctx, "core.get_session_status",
		[]any{[]string{"upload_rate", "download_rate", "total_upload", "total_download"}})
	if err != nil {
		return nil, err
	}
	var s struct {
		UploadRate    int64 `json:"upload_rate"`
		DownloadRate  int64 `json:"download_rate"`
		TotalUpload   int64 `json:"total_upload"`
		TotalDownload int64 `json:"total_download"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse session status: %w", err)
	}
	return &downloader.GlobalStats{
		UPRate: s.UploadRate, DLRate: s.DownloadRate,
	}, nil
}
