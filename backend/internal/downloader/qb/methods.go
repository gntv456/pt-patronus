package qb

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// ListTorrents 同步并返回任务列表。
func (c *Client) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	if err := c.Sync(ctx); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	out := make([]downloader.Torrent, 0, len(c.state.Torrents))
	for h, m := range c.state.Torrents {
		out = append(out, toTorrent(h, m))
	}
	return out, nil
}

// GlobalStats 从同步状态取全局速率。
func (c *Client) GlobalStats(ctx context.Context) (*downloader.GlobalStats, error) {
	if err := c.Sync(ctx); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return &downloader.GlobalStats{
		DLRate: num(c.state.ServerState["dl_info_speed"]),
		UPRate: num(c.state.ServerState["up_info_speed"]),
	}, nil
}

// AddTorrent 添加种子（magnet/链接 或 .torrent 字节）。
func (c *Client) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	if len(opts.FileContent) > 0 && opts.URL == "" {
		return c.AddTorrentFile(ctx, opts)
	}
	form := url.Values{}
	form.Set("urls", opts.URL)
	if opts.Category != "" {
		form.Set("category", opts.Category)
	}
	if opts.SavePath != "" {
		form.Set("savepath", opts.SavePath)
	}
	if len(opts.Tags) > 0 {
		form.Set("tags", strings.Join(opts.Tags, ","))
	}
	if opts.Paused {
		if c.ge5() {
			form.Set("stopped", "true")
		} else {
			form.Set("paused", "true")
		}
	}
	if opts.UploadLimit > 0 {
		form.Set("upLimit", strconv.FormatInt(opts.UploadLimit, 10))
	}
	if opts.DownloadLimit > 0 {
		form.Set("dlLimit", strconv.FormatInt(opts.DownloadLimit, 10))
	}
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/add",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("add status %d", status)
	}
	return nil
}

// Pause 暂停（qB5 用 stop）。
func (c *Client) Pause(ctx context.Context, hashes []string) error {
	path := "/api/v2/torrents/pause"
	if c.ge5() {
		path = "/api/v2/torrents/stop"
	}
	return c.postHashes(ctx, path, hashes)
}

// Resume 恢复（qB5 用 start）。
func (c *Client) Resume(ctx context.Context, hashes []string) error {
	path := "/api/v2/torrents/resume"
	if c.ge5() {
		path = "/api/v2/torrents/start"
	}
	return c.postHashes(ctx, path, hashes)
}

// Delete 删除任务。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("deleteFiles", strconv.FormatBool(deleteFiles))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/delete",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("delete status %d", status)
	}
	return nil
}

// SetCategory 设置分类。
func (c *Client) SetCategory(ctx context.Context, hashes []string, category string) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("category", category)
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/setCategory",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setCategory status %d", status)
	}
	return nil
}

// postHashes 对一组 hash 发 POST（hashes 为空表示全部）。
func (c *Client) postHashes(ctx context.Context, path string, hashes []string) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	status, _, err := c.do(ctx, http.MethodPost, path,
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 && status != 204 {
		return fmt.Errorf("%s status %d", path, status)
	}
	return nil
}
