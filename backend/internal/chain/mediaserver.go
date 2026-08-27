package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/mediaserver"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// MediaServerChain 编排媒体服务器：CRUD 透传 + 按需构建 client（解密凭据）调 REST
// （库扫描 / 刷新 / 通知 / 封面生成）+ 下载完成事件触发刷新 + webhook 接收。
//
// client 按需构建（媒体服务器通常个位数，构造廉价；HTTP 调用才是开销），无需 manager 缓存。
// 复用一个 AllowPrivate 的内网 HTTP 客户端（同下载器范式，媒体服务器多为内网部署）。
type MediaServerChain struct {
	subtitleFixer SubtitleFixer // 字幕库级补齐（nil=未启用）
	Base
	op     *oper.MediaServerOper
	hc     *httpclient.Client
	notice *notice.Service
}

// NewMediaServerChain 构造；内部构建 AllowPrivate 的内网 HTTP 客户端（同下载器范式）。
func NewMediaServerChain(b Base, op *oper.MediaServerOper, noticeSvc *notice.Service) (*MediaServerChain, error) {
	lan, err := httpclient.New(httpclient.Options{AllowPrivate: true, Timeout: 20 * time.Second})
	if err != nil {
		return nil, err
	}
	return &MediaServerChain{Base: b, op: op, hc: lan, notice: noticeSvc}, nil
}

// ---- CRUD 透传 ----

func (c *MediaServerChain) List() ([]models.MediaServer, error) { return c.op.List() }

func (c *MediaServerChain) Create(in oper.MediaServerInput) (*models.MediaServer, error) {
	return c.op.Create(in)
}

func (c *MediaServerChain) Update(id uint, in oper.MediaServerInput) (*models.MediaServer, error) {
	return c.op.Update(id, in)
}

func (c *MediaServerChain) Delete(id uint) error { return c.op.Delete(id) }

// client 为指定服务器构建 client（解密 token）。
func (c *MediaServerChain) client(id uint) (mediaserver.Client, *models.MediaServer, error) {
	m, err := c.op.Get(id)
	if err != nil {
		return nil, nil, err
	}
	token, err := c.op.Token(m)
	if err != nil {
		return nil, nil, fmt.Errorf("解密凭据失败: %w", err)
	}
	cli, err := mediaserver.NewClient(mediaserver.Server{
		ID: m.ID, Type: m.Type, URL: m.URL, Username: m.Username, Token: token,
	}, c.hc)
	if err != nil {
		return nil, nil, err
	}
	return cli, m, nil
}

// Test 连接 + 鉴权探活，返回服务信息。
func (c *MediaServerChain) Test(ctx context.Context, id uint) (string, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return "", err
	}
	return cli.Test(ctx)
}

// Libraries 列媒体库。
func (c *MediaServerChain) Libraries(ctx context.Context, id uint) ([]mediaserver.Library, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return nil, err
	}
	lib, err := cli.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		lib = []mediaserver.Library{}
	}
	return lib, nil
}

// LibraryScan 触发整库扫描。
func (c *MediaServerChain) LibraryScan(ctx context.Context, id uint, libraryID string) error {
	cli, _, err := c.client(id)
	if err != nil {
		return err
	}
	return cli.LibraryScan(ctx, libraryID)
}

// Refresh 刷新库元数据。
func (c *MediaServerChain) Refresh(ctx context.Context, id uint, libraryID string) error {
	cli, _, err := c.client(id)
	if err != nil {
		return err
	}
	return cli.Refresh(ctx, libraryID)
}

// NotifyAll 向所有服务器发管理员通知（刷流完成 / 下载完成等场景广播）。
func (c *MediaServerChain) NotifyAll(ctx context.Context, title, body string) {
	ms, err := c.op.List()
	if err != nil || len(ms) == 0 {
		return
	}
	for i := range ms {
		if err := c.NotifyOne(ctx, ms[i].ID, title, body); err != nil {
			slog.Debug("media server notify failed", "server", ms[i].Name, "err", err)
		}
	}
}

// NotifyOne 向单个服务器发管理员通知。
func (c *MediaServerChain) NotifyOne(ctx context.Context, id uint, title, body string) error {
	cli, _, err := c.client(id)
	if err != nil {
		return err
	}
	return cli.Notify(ctx, title, body)
}

// Cover 生成某库的封面拼图 JPEG 字节。
func (c *MediaServerChain) Cover(ctx context.Context, id uint, libraryID string, opt mediaserver.CoverOptions) ([]byte, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return nil, err
	}
	return mediaserver.GenerateCover(ctx, cli, libraryID, opt)
}

// OnDownloadCompleted 下载完成钩子：对所有开启 RefreshOnDownload 的服务器触发整库扫描。
// 单服务器失败不阻断其余；不向调用方报错（事件副作用，best-effort）。
func (c *MediaServerChain) OnDownloadCompleted(ctx context.Context, torrentName string) {
	ms, err := c.op.ListRefreshOnDownload()
	if err != nil || len(ms) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for i := range ms {
		srv := &ms[i]
		cli, _, err := c.client(srv.ID)
		if err != nil {
			continue
		}
		libs, err := cli.Libraries(ctx)
		if err != nil {
			slog.Debug("media server libraries failed", "server", srv.Name, "err", err)
			continue
		}
		for _, lib := range libs {
			if err := cli.LibraryScan(ctx, lib.ID); err != nil {
				slog.Debug("media server scan failed", "server", srv.Name, "library", lib.Name, "err", err)
			}
		}
	}
	c.Emit(ctx, "MediaServerRefresh", map[string]any{"trigger": "download", "name": torrentName, "servers": len(ms)})
}

// ReceiveWebhook 接收媒体服务器播放/事件 webhook：best-effort 解析标题，发通知中心 + 广播事件。
// payload 是原始请求体（Emby/JF JSON 或 Plex form/JSON）。
func (c *MediaServerChain) ReceiveWebhook(ctx context.Context, id uint, payload []byte) {
	m, err := c.op.Get(id)
	if err != nil || !m.NotifyOnPlay {
		return
	}
	title, event := parseMediaWebhook(payload)
	if title == "" {
		return
	}
	c.Emit(ctx, "MediaServerWebhook", map[string]any{"server": m.Name, "event": event, "title": title})
	if c.notice != nil {
		_ = c.notice.Notice(ctx, notice.Input{
			Source: "mediaserver",
			Key:    fmt.Sprintf("ms:%d:%s", id, title),
			Title:  fmt.Sprintf("媒体播放：%s", title),
			Body:   fmt.Sprintf("%s · %s", m.Name, event),
			Level:  "info",
		}, true)
	}
}

// parseMediaWebhook best-effort 从 webhook 载荷提取标题与事件类型。
// 兼容 Emby/JF（{Title,Event} 或嵌套 Item）与 Plex（{event, Metadata:{title}}）。
func parseMediaWebhook(payload []byte) (title, event string) {
	if len(payload) == 0 {
		return "", ""
	}
	// 顶层常见字段（Emby/JF 直发 Title；Plex 发 event + Metadata）。
	var top struct {
		Title    string `json:"Title"`
		Name     string `json:"Name"`
		Event    string `json:"Event"`
		Metadata struct {
			Title string `json:"title"`
		} `json:"Metadata"`
		Item struct {
			Name string `json:"Name"`
		} `json:"Item"`
	}
	if json.Unmarshal(payload, &top) == nil {
		title = firstNonEmpty(top.Title, top.Metadata.Title, top.Item.Name, top.Name)
		event = top.Event
	}
	if title == "" {
		// Plex form-encoded payload（&event=media.play&...&metadata[title]=X）兜底。
		title, event = parsePlexForm(string(payload))
	}
	event = normalizeMediaEvent(event)
	return title, event
}

// parsePlexForm 从 form-encoded body 提取 title/event（Plex webhook 常以表单发）。
func parsePlexForm(body string) (title, event string) {
	for _, kv := range strings.Split(body, "&") {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k, v := kv[:eq], kv[eq+1:]
		switch {
		case strings.HasSuffix(k, "[title]") || k == "metadata[title]":
			if title == "" {
				title = v
			}
		case k == "event":
			event = v
		}
	}
	return title, event
}

func normalizeMediaEvent(e string) string {
	e = strings.ToLower(strings.TrimSpace(e))
	if e == "" {
		return "play"
	}
	return e
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// Get 取媒体服务器配置（媒体代理用）。
func (c *MediaServerChain) Get(id uint) (*models.MediaServer, error) {
	if c == nil || c.op == nil {
		return nil, fmt.Errorf("media server chain not initialized")
	}
	return c.op.Get(id)
}

// MediaPath 查询条目媒体文件路径（媒体代理 302 直链用；仅 Emby/JF 支持）。
func (c *MediaServerChain) MediaPath(ctx context.Context, id uint, itemID string) (string, error) {
	cli, m, err := c.client(id)
	if err != nil {
		return "", err
	}
	pc, ok := cli.(mediaPathClient)
	if !ok {
		return "", fmt.Errorf("媒体服务器类型 %s 不支持路径查询", m.Type)
	}
	return pc.MediaPath(ctx, itemID)
}

// mediaPathClient PlaybackInfo 路径查询能力（embyJellyfinClient 实现）。
type mediaPathClient interface {
	MediaPath(ctx context.Context, itemID string) (string, error)
}
