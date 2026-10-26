package chain

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
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
	subtitleFixer SubtitleFixer    // 字幕库级补齐（nil=未启用）
	subtitleFix   subtitleFixState // 补齐防重入 + 报告同步（见 subtitlefix.go）
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

// LibraryScanAll 扫描该服务器全部媒体库，返回成功扫描的库数。
// 一个都没扫成功就不是成功：调用方（API/工具/插件）拿 scanned=0 或 error，
// 不能像此前工具面那样把 library_id 传空后拿 scanned:1 —— 那会拼出 /Items//Refresh
// 这种无效请求，或干脆什么都没做。
func (c *MediaServerChain) LibraryScanAll(ctx context.Context, id uint) (int, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return 0, err
	}
	libs, err := cli.Libraries(ctx)
	if err != nil {
		return 0, err
	}
	var scanned int
	var lastErr error
	for _, lib := range libs {
		if e := cli.LibraryScan(ctx, lib.ID); e != nil {
			lastErr = e
			slog.Warn("media server scan-all: library scan failed", "server_id", id, "library", lib.Name, "err", e)
			continue
		}
		scanned++
	}
	if scanned > 0 {
		return scanned, nil
	}
	if lastErr != nil {
		return 0, fmt.Errorf("%d 个库全部扫描失败: %w", len(libs), lastErr)
	}
	return 0, fmt.Errorf("该媒体服务器没有可扫描的库")
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

// Cover 生成某库的封面拼图 JPEG 字节 + 本轮账面（贴上了几张、几张没地址、几张取失败）。
func (c *MediaServerChain) Cover(ctx context.Context, id uint, libraryID string, opt mediaserver.CoverOptions) ([]byte, mediaserver.CoverStats, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return nil, mediaserver.CoverStats{}, err
	}
	return mediaserver.GenerateCover(ctx, cli, libraryID, opt)
}

// OnDownloadCompleted 下载完成钩子：对所有开启 RefreshOnDownload 的服务器触发整库扫描。
// 单服务器失败不阻断其余；不向调用方报错（事件副作用，best-effort）。
// 但「best-effort」不等于「无声」：以前所有失败都只打 Debug（默认日志级别看不见），
// 广播出去的事件又只带 servers 数量，用户开了自动刷库却一台都没扫成时没有任何信号。
// 现在服务器级失败打 Warn 并把原因放进事件（scanned/failures），前端与排障都能看见。
func (c *MediaServerChain) OnDownloadCompleted(ctx context.Context, torrentName string) {
	ms, err := c.op.ListRefreshOnDownload()
	if err != nil || len(ms) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	scanned := 0
	var failures []string
	for i := range ms {
		srv := &ms[i]
		cli, _, err := c.client(srv.ID)
		if err != nil {
			slog.Warn("media server refresh: client unavailable", "server", srv.Name, "err", err)
			failures = append(failures, fmt.Sprintf("%s：连接配置不可用（%v）", srv.Name, err))
			continue
		}
		libs, err := cli.Libraries(ctx)
		if err != nil {
			slog.Warn("media server refresh: libraries failed", "server", srv.Name, "err", err)
			failures = append(failures, fmt.Sprintf("%s：库列表读取失败（%v）", srv.Name, err))
			continue
		}
		ok := 0
		for _, lib := range libs {
			if err := cli.LibraryScan(ctx, lib.ID); err != nil {
				slog.Warn("media server refresh: scan failed", "server", srv.Name, "library", lib.Name, "err", err)
				continue
			}
			ok++
		}
		if ok == 0 && len(libs) > 0 {
			failures = append(failures, fmt.Sprintf("%s：%d 个库全部扫描失败", srv.Name, len(libs)))
		}
		scanned += ok
	}
	c.Emit(ctx, "MediaServerRefresh", map[string]any{
		"trigger": "download", "name": torrentName,
		"servers": len(ms), "scanned": scanned, "failures": failures,
	})
}

// ReceiveWebhook 接收媒体服务器播放/事件 webhook：best-effort 解析标题，发通知中心 + 广播事件。
// payload 是原始请求体（Emby/JF JSON 或 Plex form/JSON）；providedToken 是回调携带的
// 来源凭证（query token 或 X-Emby-Token 头）。
//
// 来源校验（P2 修复）：服务器配置了 API Token 时恒定时间比对——阻断匿名伪造
// 「媒体播放：<任意文案>」注入通知中心/外发渠道（TG 等钓鱼文本）。未配置 token 的
// 服务器保留旧行为（部分 Emby webhook 模板不含 token 字段），伪造成本不变。
// token 读取/解密失败按 fail-closed 处理（拒绝并告警）：密文损坏时旧实现把
// 「校验不了」与「没配 token」同待，静默跳过校验，公开端点可被匿名伪造。
func (c *MediaServerChain) ReceiveWebhook(ctx context.Context, id uint, payload []byte, providedToken string) {
	m, err := c.op.Get(id)
	if err != nil || !m.NotifyOnPlay {
		return
	}
	want, werr := c.op.Token(m)
	if werr != nil {
		slog.Warn("mediaserver webhook token unreadable, reject", "server", m.Name, "err", werr)
		return
	}
	if want != "" && subtle.ConstantTimeCompare([]byte(providedToken), []byte(want)) != 1 {
		return
	}
	title, event := parseMediaWebhook(payload)
	if title == "" {
		return
	}
	c.Emit(ctx, "MediaServerWebhook", map[string]any{"server": m.Name, "event": event, "title": title})
	if c.notice != nil {
		_ = c.notice.Notice(ctx, notice.Input{
			Source: notice.SourceMediaServer,
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
	// 用 url.ParseQuery 而不是手工按 &/= 切：Plex 表单是 application/x-www-form-urlencoded，
	// 空格是 '+'、方括号与中文是百分号编码。手工切会把标题显示成 "Foo+Bar" / "%E7%94%B5…"，
	// 且 metadata%5Btitle%5D 这种键名根本匹配不上。
	vals, err := url.ParseQuery(body)
	if err != nil {
		return "", ""
	}
	for k, vs := range vals {
		if len(vs) == 0 {
			continue
		}
		switch {
		case strings.HasSuffix(k, "[title]") || k == "metadata[title]":
			if title == "" {
				title = vs[0]
			}
		case k == "event":
			event = vs[0]
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
