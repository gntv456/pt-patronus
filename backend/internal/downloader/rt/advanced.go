package rt

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// —— 生命周期 ——

// Recheck → d.check_hash（XML multicall）。
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

// Reannounce → d.tracker_announce（XML multicall）。
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

// SetLocation → d.directory.set（仅改保存目录，不移动已落盘文件；rTorrent 语义，UI 需注明）。
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

// SetForceStart：rTorrent 无「强制开始」概念（种子 open/start 即全力下载，无 qB 式 force 标志位）。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

// —— 分类 / 标签 ——

// ListCategories 去重聚合各任务的 label（d.custom1）。
// ruTorrent 的「分类」纯前端对 label 字段的聚合，服务端无独立分类表；
// 因此 Create/Edit/Delete 系列无对应实体可操作，返回 ErrNotImplemented。
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

// ListTags：ruTorrent 仅有单一 label（d.custom1，已映射为 Category），无独立多标签概念。
// 保持 ErrNotImplemented，避免前端误以为存在与 qB 同级的 tags 数组。
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

// CreateCategory / EditCategory / DeleteCategories：ruTorrent 分类为前端对 label 的聚合视图，
// 服务端无分类注册表；label 仅作为 d.custom1 字段附在种子上，无独立 CRUD 实体。
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

// SetShareLimits：rTorrent 的分享比率限制基于 view（分组），ruTorrent 的 ratio 插件按比率组管理，
// 没有干净的 per-seed HTTP/XML-RPC 接口；直接 d.ratio.* 命令在多数构建里仅作用于视图而非单种子。
// 保留 ErrNotImplemented 避免误设静默失败。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	return downloader.ErrNotImplemented
}

// —— 队列 ——

// SetQueuePriority → d.priority.set（rTorrent 0=off 1=low 2=normal 3=high）。
// top/up 映射为 high(3)，down 映射为 normal(2)，bottom 映射为 low(1)。
// 注意 rTorrent priority 为绝对优先级而非相对队列位移，与 qB 的 up/down 语义有别。
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
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

// —— 顺序下载 / 首尾优先 ——

// rTorrent 不支持顺序下载 / 首尾优先（这是 qB/libtorrent 特性），保留未实现。
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// —— 限速（统一 bytes/s，0=不限）——

// SetLimits 设置单/多任务上下行限速。
// 走 XML multicall：d.set_upload_rate / d.set_download_rate（bytes/s，0=不限）。
// 这是 ruTorrent 自身 setprops 路径（mode=setprops → d.set_<s>）所生成的同一组命令，
// 兼容 rTorrent 0.8/0.9。旧实现误用 d.up.rate.set（d.up.rate 是只读当前速率，无 setter）已修正。
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	calls := make([][2]any, 0, len(hashes)*2)
	for _, h := range hashes {
		up := strings.ToUpper(h)
		// 0=取消限制（rTorrent 语义），故始终发送
		calls = append(calls, [2]any{"d.set_upload_rate", []string{up, strconv.FormatInt(limits.UploadLimit, 10)}})
		calls = append(calls, [2]any{"d.set_download_rate", []string{up, strconv.FormatInt(limits.DownloadLimit, 10)}})
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

// GetSpeedLimits 读取全局上下行速率上限。
// 走 XML multicall：get_upload_rate / get_download_rate（bytes/s；与 setul/setdl 的 set_* 对称）。
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	xmlBody := buildMulticall([][2]any{
		{"get_upload_rate", []string{}},
		{"get_download_rate", []string{}},
	})
	status, b, err := c.rpcXML(ctx, xmlBody)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("speed limits status %d", status)
	}
	vals, err := parseMulticallResults(b)
	if err != nil {
		return nil, fmt.Errorf("parse speed limits: %w", err)
	}
	if len(vals) < 2 {
		return nil, fmt.Errorf("speed limits returned %d values, want 2", len(vals))
	}
	up, _ := strconv.ParseInt(vals[0], 10, 64)
	dl, _ := strconv.ParseInt(vals[1], 10, 64)
	return &downloader.Limits{UploadLimit: up, DownloadLimit: dl}, nil
}

// SetSpeedLimits 设置全局上下行速率上限（bytes/s，0=不限）。
// 走 httprpc：mode=setul / mode=setdl → set_upload_rate / set_download_rate。
// 参数名 s（httprpc 的 $ss[] 收集器），值为字节数。
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	for _, op := range []struct {
		mode string
		val  int64
	}{
		{"setul", limits.UploadLimit},
		{"setdl", limits.DownloadLimit},
	} {
		vals := url.Values{"mode": {op.mode}, "s": {strconv.FormatInt(op.val, 10)}}
		status, _, err := c.rpcForm(ctx, vals)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("%s status %d", op.mode, status)
		}
	}
	return nil
}

// —— 文件优先级 ——

// ListFiles 列出某任务内文件。
// 走 httprpc mode=fls → f.multicall，返回 [[path, completed_chunks, size_chunks, size_bytes, priority], ...]。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	vals := url.Values{"mode": {"fls"}, "hash": {strings.ToUpper(hash)}}
	status, b, err := c.rpcForm(ctx, vals)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("files status %d", status)
	}
	var rows [][]any
	if err := ensureJSON(b, &rows); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(rows))
	for i, row := range rows {
		completed := iv(row, 1)
		sizeChunks := iv(row, 2)
		var prog float64
		if sizeChunks > 0 {
			prog = float64(completed) / float64(sizeChunks)
		}
		out = append(out, downloader.TorrentFile{
			Index:    i,
			Name:     sv(row, 0), // f.get_path（相对种子根的路径）
			Size:     iv(row, 3), // f.get_size_bytes
			Progress: prog,
			Priority: int(iv(row, 4)), // rTorrent: 0=不下载 1=正常 2=高
		})
	}
	return out, nil
}

// SetFilePriority 设置文件优先级。
// 走 httprpc mode=setprio：v=索引(可多值) s=优先级 → f.set_priority + d.update_priorities。
// rTorrent 文件优先级取值：0=不下载 1=正常 2=高。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	vals := url.Values{"mode": {"setprio"}, "hash": {strings.ToUpper(hash)}}
	for _, i := range indices {
		vals.Add("v", strconv.Itoa(i))
	}
	vals.Set("s", strconv.Itoa(priority))
	status, _, err := c.rpcForm(ctx, vals)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setprio status %d", status)
	}
	return nil
}

// —— Peers ——

// ListPeers 列出某任务当前连接的对等体。
// 走 httprpc mode=prs → p.multicall，返回
// [[id, address, client_version, is_incoming, is_encrypted, is_snubbed,
//
//	completed_percent, down_total, up_total, down_rate, up_rate, id_html, peer_rate, peer_total, port], ...]。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	vals := url.Values{"mode": {"prs"}, "hash": {strings.ToUpper(hash)}}
	status, b, err := c.rpcForm(ctx, vals)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("peers status %d", status)
	}
	var rows [][]any
	if err := ensureJSON(b, &rows); err != nil {
		return nil, fmt.Errorf("parse peers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(rows))
	for _, row := range rows {
		out = append(out, downloader.Peer{
			IP:       sv(row, 1),                  // p.get_address
			Client:   sv(row, 2),                  // p.get_client_version
			Progress: float64(iv(row, 6)) / 100.0, // p.get_completed_percent (0..100)
			DLRate:   iv(row, 9),                  // p.get_down_rate（peer→我们）
			UPRate:   iv(row, 10),                 // p.get_up_rate（我们→peer）
			Flags:    peerFlags(iv(row, 3) != 0, iv(row, 4) != 0, iv(row, 5) != 0),
		})
	}
	return out, nil
}

// —— Tracker ——

// ListTrackers 列出某任务的 tracker。
// 走 httprpc mode=trk → t.multicall，返回
// [[url, type, is_enabled, group, scrape_complete, scrape_incomplete, scrape_downloaded, normal_interval, scrape_time_last], ...]。
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	vals := url.Values{"mode": {"trk"}, "hash": {strings.ToUpper(hash)}}
	status, b, err := c.rpcForm(ctx, vals)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("trackers status %d", status)
	}
	var rows [][]any
	if err := ensureJSON(b, &rows); err != nil {
		return nil, fmt.Errorf("parse trackers: %w", err)
	}
	out := make([]downloader.Tracker, 0, len(rows))
	for _, row := range rows {
		out = append(out, downloader.Tracker{
			URL:    sv(row, 0),                              // t.get_url
			Tier:   int(iv(row, 3)),                         // t.get_group（tier）
			Status: rtTrackerStatus(iv(row, 2), iv(row, 1)), // is_enabled, type
		})
	}
	return out, nil
}

// AddTrackers / RemoveTrackers：rTorrent 不支持对已加载种子的 tracker 列表做增删
// （tracker 在种子加载时固化；ruTorrent 的 retrackers 插件仅作用于新种子，无 per-torrent 接口）。
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
