package ut

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// —— 生命周期 ——

// Recheck 强制重新校验（action=recheck）。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	return c.actionHashes(ctx, "recheck", hashes)
}

// Reannounce：uT WebUI 无 reannounce 动作。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// SetLocation：uT WebUI 无移动存储位置动作。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	return downloader.ErrNotImplemented
}

// SetForceStart 强制开始/取消（action=forcestart&v=0|1）。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	v := "0"
	if enable {
		v = "1"
	}
	extra := append(hashParams(hashes), kv{"v", v})
	status, _, err := c.do(ctx, "forcestart", extra...)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("forcestart status %d", status)
	}
	return nil
}

// —— 分类 / 标签（uT 仅 label 单值）——

// ListCategories 去重 list=1 的 label 字段返回（零额外请求）。
func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	rows, err := c.fetchList(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, row := range rows {
		if l := strAt(row, 11); l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out, nil
}

// uT 无 tag 概念（label 单值已被 category 占用）。
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

// uT label 无 savepath/CRUD 概念。
func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) DeleteCategories(ctx context.Context, names []string) error {
	return downloader.ErrNotImplemented
}

// —— 做种限制（setprops seed_ratio/seed_time）——
// 联调点：uT 的 seed_ratio 编码（>=0=ratio×10？）、seed_time（分钟）以 getprops 返回为准校准。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	for _, h := range hashes {
		var props []kv
		// seed_ratio：>=0=具体（uT 存 ratio×10 的整数倍，暂按 ×10）；-2=无限→-1；-1=全局→0
		switch limits.RatioLimit {
		case -2:
			props = append(props, kv{"s", "seed_ratio"}, kv{"v", "-1"})
		case -1:
			props = append(props, kv{"s", "seed_ratio"}, kv{"v", "0"})
		default:
			props = append(props, kv{"s", "seed_ratio"}, kv{"v", strconv.Itoa(int(limits.RatioLimit * 10))})
		}
		// seed_time（分钟）：>=0=具体；-2=无限→-1；-1=全局→0
		switch limits.SeedingTimeLimit {
		case -2:
			props = append(props, kv{"s", "seed_time"}, kv{"v", "-1"})
		case -1:
			props = append(props, kv{"s", "seed_time"}, kv{"v", "0"})
		default:
			props = append(props, kv{"s", "seed_time"}, kv{"v", strconv.Itoa(limits.SeedingTimeLimit)})
		}
		if err := c.setPropsOne(ctx, h, props...); err != nil {
			return err
		}
	}
	return nil
}

// —— 队列优先级（4 个全支持）——
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	var utAction string
	switch action {
	case "top":
		utAction = "queuetop"
	case "up":
		utAction = "queueup"
	case "down":
		utAction = "queuedown"
	case "bottom":
		utAction = "queuebottom"
	default:
		return fmt.Errorf("invalid queue action: %s", action)
	}
	return c.actionHashes(ctx, utAction, hashes)
}

// —— 顺序下载 / 首尾优先（uT 不支持）——
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// —— 限速（bytes/s；联调点：prop 名 rate_upload_max/rate_download_max 以 getprops 实测为准）——
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	for _, h := range hashes {
		props := []kv{
			{"s", "rate_upload_max"}, {"v", strconv.FormatInt(limits.UploadLimit, 10)},
			{"s", "rate_download_max"}, {"v", strconv.FormatInt(limits.DownloadLimit, 10)},
		}
		if err := c.setPropsOne(ctx, h, props...); err != nil {
			return err
		}
	}
	return nil
}

// GetSpeedLimits 读 getsettings 的 max_upload_rate/max_download_rate（bytes/s）。
// getsettings 返回 {settings: [[name, type, value, flags], ...]}。
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	status, b, err := c.do(ctx, "getsettings")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("getsettings status %d", status)
	}
	var resp struct {
		Settings [][]any `json:"settings"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("parse getsettings: %w", err)
	}
	var up, dl int64
	for _, s := range resp.Settings {
		if len(s) < 3 {
			continue
		}
		name, _ := s[0].(string)
		switch name {
		case "max_upload_rate":
			up = toInt64(s[2])
		case "max_download_rate":
			dl = toInt64(s[2])
		}
	}
	return &downloader.Limits{UploadLimit: up, DownloadLimit: dl}, nil
}

// SetSpeedLimits 全局速度上限（setsetting）。
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	for _, op := range []struct {
		k string
		v int64
	}{{"max_upload_rate", limits.UploadLimit}, {"max_download_rate", limits.DownloadLimit}} {
		status, _, err := c.do(ctx, "setsetting", kv{"s", op.k}, kv{"v", strconv.FormatInt(op.v, 10)})
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("setsetting status %d", status)
		}
	}
	return nil
}

// —— 文件优先级 ——

// ListFiles 列出某任务内文件（getfiles）。
// 联调点：getfiles 返回每文件元组 [name, size(bytes), downloaded(bytes), priority(int), ...]。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	status, b, err := c.do(ctx, "getfiles", kv{"hash", hash})
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("getfiles status %d", status)
	}
	var resp struct {
		Files [][]any `json:"files"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("parse getfiles: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(resp.Files))
	for i, f := range resp.Files {
		var prog float64
		sz := toInt64(at(f, 1))
		if sz > 0 {
			prog = float64(toInt64(at(f, 2))) / float64(sz)
		}
		out = append(out, downloader.TorrentFile{
			Index:    i,
			Name:     strAny(at(f, 0)),
			Size:     sz,
			Progress: prog,
			Priority: int(toInt64(at(f, 3))),
		})
	}
	return out, nil
}

// SetFilePriority 设置文件优先级（setprio&hash=&p=N&f=index）。uT priority ∈ {0=skip,1=low,2=normal,3=high}。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	for _, idx := range indices {
		status, _, err := c.do(ctx, "setprio",
			kv{"hash", hash}, kv{"p", strconv.Itoa(priority)}, kv{"f", strconv.Itoa(idx)})
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("setprio status %d", status)
		}
	}
	return nil
}

// —— Peers ——

// ListPeers 列出某任务当前连接的对等体（getpeers）。
// 联调点：getpeers 返回 peers 数组，每项元组 [ip, ..., useragent, flags, ...]，字段位置以实测为准。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	status, b, err := c.do(ctx, "getpeers", kv{"hash", hash})
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("getpeers status %d", status)
	}
	var resp struct {
		Peers [][]any `json:"peers"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("parse getpeers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(resp.Peers))
	for _, p := range resp.Peers {
		out = append(out, downloader.Peer{
			IP:     strAny(at(p, 0)),
			Client: strAny(at(p, 3)),
			Flags:  strAny(at(p, 6)),
		})
	}
	return out, nil
}

// —— Tracker（uT WebUI 不暴露 tracker 操作）——
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	return nil, downloader.ErrNotImplemented
}
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}

// —— 小工具 ——

func at(row []any, i int) any {
	if i >= len(row) {
		return nil
	}
	return row[i]
}

func strAny(v any) string {
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
		f, _ := strconv.ParseFloat(n, 64)
		return int64(f)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
