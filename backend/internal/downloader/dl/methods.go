package dl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// listFields 是 get_torrents_status 请求的字段集。
var listFields = []string{
	"hash", "name", "progress", "ratio", "time_added", "save_path", "label",
	"state", "total_size", "upload_payload_rate", "download_payload_rate",
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
	if opts.Category != "" {
		if hash := extractAddHash(raw); hash != "" {
			_, _ = c.request(ctx, "label.set_torrent", []any{hash, opts.Category})
		}
	}
	return nil
}

// extractAddHash 解析 add_torrent_file/url 的返回 [ok, hashOrError]，成功返回 hash。
func extractAddHash(raw json.RawMessage) string {
	var r []json.RawMessage
	if err := json.Unmarshal(raw, &r); err != nil || len(r) < 2 {
		return ""
	}
	var ok bool
	if err := json.Unmarshal(r[0], &ok); err != nil || !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(r[1], &s); err != nil {
		return ""
	}
	return s
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
