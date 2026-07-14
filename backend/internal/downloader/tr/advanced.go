package tr

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// bytesToKB 把 bytes/s 转 KiB/s（向上取整，避免 <1024 截断为 0）。0 或负数返回 0（=不限）。
func bytesToKB(b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (b + 1023) / 1024
}

// extractAddedID 从 torrent-add 响应取新增任务 id（torrent-added 或 torrent-duplicate）。
func extractAddedID(raw json.RawMessage) int {
	var r struct {
		Added *struct {
			ID int `json:"id"`
		} `json:"torrent-added"`
		Duplicate *struct {
			ID int `json:"id"`
		} `json:"torrent-duplicate"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0
	}
	if r.Added != nil {
		return r.Added.ID
	}
	if r.Duplicate != nil {
		return r.Duplicate.ID
	}
	return 0
}

// —— 生命周期 ——

// Recheck 强制重新校验。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	_, err := c.rpc(ctx, "torrent-verify", map[string]any{"ids": hashes})
	return err
}

// Reannounce 重新汇报。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	_, err := c.rpc(ctx, "torrent-reannounce", map[string]any{"ids": hashes})
	return err
}

// SetLocation 移动保存路径（TR 3.0+；旧版会 RPC 报错）。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	_, err := c.rpc(ctx, "torrent-set-location", map[string]any{
		"ids":      hashes,
		"location": location,
		"move":     true,
	})
	return err
}

// —— 分类 / 标签 ——

// ListCategories：Transmission 无分类概念。
func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	return nil, downloader.ErrNotImplemented
}

// ListTags 聚合全部任务的 labels 去重。
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{"fields": []string{"labels"}})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []struct {
			Labels []string `json:"labels"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse labels: %w", err)
	}
	seen := map[string]bool{}
	var tags []string
	for _, t := range out.Torrents {
		for _, l := range t.Labels {
			if !seen[l] {
				seen[l] = true
				tags = append(tags, l)
			}
		}
	}
	return tags, nil
}

// setLabels 覆盖式设置某组任务的 labels。
func (c *Client) setLabels(ctx context.Context, hashes []string, labels []string) error {
	_, err := c.rpc(ctx, "torrent-set", map[string]any{"ids": hashes, "labels": labels})
	return err
}

// labelsOf 取单个任务的当前 labels。
func (c *Client) labelsOf(ctx context.Context, hash string) ([]string, error) {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{
		"ids":    []string{hash},
		"fields": []string{"labels"},
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []struct {
			Labels []string `json:"labels"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Torrents) == 0 {
		return nil, nil
	}
	return out.Torrents[0].Labels, nil
}

// SetTags 覆盖式设置标签（TR labels 天然覆盖语义）。
func (c *Client) SetTags(ctx context.Context, hashes []string, tags []string) error {
	return c.setLabels(ctx, hashes, tags)
}

// AddTags 追加标签（TR labels 是 per-torrent，逐个合并去重）。
func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	for _, h := range hashes {
		old, err := c.labelsOf(ctx, h)
		if err != nil {
			return err
		}
		if err := c.setLabels(ctx, []string{h}, mergeUnique(old, tags)); err != nil {
			return err
		}
	}
	return nil
}

// RemoveTags 移除标签（逐个过滤）。
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	rm := make(map[string]bool, len(tags))
	for _, t := range tags {
		rm[t] = true
	}
	for _, h := range hashes {
		old, err := c.labelsOf(ctx, h)
		if err != nil {
			return err
		}
		var kept []string
		for _, l := range old {
			if !rm[l] {
				kept = append(kept, l)
			}
		}
		if err := c.setLabels(ctx, []string{h}, kept); err != nil {
			return err
		}
	}
	return nil
}

func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// —— 限速（入参 bytes/s，内部转 KiB/s）——

// SetLimits 设置单/多任务限速（0=不限，需同时置 *Limited 标志）。
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	_, err := c.rpc(ctx, "torrent-set", map[string]any{
		"ids":             hashes,
		"uploadLimit":     bytesToKB(limits.UploadLimit),
		"uploadLimited":   limits.UploadLimit > 0,
		"downloadLimit":   bytesToKB(limits.DownloadLimit),
		"downloadLimited": limits.DownloadLimit > 0,
	})
	return err
}

// GetSpeedLimits 读取全局速度上限（session-get，KiB/s 转 bytes/s；未启用记为 0=不限）。
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	raw, err := c.rpc(ctx, "session-get", nil)
	if err != nil {
		return nil, err
	}
	var s struct {
		SpeedLimitUp          int64 `json:"speed-limit-up"`
		SpeedLimitUpEnabled   bool  `json:"speed-limit-up-enabled"`
		SpeedLimitDown        int64 `json:"speed-limit-down"`
		SpeedLimitDownEnabled bool  `json:"speed-limit-down-enabled"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	var up, dl int64
	if s.SpeedLimitUpEnabled {
		up = s.SpeedLimitUp * 1024
	}
	if s.SpeedLimitDownEnabled {
		dl = s.SpeedLimitDown * 1024
	}
	return &downloader.Limits{UploadLimit: up, DownloadLimit: dl}, nil
}

// SetSpeedLimits 设置全局速度上限（bytes/s，0=不限）。
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	_, err := c.rpc(ctx, "session-set", map[string]any{
		"speed-limit-up":           bytesToKB(limits.UploadLimit),
		"speed-limit-up-enabled":   limits.UploadLimit > 0,
		"speed-limit-down":         bytesToKB(limits.DownloadLimit),
		"speed-limit-down-enabled": limits.DownloadLimit > 0,
	})
	return err
}

// —— 文件优先级 ——

// ListFiles 列出某任务内文件。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{
		"ids":    []string{hash},
		"fields": []string{"files"},
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []struct {
			Files []struct {
				Name           string `json:"name"`
				Length         int64  `json:"length"`
				BytesCompleted int64  `json:"bytesCompleted"`
				Priority       int    `json:"priority"`
			} `json:"files"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	if len(out.Torrents) == 0 {
		return nil, nil
	}
	fs := out.Torrents[0].Files
	res := make([]downloader.TorrentFile, 0, len(fs))
	for i, f := range fs {
		var prog float64
		if f.Length > 0 {
			prog = float64(f.BytesCompleted) / float64(f.Length)
		}
		res = append(res, downloader.TorrentFile{
			Index: i, Name: f.Name, Size: f.Length, Progress: prog, Priority: f.Priority,
		})
	}
	return res, nil
}

// SetFilePriority 设置文件优先级（TR priority ∈ {-1=低, 0=正常, 1=高}）。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	_, err := c.rpc(ctx, "torrent-set", map[string]any{
		"ids":      []string{hash},
		"files":    indices,
		"priority": priority,
	})
	return err
}

// —— Tracker ——

// ListTrackers 列出某任务的 tracker。
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{
		"ids":    []string{hash},
		"fields": []string{"trackers"},
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []struct {
			Trackers []struct {
				Announce string `json:"announce"`
				Tier     int    `json:"tier"`
			} `json:"trackers"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse trackers: %w", err)
	}
	if len(out.Torrents) == 0 {
		return nil, nil
	}
	res := make([]downloader.Tracker, 0, len(out.Torrents[0].Trackers))
	for _, t := range out.Torrents[0].Trackers {
		res = append(res, downloader.Tracker{URL: t.Announce, Tier: t.Tier})
	}
	return res, nil
}

// AddTrackers 追加 tracker（TR 4.x trackerAdd 接受 URL 数组）。
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	_, err := c.rpc(ctx, "torrent-set", map[string]any{
		"ids":        []string{hash},
		"trackerAdd": urls,
	})
	return err
}

// RemoveTrackers maps tracker URLs to Transmission tracker IDs before removal.
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{
		"ids":    []string{hash},
		"fields": []string{"trackers"},
	})
	if err != nil {
		return err
	}
	var out struct {
		Torrents []struct {
			Trackers []struct {
				ID       int    `json:"id"`
				Announce string `json:"announce"`
			} `json:"trackers"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("parse trackers: %w", err)
	}
	wanted := make(map[string]struct{}, len(urls))
	for _, trackerURL := range urls {
		wanted[trackerURL] = struct{}{}
	}
	var ids []int
	if len(out.Torrents) > 0 {
		for _, tracker := range out.Torrents[0].Trackers {
			if _, ok := wanted[tracker.Announce]; ok {
				ids = append(ids, tracker.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = c.rpc(ctx, "torrent-set", map[string]any{
		"ids":           []string{hash},
		"trackerRemove": ids,
	})
	return err
}

// —— 做种限制 ——

// SetShareLimits 设置分享比率/空闲做种时长上限（TR seedRatio* / seedIdle*）。
// ratioLimit/seedTimeLimit 统一语义：-2=无限 -1=全局默认 >=0=具体值；TR 做种时长映射为「空闲时长」。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	args := map[string]any{"ids": hashes}
	switch limits.RatioLimit {
	case -2:
		args["seedRatioMode"] = 2 // unlimited
	case -1:
		args["seedRatioMode"] = 0 // global
	default:
		args["seedRatioMode"] = 1 // override
		args["seedRatioLimit"] = limits.RatioLimit
	}
	switch limits.SeedingTimeLimit {
	case -2:
		args["seedIdleMode"] = 2
	case -1:
		args["seedIdleMode"] = 0
	default:
		args["seedIdleMode"] = 1
		args["seedIdleLimit"] = limits.SeedingTimeLimit
	}
	_, err := c.rpc(ctx, "torrent-set", args)
	return err
}

// —— 队列优先级 ——

// SetQueuePriority TR 仅支持 top/bottom（绝对 queuePosition）；up/down 返回 ErrNotImplemented。
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	var pos int
	switch action {
	case "top":
		pos = 0
	case "bottom":
		pos = 2147483647 // 超大值，TR 归一化后落至队尾
	default:
		return downloader.ErrNotImplemented
	}
	_, err := c.rpc(ctx, "torrent-set", map[string]any{"ids": hashes, "queuePosition": pos})
	return err
}

// SetForceStart Transmission 无强制开始概念。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

// ToggleSequentialDownload Transmission 不支持顺序下载。
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// ToggleFirstLastPiece Transmission 不支持首尾优先。
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// —— Peers ——

// ListPeers 列出某任务当前连接的对等体（torrent-get fields=peers）。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{
		"ids":    []string{hash},
		"fields": []string{"peers"},
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []struct {
			Peers []struct {
				Address      string  `json:"address"`
				ClientName   string  `json:"clientName"`
				Progress     float64 `json:"progress"`
				RateToClient int64   `json:"rateToClient"` // peer→我们
				RateToPeer   int64   `json:"rateToPeer"`   // 我们→peer
				FlagStr      string  `json:"flagStr"`
			} `json:"peers"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse peers: %w", err)
	}
	if len(out.Torrents) == 0 {
		return nil, nil
	}
	res := make([]downloader.Peer, 0, len(out.Torrents[0].Peers))
	for _, p := range out.Torrents[0].Peers {
		res = append(res, downloader.Peer{
			IP:       p.Address,
			Client:   p.ClientName,
			Progress: p.Progress,
			DLRate:   p.RateToClient,
			UPRate:   p.RateToPeer,
			Flags:    p.FlagStr,
		})
	}
	return res, nil
}

// —— 分类增删（TR 无分类概念，统一返回未实现）——

func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}

func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}

func (c *Client) DeleteCategories(ctx context.Context, names []string) error {
	return downloader.ErrNotImplemented
}
