// Package tr 是 Transmission RPC 客户端，实现 downloader.Downloader 接口。
// 协议：POST JSON-RPC 风格，首次返回 409 携带 X-Transmission-Session-Id，
// 重试带上该头；Basic Auth；torrent-get/torrent-add/torrent-stop/start/remove。
package tr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

const defaultEndpoint = "/transmission/rpc"

// Client 是一个 Transmission 实例的客户端。
type Client struct {
	baseURL   string // 含 RPC endpoint（/transmission/rpc）
	username  string
	password  string
	hc        *http.Client
	mu        sync.Mutex
	sessionID string
}

// New 构造客户端。baseURL 形如 http://host:9091（自动补 /transmission/rpc）。
func New(baseURL, username, password string) *Client {
	b := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(b, defaultEndpoint) {
		b += defaultEndpoint
	}
	return &Client{
		baseURL:  b,
		username: username,
		password: password,
		hc:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Type 返回下载器类型。
func (c *Client) Type() string { return "tr" }

// rpc 执行一次 RPC 调用，处理 409 会话握手与 Basic 鉴权。
func (c *Client) rpc(ctx context.Context, method string, args map[string]any) (json.RawMessage, error) {
	reqBody, _ := json.Marshal(map[string]any{"method": method, "arguments": args})

	exec := func(sessionID string) (int, http.Header, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(reqBody))
		if err != nil {
			return 0, nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.username != "" {
			req.SetBasicAuth(c.username, c.password)
		}
		if sessionID != "" {
			req.Header.Set("X-Transmission-Session-Id", sessionID)
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("%w: %v", downloader.ErrCannotConnect, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header, b, nil
	}

	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()

	status, hdr, b, err := exec(sid)
	if err != nil {
		return nil, err
	}
	if status == 409 { // 会话握手：取 session-id 后重试
		sid := hdr.Get("X-Transmission-Session-Id")
		if sid == "" {
			sid = extractSessionID(string(b))
		}
		c.mu.Lock()
		c.sessionID = sid
		c.mu.Unlock()
		status, _, b, err = exec(sid)
		if err != nil {
			return nil, err
		}
	}
	if status == 401 {
		return nil, downloader.ErrInvalidCredentials
	}
	if status != 200 {
		return nil, fmt.Errorf("rpc status %d", status)
	}
	var resp struct {
		Result    string          `json:"result"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("parse rpc: %w", err)
	}
	if resp.Result != "success" {
		return nil, fmt.Errorf("rpc result: %s", resp.Result)
	}
	return resp.Arguments, nil
}

// TestConnection 取 session 信息。
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.rpc(ctx, "session-get", nil)
	return err
}

type trTorrent struct {
	ID                 int      `json:"id"`
	Name               string   `json:"name"`
	HashString         string   `json:"hashString"`
	TotalSize          int64    `json:"totalSize"`
	DownloadedEver     int64    `json:"downloadedEver"`
	UploadedEver       int64    `json:"uploadedEver"`
	PercentDone        float64  `json:"percentDone"`
	UploadRatio        float64  `json:"uploadRatio"`
	Status             int      `json:"status"`
	RateDownload       int64    `json:"rateDownload"`
	RateUpload         int64    `json:"rateUpload"`
	DownloadDir        string   `json:"downloadDir"`
	Labels             []string `json:"labels"`
	AddedDate          int64    `json:"addedDate"`
	PeersConnected     int      `json:"peersConnected"`
	PeersSendingToUs   int      `json:"peersSendingToUs"`
	PeersGettingFromUs int      `json:"peersGettingFromUs"`
	SeedRatioLimit     float64  `json:"seedRatioLimit"`
	SeedRatioMode      int      `json:"seedRatioMode"`
	SeedIdleLimit      int      `json:"seedIdleLimit"`
	SeedIdleMode       int      `json:"seedIdleMode"`
	QueuePosition      int      `json:"queuePosition"`
	DoneDate           int64    `json:"doneDate"`
}

// ListTorrents 用 torrent-get 拉取任务列表。
func (c *Client) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	args := map[string]any{
		"fields": []string{"id", "name", "hashString", "totalSize", "downloadedEver",
			"uploadedEver", "percentDone", "uploadRatio", "status", "rateDownload",
			"rateUpload", "downloadDir", "labels", "addedDate",
			"peersConnected", "peersSendingToUs", "peersGettingFromUs",
			"seedRatioLimit", "seedRatioMode", "seedIdleLimit", "seedIdleMode",
			"queuePosition", "doneDate"},
	}
	raw, err := c.rpc(ctx, "torrent-get", args)
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []trTorrent `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse torrents: %w", err)
	}
	res := make([]downloader.Torrent, 0, len(out.Torrents))
	for _, t := range out.Torrents {
		res = append(res, toTorrent(t))
	}
	return res, nil
}

// AddTorrent 添加种子（magnet/链接 或 .torrent 字节）。
// .torrent 走 metainfo=base64；TR 不支持 add 时设限速，add 后取 id 再 torrent-set 两步完成。
func (c *Client) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	args := map[string]any{}
	if len(opts.FileContent) > 0 && opts.URL == "" {
		args["metainfo"] = base64.StdEncoding.EncodeToString(opts.FileContent)
	} else {
		args["filename"] = opts.URL
	}
	if opts.SavePath != "" {
		args["download-dir"] = opts.SavePath
	}
	if len(opts.Tags) > 0 {
		args["labels"] = opts.Tags
	}
	if opts.Paused {
		args["paused"] = true
	}
	raw, err := c.rpc(ctx, "torrent-add", args)
	if err != nil {
		return err
	}
	if opts.UploadLimit > 0 || opts.DownloadLimit > 0 {
		if id := extractAddedID(raw); id > 0 {
			lim := map[string]any{"ids": []int{id}}
			if opts.UploadLimit > 0 {
				lim["uploadLimit"] = bytesToKB(opts.UploadLimit)
				lim["uploadLimited"] = true
			}
			if opts.DownloadLimit > 0 {
				lim["downloadLimit"] = bytesToKB(opts.DownloadLimit)
				lim["downloadLimited"] = true
			}
			_, _ = c.rpc(ctx, "torrent-set", lim)
		}
	}
	return nil
}

// Pause → torrent-stop。
func (c *Client) Pause(ctx context.Context, hashes []string) error {
	return c.rpcMethodIds(ctx, "torrent-stop", hashes)
}

// Resume → torrent-start。
func (c *Client) Resume(ctx context.Context, hashes []string) error {
	return c.rpcMethodIds(ctx, "torrent-start", hashes)
}

// Delete 删除任务。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	args := map[string]any{"ids": hashes, "delete-local-data": deleteFiles}
	_, err := c.rpc(ctx, "torrent-remove", args)
	return err
}

// SetCategory：Transmission 无分类概念，返回未实现。
func (c *Client) SetCategory(ctx context.Context, hashes []string, category string) error {
	return downloader.ErrNotImplemented
}

// GlobalStats 从 session-get 取全局速率；TotalSize 累加所有种子体积（参照 ut/dl 实现）。
func (c *Client) GlobalStats(ctx context.Context) (*downloader.GlobalStats, error) {
	raw, err := c.rpc(ctx, "session-get", nil)
	if err != nil {
		return nil, err
	}
	var s struct {
		DownloadSpeed int64 `json:"downloadSpeed"`
		UploadSpeed   int64 `json:"uploadSpeed"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	stats := &downloader.GlobalStats{DLRate: s.DownloadSpeed, UPRate: s.UploadSpeed}
	// TR 无全局体积端点，复用 ListTorrents 累加 totalSize（ut/dl 同模式）
	ts, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		stats.TotalSize += t.Size
	}
	return stats, nil
}

func (c *Client) rpcMethodIds(ctx context.Context, method string, hashes []string) error {
	args := map[string]any{}
	if len(hashes) > 0 {
		args["ids"] = hashes
	}
	_, err := c.rpc(ctx, method, args)
	return err
}

func toTorrent(t trTorrent) downloader.Torrent {
	// TR seedRatioMode：0=全局默认→-1，1=override→具体值，2=unlimited→-2（统一到 ShareLimits 语义）
	ratioLimit := -1.0
	switch t.SeedRatioMode {
	case 2:
		ratioLimit = -2
	case 1:
		ratioLimit = t.SeedRatioLimit
	}
	// TR 做种时长限制是「空闲时长 seedIdle」（分钟），映射为 SeedingTimeLimit，UI 注明语义差异
	seedTimeLimit := -1
	switch t.SeedIdleMode {
	case 2:
		seedTimeLimit = -2
	case 1:
		seedTimeLimit = t.SeedIdleLimit
	}
	var completedAt time.Time
	if t.DoneDate > 0 { // 0 = 未完成
		completedAt = time.Unix(t.DoneDate, 0)
	}
	return downloader.Torrent{
		Hash:       t.HashString,
		Name:       t.Name,
		Size:       t.TotalSize,
		Downloaded: t.DownloadedEver,
		Uploaded:   t.UploadedEver,
		Progress:   t.PercentDone,
		Ratio:      t.UploadRatio,
		State:      trStatusString(t.Status, t.PercentDone),
		DLRate:     t.RateDownload,
		UPRate:     t.RateUpload,
		SavePath:   t.DownloadDir,
		Tags:       t.Labels,
		Category:   "", // TR 无分类
		AddedAt:    time.Unix(t.AddedDate, 0),

		// —— 详情字段（TR 子集：无 swarm 总数/可用性/强制/顺序/首尾，留零值）——
		NumSeeds:         t.PeersSendingToUs,   // 给我们传的≈做种者
		NumLeechs:        t.PeersGettingFromUs, // 我们给传的≈下载者
		RatioLimit:       ratioLimit,
		SeedingTimeLimit: seedTimeLimit,
		Priority:         t.QueuePosition,
		CompletedAt:      completedAt,
	}
}

// trStatusString 把 Transmission 数值状态映射为统一状态串。
func trStatusString(status int, progress float64) string {
	switch status {
	case 0:
		if progress >= 1 {
			return "stopped"
		}
		return "paused"
	case 1, 2, 3:
		return "queued"
	case 4:
		return "downloading"
	case 5, 6:
		return "seeding"
	default:
		return "unknown"
	}
}

// extractSessionID 从 409 响应体里抽取会话 ID（兜底）。
func extractSessionID(body string) string {
	mark := "X-Transmission-Session-Id: "
	i := strings.Index(body, mark)
	if i < 0 {
		return ""
	}
	rest := body[i+len(mark):]
	if j := strings.IndexAny(rest, "<\r\n\""); j >= 0 {
		return rest[:j]
	}
	return strings.TrimSpace(rest)
}
