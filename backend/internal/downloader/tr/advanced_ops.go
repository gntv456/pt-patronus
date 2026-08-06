package tr

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

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
