package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/trakt"
)

// traktSettingsKey 是 SystemConfig 中存储 Trakt 配置（含 OAuth 令牌）的 key（JSON）。
const traktSettingsKey = "trakt.settings"

// traktStore 抽象 SystemConfigOper 的 Get/Set（便于无 DB 单测；oper.SystemConfigOper 隐式实现）。
type traktStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// TraktSettings 是 Trakt 同步配置 + OAuth 令牌（落 SystemConfig，密钥字段脱敏对外）。
type TraktSettings struct {
	Enabled      bool   `json:"enabled"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"` // refresh token 需要；device 流可用占位
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"` // unix 秒
	SyncMovies   bool   `json:"sync_movies"`
	SyncShows    bool   `json:"sync_shows"`
	// 订阅默认值（可选）
	Resolution   string `json:"resolution"`
	DownloaderID uint   `json:"downloader_id"`
	// 同步状态
	LastSyncAt    int64  `json:"last_sync_at"`
	LastSyncCount int    `json:"last_sync_count"`
	LastError     string `json:"last_error"`
	// Device OAuth 进行中状态（用户尚未在浏览器完成授权时保留）
	PendingDeviceCode      string `json:"pending_device_code"`
	PendingUserCode        string `json:"pending_user_code"`
	PendingVerificationURL string `json:"pending_verification_url"`
	PendingExpiresAt       int64  `json:"pending_expires_at"`
	PendingInterval        int    `json:"pending_interval"`
}

// defaults 填充合理默认值。
func defaultTraktSettings() TraktSettings {
	return TraktSettings{RedirectURI: "urn:ietf:wg:oauth:2.0:oob", SyncMovies: true, SyncShows: true}
}

// TraktChain 编排 Trakt 同步：Device OAuth 授权 → 拉取 WatchList → 映射为本地订阅。
// 凭据/令牌落 SystemConfig（trakt.settings）；建订阅复用 SubscribeChain（幂等去重）。
// 整份配置是一个 JSON  blob，所有写入都走 patchSettings（锁内读-改-写），
// 且各路径只写自己负责的字段——见 patchSettings 注释。
type TraktChain struct {
	mu     sync.Mutex // 保护 settings 的读-改-写（GetSettings → 改 → saveSettings）
	subCh  *SubscribeChain
	store  traktStore
	notice *notice.Service
	client *trakt.Client
}

// NewTraktChain 构造。store 为 SystemConfigOper（或任意 traktStore 实现）。
func NewTraktChain(subCh *SubscribeChain, store traktStore, ns *notice.Service) *TraktChain {
	return &TraktChain{subCh: subCh, store: store, notice: ns, client: trakt.NewClient("")}
}

// patchSettings 锁内重读现值、按 fn 打补丁、写回。
// 不用「入口读到的快照 + 结束时整份写回」：同步要拉 watchlist 并逐条建订阅（数秒到数分钟），
// 期间用户可能在设置页改了分辨率/下载器/开关，整份写回会把他的改动静默撤销；
// 授权轮转同理握着跨 HTTP 的旧快照。谁产生的字段才写谁，其余以库里的最新值为准。
func (c *TraktChain) patchSettings(fn func(s *TraktSettings)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.GetSettings()
	fn(&s)
	return c.saveSettings(s)
}

// TraktConfigInput 是用户可编辑的 Trakt 配置（密钥空串=保留旧值）。
type TraktConfigInput struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
	RedirectURI  string
	SyncMovies   bool
	SyncShows    bool
	Resolution   string
	DownloaderID uint
}

// UpdateConfig 保存用户可编辑字段（锁内重读现值，令牌/同步状态等由各自的写入路径负责）。
// 合并放在 chain 里做而不是让 handler 先 Get 再 Save：两次调用之间若发生令牌轮转，
// 旧快照会把一次性 refresh_token 写回失效值，用户只能重新走一遍设备授权。
func (c *TraktChain) UpdateConfig(in TraktConfigInput) error {
	return c.patchSettings(func(s *TraktSettings) {
		s.Enabled = in.Enabled
		s.SyncMovies = in.SyncMovies
		s.SyncShows = in.SyncShows
		s.Resolution = in.Resolution
		s.DownloaderID = in.DownloaderID
		if in.RedirectURI != "" {
			s.RedirectURI = in.RedirectURI
		}
		// client_id/secret 改变 → 作废既有令牌（凭据已换，旧令牌无效）。
		if in.ClientID != "" && in.ClientID != s.ClientID {
			s.ClientID = in.ClientID
			s.AccessToken = ""
			s.RefreshToken = ""
			s.PendingDeviceCode = ""
		}
		if in.ClientSecret != "" && in.ClientSecret != s.ClientSecret {
			s.ClientSecret = in.ClientSecret
			s.AccessToken = ""
			s.RefreshToken = ""
			s.PendingDeviceCode = ""
		}
	})
}

// GetSettings 读取配置（含密钥，仅管理员侧用；handler 对外脱敏）。
func (c *TraktChain) GetSettings() TraktSettings {
	s := defaultTraktSettings()
	if c.store == nil {
		return s
	}
	raw, err := c.store.Get(traktSettingsKey)
	if err != nil || raw == "" {
		return s
	}
	_ = json.Unmarshal([]byte(raw), &s) // 合并到默认值上，缺失字段不丢
	return s
}

// saveSettings 落库。
func (c *TraktChain) saveSettings(s TraktSettings) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return c.store.Set(traktSettingsKey, string(raw))
}

// SaveSettings 保存配置（handler 用）。
func (c *TraktChain) SaveSettings(s TraktSettings) error { return c.saveSettings(s) }

// SyncAll 拉取 WatchList 并同步为本地订阅（cron 入口）。
// 同步期间的跨网络耗时（拉 watchlist + 逐条建订阅）里用户可能改设置，
// 因此结束时只写自己负责的同步字段，不写回入口快照。
func (c *TraktChain) SyncAll(ctx context.Context) error {
	s := c.GetSettings()
	if !s.Enabled || s.AccessToken == "" {
		return nil
	}
	if err := c.refreshTokenIfNeeded(ctx, &s, false); err != nil {
		_ = c.patchSettings(func(cur *TraktSettings) { cur.LastError = errtext.Text(err) })
		return err
	}
	created, err := c.syncWatchlist(ctx, &s)
	if err != nil {
		_ = c.patchSettings(func(cur *TraktSettings) { cur.LastError = errtext.Text(err) })
		return err
	}
	var syncAt int64
	_ = c.patchSettings(func(cur *TraktSettings) {
		cur.LastError = ""
		syncAt = time.Now().Unix()
		cur.LastSyncAt = syncAt
		cur.LastSyncCount = created
	})
	// 仅确有新增才通知：此前幂等路径也计数，每小时外发「新增 N 条」成功通知（噪声）。
	if c.notice != nil && created > 0 {
		_ = c.notice.Notice(ctx, notice.Input{
			Source: notice.SourceSubscribeHit, // 复用订阅命中通道
			Key:    fmt.Sprintf("trakt:%d", syncAt),
			Title:  "Trakt 同步完成",
			Body:   fmt.Sprintf("本次新增 %d 条订阅。", created),
			Level:  "success",
		}, false)
	}
	return nil
}

// SyncNow 手动触发同步，返回新增订阅数（handler 用）。
func (c *TraktChain) SyncNow(ctx context.Context) (int, error) {
	s := c.GetSettings()
	if s.AccessToken == "" {
		return 0, fmt.Errorf("%w：请先点「发起授权」在浏览器里输码完成，再来同步", ErrTraktNotAuthorized)
	}
	if err := c.refreshTokenIfNeeded(ctx, &s, false); err != nil {
		return 0, err
	}
	n, err := c.syncWatchlist(ctx, &s)
	if err != nil {
		_ = c.patchSettings(func(cur *TraktSettings) { cur.LastError = errtext.Text(err) })
		return n, err
	}
	_ = c.patchSettings(func(cur *TraktSettings) {
		cur.LastError = ""
		cur.LastSyncAt = time.Now().Unix()
		cur.LastSyncCount = n
	})
	return n, nil
}

// syncWatchlist 按 settings 拉取 movies/shows watchlist，映射为订阅（幂等去重）。
func (c *TraktChain) syncWatchlist(ctx context.Context, s *TraktSettings) (int, error) {
	created := 0
	if s.SyncMovies {
		items, err := c.watchlistWithRetry(ctx, s, "movies")
		if err != nil {
			return 0, fmt.Errorf("拉取电影 watchlist 失败：%w", err)
		}
		created += c.createFor(items, "movie", s)
	}
	if s.SyncShows {
		items, err := c.watchlistWithRetry(ctx, s, "shows")
		if err != nil {
			return created, fmt.Errorf("拉取剧集 watchlist 失败：%w", err)
		}
		created += c.createFor(items, "tv", s)
	}
	return created, nil
}

// watchlistWithRetry 拉取 watchlist；遇 401（令牌被服务端提前吊销）时强制刷新一次后重试，
// 弥补 refreshTokenIfNeeded 仅按 ExpiresAt 主动刷新、无法应对提前失效的短板。
func (c *TraktChain) watchlistWithRetry(ctx context.Context, s *TraktSettings, kind string) ([]trakt.WatchlistItem, error) {
	items, err := c.client.Watchlist(ctx, s.AccessToken, s.ClientID, kind)
	if err == nil {
		return items, nil
	}
	var unauth *trakt.UnauthorizedError
	if !errors.As(err, &unauth) {
		return nil, err
	}
	if rerr := c.refreshTokenIfNeeded(ctx, s, true); rerr != nil {
		return nil, rerr
	}
	// 新令牌已由 refreshTokenIfNeeded 立即落盘，这里不再整份写回快照。
	return c.client.Watchlist(ctx, s.AccessToken, s.ClientID, kind)
}

// createFor 把 watchlist 条目逐个建为订阅（幂等去重）。返回「真正新建」条数。
func (c *TraktChain) createFor(items []trakt.WatchlistItem, typ string, s *TraktSettings) int {
	n := 0
	for _, it := range items {
		if it.Title == "" || it.TraktID == 0 {
			continue
		}
		srcID := strconv.Itoa(it.TraktID)
		// 已在库（subscribed/paused）→ 直接跳过：① 幂等路径此前也计 n++，每小时误报
		// 「新增 N 条」；② 不得复活用户手动暂停的订阅（此前经 Create 的 paused 复用
		// 路径被静默改回 subscribed，用户的暂停每小时被撤销）。
		if existing, err := c.subCh.op.FindActiveBySource("trakt", srcID); err == nil && existing != nil {
			continue
		}
		sub := &models.Subscription{
			Source:       "trakt",
			SourceID:     srcID,
			Title:        it.Title,
			Year:         it.Year,
			Type:         typ,
			Resolution:   s.Resolution,
			DownloaderID: s.DownloaderID,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		_, err := c.subCh.Create(ctx, sub)
		cancel()
		if err != nil {
			slog.Warn("trakt create subscription failed", "title", it.Title, "err", err)
			continue
		}
		n++
	}
	return n
}
