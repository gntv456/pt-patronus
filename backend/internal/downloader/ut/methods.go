// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ut

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// TestConnection 取 token 并 list=1，两者成功即连通。
func (c *Client) TestConnection(ctx context.Context) error {
	if err := c.fetchToken(ctx); err != nil {
		return err
	}
	status, _, err := c.do(ctx, "", kv{"list", "1"})
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("list status %d", status)
	}
	return nil
}

// listResp 是 /gui/?list=1 的响应。torrents 为元组数组（混 string/number）。
type listResp struct {
	Torrents [][]any `json:"torrents"`
}

// fetchList 拉取并解析 list=1。
func (c *Client) fetchList(ctx context.Context) ([][]any, error) {
	status, b, err := c.do(ctx, "", kv{"list", "1"})
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("list status %d", status)
	}
	var r listResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse list: %w", err)
	}
	return r.Torrents, nil
}

// ListTorrents 列出全部任务。
func (c *Client) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	rows, err := c.fetchList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]downloader.Torrent, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTorrent(row))
	}
	return out, nil
}

// AddTorrent 添加种子（magnet/链接 或 .torrent 字节）。
// uT 的 add-url/add-file 仅接受保存路径（path，相对 WebUI 配置的下载目录）；分类/暂停/限速
// 需 add 后凭 infoHash 调 setprops/pause。磁力链接可本地解析 btih，故对其 post-set；
// .torrent 文件无法本地算 hash（uT 盲返回不回 hash），仅 add。
func (c *Client) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	if len(opts.FileContent) > 0 && opts.URL == "" {
		// add-file（multipart）
		extra := []kv{{"download_dir", "0"}}
		if opts.SavePath != "" {
			extra = append(extra, kv{"path", opts.SavePath})
		}
		status, _, err := c.doPostMultipart(ctx, "add-file", extra, opts.FileName, opts.FileContent)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("add-file status %d", status)
		}
		// 二轮 P2-2 修复：.torrent 本地算 infohash 后补设 label/paused/限速——
		// 此前 FileContent 路径只 add 不 set，订阅/RSS 经 cookie 下载改写后必走
		// 此路径，Category 丢失 → 整理白名单失配（链路断）。uT 盲返回不带 hash，
		// 用最小 bencode 解析 info 字典算 btih（无需等元数据握手）。
		if h := torrentInfoHash(opts.FileContent); h != "" {
			if err := c.postAddProps(ctx, h, opts); err != nil {
				return err
			}
		}
		return nil
	}
	// add-url（magnet/http 链接）
	extra := []kv{{"download_dir", "0"}}
	if opts.SavePath != "" {
		extra = append(extra, kv{"path", opts.SavePath})
	}
	extra = append(extra, kv{"s", opts.URL})
	status, _, err := c.do(ctx, "add-url", extra...)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("add-url status %d", status)
	}
	// 磁力链接：本地解析 btih 后 post-set label/paused/limit（uT add 不直接支持）
	if h := magnetInfoHash(opts.URL); h != "" {
		if err := c.postAddProps(ctx, h, opts); err != nil {
			return err
		}
	}
	return nil
}

// postAddProps 对刚添加的磁力种子按 opts 设分类/暂停/上传限速（uT add 后凭本地 btih）。
func (c *Client) postAddProps(ctx context.Context, hash string, opts downloader.AddOptions) error {
	if opts.Category != "" {
		if err := c.SetCategory(ctx, []string{hash}, opts.Category); err != nil {
			return err
		}
	}
	if opts.Paused {
		if err := c.Pause(ctx, []string{hash}); err != nil {
			return err
		}
	}
	if opts.UploadLimit > 0 {
		if err := c.setPropsOne(ctx, hash,
			kv{"s", "rate_upload_max"}, kv{"v", strconv.FormatInt(opts.UploadLimit, 10)}); err != nil {
			return err
		}
	}
	return nil
}

// Pause 暂停（action=pause）。
func (c *Client) Pause(ctx context.Context, hashes []string) error {
	return c.actionHashes(ctx, "pause", hashes)
}

// Resume 恢复（action=start）。
func (c *Client) Resume(ctx context.Context, hashes []string) error {
	return c.actionHashes(ctx, "start", hashes)
}

// Delete 删除任务（deleteFiles 选 removedatatorrent/removetorrent）。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	action := "removetorrent"
	if deleteFiles {
		action = "removedatatorrent"
	}
	return c.actionHashes(ctx, action, hashes)
}

// SetCategory 设置分类（uT label）。逐个 hash setprops s=label v=category。
func (c *Client) SetCategory(ctx context.Context, hashes []string, category string) error {
	for _, h := range hashes {
		if err := c.setPropsOne(ctx, h, kv{"s", "label"}, kv{"v", category}); err != nil {
			return err
		}
	}
	return nil
}

// GlobalStats 求和 list=1 各种子的上下行速率与总体积（uT 无全局速度端点）。
func (c *Client) GlobalStats(ctx context.Context) (*downloader.GlobalStats, error) {
	rows, err := c.fetchList(ctx)
	if err != nil {
		return nil, err
	}
	var s downloader.GlobalStats
	for _, row := range rows {
		s.UPRate += numAt(row, 8)
		s.DLRate += numAt(row, 9)
		s.TotalSize += numAt(row, 3)
	}
	return &s, nil
}

// —— 内部 helpers ——

// hashParams 生成多个 {hash, h} 对（uT 接受重复 hash 参数表示多任务）。
func hashParams(hashes []string) []kv {
	out := make([]kv, 0, len(hashes))
	for _, h := range hashes {
		out = append(out, kv{"hash", h})
	}
	return out
}

// actionHashes 对一组 hash 发 action（GET action=...&hash=h1&hash=h2）。
func (c *Client) actionHashes(ctx context.Context, action string, hashes []string) error {
	status, _, err := c.do(ctx, action, hashParams(hashes)...)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("%s status %d", action, status)
	}
	return nil
}

// setPropsOne 对单个种子设置一组属性（s/v 对）：action=setprops&hash=h&s=K&v=V[&s=K2&v=V2]。
func (c *Client) setPropsOne(ctx context.Context, hash string, props ...kv) error {
	extra := make([]kv, 0, 1+len(props))
	extra = append(extra, kv{"hash", hash})
	extra = append(extra, props...)
	status, _, err := c.do(ctx, "setprops", extra...)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setprops status %d", status)
	}
	return nil
}
