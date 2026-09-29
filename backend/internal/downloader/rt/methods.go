package rt

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// listResp 是 httprpc mode=list 的响应。
type listResp struct {
	T map[string][]any `json:"t"`
}

// ListTorrents 列出全部任务。
func (c *Client) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	status, b, err := c.rpcForm(ctx, url.Values{"mode": {"list"}})
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("list status %d", status)
	}
	var r listResp
	if err := ensureJSON(b, &r); err != nil {
		return nil, fmt.Errorf("parse list: %w", err)
	}
	out := make([]downloader.Torrent, 0, len(r.T))
	for hash, row := range r.T {
		out = append(out, toTorrent(hash, row))
	}
	return out, nil
}

// AddTorrent 添加种子。
func (c *Client) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	fields := map[string]string{"json": "1"}
	if opts.SavePath != "" {
		fields["dir_edit"] = opts.SavePath
	}
	if opts.Paused {
		fields["torrents_start_stopped"] = "1"
	}
	if opts.Category != "" {
		fields["label"] = opts.Category
	}
	// 二轮 P2-3：ptpsrc 标签随 add 下发（rutorrent label 字段支持逗号多值；
	// 此前只发 Category，标签丢失 → 整理 Path A 降级）。map[string]string 无 nil，
	// 直接读零值拼接。
	if len(opts.Tags) > 0 {
		extra := strings.Join(opts.Tags, ",")
		if cur := fields["label"]; cur != "" {
			fields["label"] = cur + "," + extra
		} else {
			fields["label"] = extra
		}
	}

	if len(opts.FileContent) > 0 && opts.URL == "" {
		status, b, err := c.postMultipart(ctx, "/php/addtorrent.php", fields, "torrent_file", opts.FileName, opts.FileContent)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("addtorrent status %d", status)
		}
		return checkAddResult(b)
	}
	// URL / magnet
	vals := url.Values{}
	for k, v := range fields {
		vals.Set(k, v)
	}
	vals.Set("url", opts.URL)
	status, b, err := c.do(ctx, "POST", "/php/addtorrent.php", strings.NewReader(vals.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("addtorrent status %d", status)
	}
	return checkAddResult(b)
}

func checkAddResult(b []byte) error {
	var r struct {
		Result string `json:"result"`
	}
	if err := ensureJSON(b, &r); err != nil {
		// 非 JSON 也当成功（部分版本）
		return nil
	}
	if r.Result != "" && r.Result != "Success" {
		return fmt.Errorf("addtorrent: %s", r.Result)
	}
	return nil
}

// Pause mode=pause。
func (c *Client) Pause(ctx context.Context, hashes []string) error {
	return c.modeHashes(ctx, "pause", hashes)
}

// Resume → mode=post。
func (c *Client) Resume(ctx context.Context, hashes []string) error {
	for _, h := range hashes {
		status, _, err := c.rpcForm(ctx, url.Values{"mode": {"post"}, "hash": {strings.ToUpper(h)}})
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("resume status %d", status)
		}
	}
	return nil
}

// Delete 删除；deleteFiles 用 XML multicall erase + delete_tied。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	for _, h := range hashes {
		up := strings.ToUpper(h)
		if deleteFiles {
			xml := buildMulticall([][2]any{
				{"d.custom5.set", []string{up, "1"}},
				{"d.delete_tied", []string{up}},
				{"d.erase", []string{up}},
			})
			status, _, err := c.rpcXML(ctx, xml)
			if err != nil {
				return err
			}
			if status != 200 {
				return fmt.Errorf("erase status %d", status)
			}
			continue
		}
		status, _, err := c.rpcForm(ctx, url.Values{"mode": {"remove"}, "hash": {up}})
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("remove status %d", status)
		}
	}
	return nil
}

// SetCategory → d.custom1.set (label)。
func (c *Client) SetCategory(ctx context.Context, hashes []string, category string) error {
	for _, h := range hashes {
		up := strings.ToUpper(h)
		xml := buildMulticall([][2]any{
			{"d.custom1.set", []string{up, category}},
		})
		status, _, err := c.rpcXML(ctx, xml)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("set label status %d", status)
		}
	}
	return nil
}

// GlobalStats 汇总 list 速率。
func (c *Client) GlobalStats(ctx context.Context) (*downloader.GlobalStats, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	s := &downloader.GlobalStats{}
	for _, t := range list {
		s.DLRate += t.DLRate
		s.UPRate += t.UPRate
		s.TotalSize += t.Size
	}
	return s, nil
}

func (c *Client) modeHashes(ctx context.Context, mode string, hashes []string) error {
	for _, h := range hashes {
		status, _, err := c.rpcForm(ctx, url.Values{"mode": {mode}, "hash": {strings.ToUpper(h)}})
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("%s status %d", mode, status)
		}
	}
	return nil
}
