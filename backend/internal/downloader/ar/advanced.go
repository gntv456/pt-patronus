package ar

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// —— 生命周期 ——

// Recheck：Aria2 无原生 recheck。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// Reannounce：Aria2 无原生 reannounce。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// SetLocation：Aria2 可通过 changeOption dir，但不会移动已下文件，语义不完全，暂不实现。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	return downloader.ErrNotImplemented
}

// SetForceStart：不支持。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

// —— 分类 / 标签 ——

func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	return nil, downloader.ErrNotImplemented
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

// —— 做种限制 ——

// SetShareLimits：Aria2 无 seed ratio/time 按任务接口（全局 seed-ratio 可设，任务级不实现）。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	return downloader.ErrNotImplemented
}

// —— 队列（aria2.changePosition）——

func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	// POS_SET=0 how: POS_CUR relative; aria2.changePosition(gid, pos, how)
	// top/bottom：how=POS_SET pos=0 / -1；up/down：POS_CUR ±1
	for _, h := range hashes {
		var pos int
		var how string
		switch action {
		case "top":
			pos, how = 0, "POS_SET"
		case "bottom":
			pos, how = -1, "POS_SET"
		case "up":
			pos, how = -1, "POS_CUR"
		case "down":
			pos, how = 1, "POS_CUR"
		default:
			return fmt.Errorf("invalid queue action: %s", action)
		}
		if _, err := c.call(ctx, "aria2.changePosition", []any{h, pos, how}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// —— 限速 ——

func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	opts := map[string]string{}
	if limits.UploadLimit >= 0 {
		opts["max-upload-limit"] = strconv.FormatInt(limits.UploadLimit, 10)
	}
	if limits.DownloadLimit >= 0 {
		opts["max-download-limit"] = strconv.FormatInt(limits.DownloadLimit, 10)
	}
	for _, h := range hashes {
		if _, err := c.call(ctx, "aria2.changeOption", []any{h, opts}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	raw, err := c.call(ctx, "aria2.getGlobalOption", nil)
	if err != nil {
		return nil, err
	}
	var opts map[string]string
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, fmt.Errorf("parse globalOption: %w", err)
	}
	return &downloader.Limits{
		UploadLimit:   parseI64(opts["max-overall-upload-limit"]),
		DownloadLimit: parseI64(opts["max-overall-download-limit"]),
	}, nil
}

func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	opts := map[string]string{
		"max-overall-upload-limit":   strconv.FormatInt(limits.UploadLimit, 10),
		"max-overall-download-limit": strconv.FormatInt(limits.DownloadLimit, 10),
	}
	_, err := c.call(ctx, "aria2.changeGlobalOption", []any{opts})
	return err
}

// —— 文件优先级 ——

func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	raw, err := c.call(ctx, "aria2.getFiles", []any{hash})
	if err != nil {
		return nil, err
	}
	var files []struct {
		Index           string `json:"index"`
		Path            string `json:"path"`
		Length          string `json:"length"`
		CompletedLength string `json:"completedLength"`
		Selected        string `json:"selected"`
	}
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(files))
	for _, f := range files {
		idx := int(parseI64(f.Index))
		// Aria2 index 从 1 起 → 统一 0-based
		if idx > 0 {
			idx--
		}
		total := parseI64(f.Length)
		done := parseI64(f.CompletedLength)
		var prog float64
		if total > 0 {
			prog = float64(done) / float64(total)
		}
		prio := 1 // selected
		if f.Selected == "false" {
			prio = 0
		}
		out = append(out, downloader.TorrentFile{
			Index: idx, Name: f.Path, Size: total, Progress: prog, Priority: prio,
		})
	}
	return out, nil
}

// SetFilePriority：Aria2 用 select-file 选项（逗号分隔 1-based index），priority=0 跳过。
// 简化：读当前 files，合并 selected 集合后 changeOption。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	files, err := c.ListFiles(ctx, hash)
	if err != nil {
		return err
	}
	selected := map[int]bool{}
	for _, f := range files {
		if f.Priority != 0 {
			selected[f.Index] = true
		}
	}
	for _, idx := range indices {
		if priority == 0 {
			delete(selected, idx)
		} else {
			selected[idx] = true
		}
	}
	// 组装 1-based 列表
	parts := make([]string, 0, len(selected))
	for _, f := range files {
		if selected[f.Index] {
			parts = append(parts, strconv.Itoa(f.Index+1))
		}
	}
	sel := "0" // 空选择 aria2 用 0?
	if len(parts) > 0 {
		sel = joinComma(parts)
	}
	_, err = c.call(ctx, "aria2.changeOption", []any{hash, map[string]string{"select-file": sel}})
	return err
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	s := parts[0]
	for i := 1; i < len(parts); i++ {
		s += "," + parts[i]
	}
	return s
}

// —— Peers ——

func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	raw, err := c.call(ctx, "aria2.getPeers", []any{hash})
	if err != nil {
		return nil, err
	}
	var peers []struct {
		IP            string `json:"ip"`
		Port          string `json:"port"`
		PeerID        string `json:"peerId"`
		Bitfield      string `json:"bitfield"`
		AmChoking     string `json:"amChoking"`
		PeerChoking   string `json:"peerChoking"`
		DownloadSpeed string `json:"downloadSpeed"`
		UploadSpeed   string `json:"uploadSpeed"`
		Seeder        string `json:"seeder"`
	}
	if err := json.Unmarshal(raw, &peers); err != nil {
		return nil, fmt.Errorf("parse peers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(peers))
	for _, p := range peers {
		out = append(out, downloader.Peer{
			IP:     p.IP,
			Client: p.PeerID,
			DLRate: parseI64(p.DownloadSpeed),
			UPRate: parseI64(p.UploadSpeed),
		})
	}
	return out, nil
}

// —— Tracker：从 tellStatus 的 announceList 只读 ——

func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	raw, err := c.call(ctx, "aria2.tellStatus", []any{hash, []string{"bittorrent"}})
	if err != nil {
		return nil, err
	}
	var t rawTask
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	if t.Bittorrent == nil {
		return nil, nil
	}
	var out []downloader.Tracker
	for tier, group := range t.Bittorrent.AnnounceList {
		for _, u := range group {
			out = append(out, downloader.Tracker{URL: u, Tier: tier})
		}
	}
	return out, nil
}

func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
