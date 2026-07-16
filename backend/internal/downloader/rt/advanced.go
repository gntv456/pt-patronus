package rt

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// Recheck → d.check_hash
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	calls := make([][2]any, 0, len(hashes))
	for _, h := range hashes {
		calls = append(calls, [2]any{"d.check_hash", []string{strings.ToUpper(h)}})
	}
	status, _, err := c.rpcXML(ctx, buildMulticall(calls))
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("recheck status %d", status)
	}
	return nil
}

// Reannounce → d.tracker_announce
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	calls := make([][2]any, 0, len(hashes))
	for _, h := range hashes {
		calls = append(calls, [2]any{"d.tracker_announce", []string{strings.ToUpper(h)}})
	}
	status, _, err := c.rpcXML(ctx, buildMulticall(calls))
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("reannounce status %d", status)
	}
	return nil
}

// SetLocation → d.directory.set（不移动已有文件，rTorrent 语义）
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	calls := make([][2]any, 0, len(hashes))
	for _, h := range hashes {
		calls = append(calls, [2]any{"d.directory.set", []string{strings.ToUpper(h), location}})
	}
	status, _, err := c.rpcXML(ctx, buildMulticall(calls))
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("set location status %d", status)
	}
	return nil
}

func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

// ListCategories 去重 label。
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

// SetShareLimits → d.ratio 相关（简化：d.uploads_max 不映射；用 d.custom 不稳）
// rTorrent: 通过 d.throttle 不统一；返回 NotImplemented 避免误设。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	return downloader.ErrNotImplemented
}

// SetQueuePriority → d.priority.set (0=off,1=low,2=normal,3=high)
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	// top/up → high(3)；down → normal(2)；bottom → low(1)
	prio := "2"
	switch action {
	case "top", "up":
		prio = "3"
	case "down":
		prio = "2"
	case "bottom":
		prio = "1"
	default:
		return fmt.Errorf("invalid queue action: %s", action)
	}
	calls := make([][2]any, 0, len(hashes))
	for _, h := range hashes {
		calls = append(calls, [2]any{"d.priority.set", []string{strings.ToUpper(h), prio}})
	}
	status, _, err := c.rpcXML(ctx, buildMulticall(calls))
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("priority status %d", status)
	}
	return nil
}

func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// SetLimits → d.throttle 不稳；用 d.up.rate / d.down.rate（若支持）
// 简化：d.set_upload_rate / d.set_download_rate（部分 rTorrent）
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	calls := make([][2]any, 0, len(hashes)*2)
	for _, h := range hashes {
		up := strings.ToUpper(h)
		if limits.UploadLimit >= 0 {
			calls = append(calls, [2]any{"d.up.rate.set", []string{up, fmt.Sprintf("%d", limits.UploadLimit)}})
		}
		if limits.DownloadLimit >= 0 {
			calls = append(calls, [2]any{"d.down.rate.set", []string{up, fmt.Sprintf("%d", limits.DownloadLimit)}})
		}
	}
	if len(calls) == 0 {
		return nil
	}
	status, _, err := c.rpcXML(ctx, buildMulticall(calls))
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("set limits status %d", status)
	}
	return nil
}

func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	return nil, downloader.ErrNotImplemented
}
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	return downloader.ErrNotImplemented
}

func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	return nil, downloader.ErrNotImplemented
}
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	return downloader.ErrNotImplemented
}
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	return nil, downloader.ErrNotImplemented
}
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	return nil, downloader.ErrNotImplemented
}
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
