package fl

// advanced.go 实现 Flood (jesec/flood) 的高级操作，对标 qb/tr。
// API 参考：https://github.com/jesec/flood （/server/routes/api/torrents.ts、client.ts）。
//
// jesec 路径与 legacy Flood-UI 不同：本文件面向 jesec（主流分叉），
// 遇 404 回退 downloader.ErrNotImplemented（兼容 legacy 探测）。
// Flood 基于 rTorrent，部分 qBittorrent 概念无对应端点（见各方法注释）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// —— 生命周期 ——

// Recheck 强制重新校验 → POST /api/torrents/check-hash {hashes}。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	return c.postHashesJesec(ctx, "/api/torrents/check-hash", hashes)
}

// Reannounce 重新汇报 → POST /api/torrents/reannounce {hashes}。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	return c.postHashesJesec(ctx, "/api/torrents/reannounce", hashes)
}

// SetLocation 移动保存路径 → POST /api/torrents/move。
// jesec moveTorrentsSchema 要求 moveFiles/isBasePath/isCheckHash 三字段（strictObject 必填）。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	payload := map[string]any{
		"hashes":      upperAll(hashes),
		"destination": location,
		"moveFiles":   true,  // 移动数据而非复制
		"isBasePath":  false, // destination 为目标目录（非 base path）
		"isCheckHash": false, // 移动后不强制重新校验
	}
	status, _, err := c.raw(ctx, http.MethodPost, "/api/torrents/move",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("move status %d", status)
	}
	return nil
}

// SetForceStart 强制开始/取消强制。
// Flood（rTorrent）无"强制开始"概念：rTorrent 只有 start/stop，无视队列的
// force-start 开关未通过 Flood REST 暴露。保持未实现。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

// —— 分类 / 标签 ——

// ListCategories Flood 无分类实体，按首个 tag 聚合近似（tags[0] 视为分类）。
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

// ListTags 从任务列表聚合所有 tag。
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range list {
		for _, tag := range t.Tags {
			if tag != "" && !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
	}
	return out, nil
}

// SetTags 覆盖式设置标签 → jesec PATCH /api/torrents/tags {hashes, tags}。
// jesec 该端点为 SET 语义（整体覆盖），正好匹配 SetTags 契约。
func (c *Client) SetTags(ctx context.Context, hashes []string, tags []string) error {
	payload := map[string]any{"hashes": upperAll(hashes), "tags": tags}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/tags",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set tags status %d", status)
	}
	return nil
}

// AddTags 追加标签。jesec 无原生 add 端点（只有覆盖式 SET），按每任务 diff 实现：
// 读当前 tags → 并入新 tags → SetTags 覆盖。不同任务原 tag 不同故逐个设置。
func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	cur, err := c.tagsByHashes(ctx, hashes)
	if err != nil {
		return err
	}
	for _, h := range hashes {
		merged := unionStrings(cur[strings.ToLower(h)], tags)
		if err := c.SetTags(ctx, []string{h}, merged); err != nil {
			return err
		}
	}
	return nil
}

// RemoveTags 移除标签。同样按每任务 diff：读当前 → 减去要移除的 → SetTags 剩余。
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	cur, err := c.tagsByHashes(ctx, hashes)
	if err != nil {
		return err
	}
	for _, h := range hashes {
		remain := subtractStrings(cur[strings.ToLower(h)], tags)
		if err := c.SetTags(ctx, []string{h}, remain); err != nil {
			return err
		}
	}
	return nil
}

// CreateCategory / EditCategory / DeleteCategories：Flood 无分类实体。
// 分类即 tags；Flood 的 torrentDestinations 设置仅是 tag→目录的弱映射，
// 非 qB 意义上的分类 CRUD（无法仅凭标签新建/编辑/删除带保存路径的分类）。保持未实现。
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

// SetShareLimits 设置分享比率/做种时长上限。
// Flood REST 未暴露分享比率/做种时长限制端点；rTorrent 底层经 XML-RPC（views/group）
// 支持但 Flood REST 不开放。保持未实现（对标 MoviePilot 亦未对 Flood 实现）。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	return downloader.ErrNotImplemented
}

// —— 队列优先级 ——

// SetQueuePriority 调整优先级 → jesec PATCH /api/torrents/priority {hashes, priority}。
// jesec TorrentPriority 取值 0-3（0=不下载 1=低 2=正常 3=高）。
// 接口 action∈top|up|down|bottom 映射为绝对优先级；Flood 无相对位移概念，
// 故 up/top 同为高、down/bottom 区分正常/低（caps.queueFull=false 反映此限制）。
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	var prio int
	switch action {
	case "top", "up":
		prio = 3
	case "down":
		prio = 2
	case "bottom":
		prio = 1
	default:
		return fmt.Errorf("invalid queue action: %s", action)
	}
	payload := map[string]any{"hashes": upperAll(hashes), "priority": prio}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/priority",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("priority status %d", status)
	}
	return nil
}

// —— 顺序下载 / 首尾优先 ——

// ToggleSequentialDownload 切换顺序下载。
// jesec 端点为 SET（非 toggle）：PATCH /api/torrents/sequential {hashes, isSequential}。
// 实现按每任务当前状态翻转：当前开→关、当前关→开，分组各发一次。
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return err
	}
	state := make(map[string]bool, len(list))
	for _, t := range list {
		state[t.Hash] = t.Sequential
	}
	var on, off []string
	for _, h := range hashes {
		lh := strings.ToLower(h)
		if state[lh] {
			on = append(on, lh) // 当前顺序开 → 翻转为关
		} else {
			off = append(off, lh) // 当前关 → 翻转为开
		}
	}
	if len(off) > 0 {
		if err := c.setSequential(ctx, off, true); err != nil {
			return err
		}
	}
	if len(on) > 0 {
		if err := c.setSequential(ctx, on, false); err != nil {
			return err
		}
	}
	return nil
}

// setSequential jesec PATCH /api/torrents/sequential {hashes, isSequential}。
func (c *Client) setSequential(ctx context.Context, hashes []string, enable bool) error {
	payload := map[string]any{"hashes": upperAll(hashes), "isSequential": enable}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/sequential",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("sequential status %d", status)
	}
	return nil
}

// ToggleFirstLastPiece 切换首尾优先。
// Flood REST 未暴露首尾优先端点（rTorrent 内部支持但 Flood REST 不开放）。保持未实现。
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// —— 限速（bytes/s，0=不限）——

// SetLimits 设置单/多个任务上下行限速。
// Flood REST 无单种子限速端点（仅全局 throttleGlobal* 经 /api/client/settings）。
// 保持未实现。
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	return downloader.ErrNotImplemented
}

// GetSpeedLimits 读取全局速度上限 → GET /api/client/settings。
// jesec ClientSettings.throttleGlobalDownSpeed / throttleGlobalUpSpeed，单位 B/s。
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/client/settings", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("client settings status %d", status)
	}
	var s struct {
		Down int64 `json:"throttleGlobalDownSpeed"`
		Up   int64 `json:"throttleGlobalUpSpeed"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse client settings: %w", err)
	}
	return &downloader.Limits{DownloadLimit: s.Down, UploadLimit: s.Up}, nil
}

// SetSpeedLimits 设置全局速度上限 → PATCH /api/client/settings。
// throttleGlobalDownSpeed/UpSpeed 属 SAFE_CLIENT_SETTINGS（非 admin 字段），普通用户可改。
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	payload := map[string]any{
		"throttleGlobalDownSpeed": limits.DownloadLimit,
		"throttleGlobalUpSpeed":   limits.UploadLimit,
	}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/client/settings",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set client settings status %d", status)
	}
	return nil
}

// —— 文件优先级 ——

// ListFiles 列出某任务内文件 → GET /api/torrents/:hash/contents。
// jesec TorrentContent：index / path / filename / percentComplete(0-100) /
// priority(0-2: 0=不下载 1=正常 2=高) / sizeBytes。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/contents", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("files status %d", status)
	}
	var files []struct {
		Index    int     `json:"index"`
		Path     string  `json:"path"`
		Filename string  `json:"filename"`
		Size     int64   `json:"sizeBytes"`
		Progress float64 `json:"percentComplete"`
		Priority int     `json:"priority"`
	}
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(files))
	for _, f := range files {
		name := f.Path
		if name == "" {
			name = f.Filename
		}
		prog := f.Progress
		if prog > 1.0001 { // jesec 返回 0-100，归一化到 0-1
			prog /= 100
		}
		out = append(out, downloader.TorrentFile{
			Index: f.Index, Name: name, Size: f.Size, Progress: prog, Priority: f.Priority,
		})
	}
	return out, nil
}

// SetFilePriority 设置文件优先级 → PATCH /api/torrents/:hash/contents {indices, priority}。
// jesec priority 取值 0-2（0=不下载 1=正常 2=高），超出范围会被 zod 拒绝。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/"+strings.ToUpper(hash)+"/contents",
		mustJSON(map[string]any{"indices": indices, "priority": priority}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set file prio status %d", status)
	}
	return nil
}

// —— Peers ——

// ListPeers 列出某任务当前连接的对等体 → GET /api/torrents/:hash/peers。
// jesec TorrentPeer：address / clientVersion / completedPercent(0-100) /
// downloadRate / uploadRate / isEncrypted / isIncoming。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/peers", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("peers status %d", status)
	}
	var peers []struct {
		Address      string  `json:"address"`
		Client       string  `json:"clientVersion"`
		CompletedPct float64 `json:"completedPercent"`
		DlRate       int64   `json:"downloadRate"`
		UpRate       int64   `json:"uploadRate"`
		IsEncrypted  bool    `json:"isEncrypted"`
		IsIncoming   bool    `json:"isIncoming"`
	}
	if err := json.Unmarshal(b, &peers); err != nil {
		return nil, fmt.Errorf("parse peers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(peers))
	for _, p := range peers {
		// 汇总标志位：E=加密 I=入站连接（对标 qB flags 的子集语义）
		var flags strings.Builder
		if p.IsEncrypted {
			flags.WriteByte('E')
		}
		if p.IsIncoming {
			flags.WriteByte('I')
		}
		prog := p.CompletedPct
		if prog > 1.0001 { // 0-100 → 0-1
			prog /= 100
		}
		out = append(out, downloader.Peer{
			IP:       p.Address,
			Client:   p.Client,
			Progress: prog,
			DLRate:   p.DlRate,
			UPRate:   p.UpRate,
			Flags:    flags.String(),
		})
	}
	return out, nil
}

// —— Tracker ——

// ListTrackers 列出某任务的 tracker → GET /api/torrents/:hash/trackers。
// jesec TorrentTracker：url / type(1=HTTP 2=UDP 3=DHT)。Flood 不暴露 tier，type 映射到 Status。
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/trackers", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("trackers status %d", status)
	}
	var tr []struct {
		URL  string `json:"url"`
		Type int    `json:"type"`
	}
	if err := json.Unmarshal(b, &tr); err != nil {
		return nil, fmt.Errorf("parse trackers: %w", err)
	}
	out := make([]downloader.Tracker, 0, len(tr))
	for _, t := range tr {
		out = append(out, downloader.Tracker{URL: t.URL, Status: trackerTypeStatus(t.Type)})
	}
	return out, nil
}

// AddTrackers 追加 tracker。jesec 仅提供覆盖式 SET（PATCH /api/torrents/trackers），
// 无原生 add：读当前 tracker → 并入新 URL → 覆盖设置。
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	cur, err := c.ListTrackers(ctx, hash)
	if err != nil {
		return err
	}
	existing := make([]string, 0, len(cur))
	for _, t := range cur {
		existing = append(existing, t.URL)
	}
	return c.setTrackers(ctx, []string{hash}, unionStrings(existing, urls))
}

// RemoveTrackers 移除 tracker。同样经 SET：读当前 → 减去要删的 → 覆盖设置。
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	cur, err := c.ListTrackers(ctx, hash)
	if err != nil {
		return err
	}
	existing := make([]string, 0, len(cur))
	for _, t := range cur {
		existing = append(existing, t.URL)
	}
	return c.setTrackers(ctx, []string{hash}, subtractStrings(existing, urls))
}

// setTrackers jesec PATCH /api/torrents/trackers {hashes, trackers}（覆盖式 SET）。
// schema：hashes 非空数组、trackers 可为空（清空所有 tracker）。
func (c *Client) setTrackers(ctx context.Context, hashes []string, trackers []string) error {
	payload := map[string]any{"hashes": upperAll(hashes), "trackers": trackers}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/trackers",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set trackers status %d", status)
	}
	return nil
}

// —— 内部工具 ——

// postHashesJesec 公用：POST {hashes} 到 jesec 端点，404→ErrNotImplemented。
func (c *Client) postHashesJesec(ctx context.Context, path string, hashes []string) error {
	status, _, err := c.raw(ctx, http.MethodPost, path,
		mustJSON(map[string]any{"hashes": upperAll(hashes)}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("%s status %d", path, status)
	}
	return nil
}

// tagsByHashes 一次 ListTorrents，构建 hash(小写)→tags 映射，供 AddTags/RemoveTags diff。
func (c *Client) tagsByHashes(ctx context.Context, hashes []string) (map[string][]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(list))
	for _, t := range list {
		out[t.Hash] = t.Tags
	}
	return out, nil
}
