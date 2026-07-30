package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
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
type TraktChain struct {
	subCh  *SubscribeChain
	store  traktStore
	notice *notice.Service
	client *trakt.Client
}

// NewTraktChain 构造。store 为 SystemConfigOper（或任意 traktStore 实现）。
func NewTraktChain(subCh *SubscribeChain, store traktStore, ns *notice.Service) *TraktChain {
	return &TraktChain{subCh: subCh, store: store, notice: ns, client: trakt.NewClient("")}
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

// StartDeviceAuth 发起设备授权，返回 device code + 把 pending 状态存库（前端展示 user_code/url）。
func (c *TraktChain) StartDeviceAuth(ctx context.Context) (TraktSettings, error) {
	s := c.GetSettings()
	if s.ClientID == "" || s.ClientSecret == "" {
		return s, errors.New("缺少 client_id/client_secret，请先在 Trakt 创建应用并填入")
	}
	dc, err := c.client.DeviceCode(ctx, trakt.Credentials{ClientID: s.ClientID, ClientSecret: s.ClientSecret})
	if err != nil {
		return s, err
	}
	s.PendingDeviceCode = dc.DeviceCode
	s.PendingUserCode = dc.UserCode
	s.PendingVerificationURL = dc.VerificationURL
	s.PendingExpiresAt = time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second).Unix()
	s.PendingInterval = dc.Interval
	s.LastError = ""
	if err := c.saveSettings(s); err != nil {
		return s, err
	}
	return s, nil
}

// PollDeviceStatus 轮询设备授权：若用户已完成授权则换到令牌并落库；未完成返 (false, nil)。
func (c *TraktChain) PollDeviceStatus(ctx context.Context) (bool, error) {
	s := c.GetSettings()
	if s.PendingDeviceCode == "" {
		return s.AccessToken != "", nil
	}
	if time.Now().Unix() > s.PendingExpiresAt {
		s.PendingDeviceCode = ""
		_ = c.saveSettings(s)
		return false, errors.New("设备授权码已过期，请重新发起")
	}
	tok, err := c.client.PollDeviceToken(ctx, s.PendingDeviceCode,
		trakt.Credentials{ClientID: s.ClientID, ClientSecret: s.ClientSecret})
	if err != nil {
		var dte *trakt.DeviceTokenError
		if errors.As(err, &dte) {
			// authorization_pending / slow_down：未完成，保持 pending
			return false, nil
		}
		return false, err
	}
	s.AccessToken = tok.AccessToken
	s.RefreshToken = tok.RefreshToken
	s.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Unix()
	s.PendingDeviceCode = ""
	s.PendingUserCode = ""
	s.LastError = ""
	if err := c.saveSettings(s); err != nil {
		return false, err
	}
	return true, nil
}

// refreshTokenIfNeeded 令牌临近过期则刷新；刷新失败清令牌并记错误（要求重新授权）。
func (c *TraktChain) refreshTokenIfNeeded(ctx context.Context, s *TraktSettings, force bool) error {
	if s.AccessToken == "" {
		return errors.New("未授权，请先完成设备授权")
	}
	if !force && (s.ExpiresAt == 0 || time.Now().Unix() < s.ExpiresAt-60) {
		return nil // 未到期（force=true 时跳过，用于 401 后强制刷新）
	}
	if s.RefreshToken == "" {
		return errors.New("令牌过期且无 refresh_token，请重新授权")
	}
	tok, err := c.client.RefreshToken(ctx, s.RefreshToken, s.RedirectURI,
		trakt.Credentials{ClientID: s.ClientID, ClientSecret: s.ClientSecret})
	if err != nil {
		s.AccessToken = ""
		s.RefreshToken = ""
		s.LastError = "刷新令牌失败：" + err.Error()
		_ = c.saveSettings(*s)
		return errors.New("刷新令牌失败，请重新授权")
	}
	s.AccessToken = tok.AccessToken
	s.RefreshToken = tok.RefreshToken
	s.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Unix()
	return nil
}

// SyncAll 拉取 WatchList 并同步为本地订阅（cron 入口）。
func (c *TraktChain) SyncAll(ctx context.Context) error {
	s := c.GetSettings()
	if !s.Enabled || s.AccessToken == "" {
		return nil
	}
	if err := c.refreshTokenIfNeeded(ctx, &s, false); err != nil {
		s.LastError = err.Error()
		_ = c.saveSettings(s)
		return err
	}
	created, err := c.syncWatchlist(ctx, &s)
	if err != nil {
		s.LastError = err.Error()
		_ = c.saveSettings(s)
		return err
	}
	s.LastError = ""
	s.LastSyncAt = time.Now().Unix()
	s.LastSyncCount = created
	_ = c.saveSettings(s)
	if c.notice != nil {
		_ = c.notice.Notice(ctx, notice.Input{
			Source: notice.SourceSubscribeHit, // 复用订阅命中通道
			Key:    fmt.Sprintf("trakt:%d", s.LastSyncAt),
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
		return 0, errors.New("未授权，请先完成设备授权")
	}
	if err := c.refreshTokenIfNeeded(ctx, &s, false); err != nil {
		return 0, err
	}
	n, err := c.syncWatchlist(ctx, &s)
	if err == nil {
		s.LastError = ""
		s.LastSyncAt = time.Now().Unix()
		s.LastSyncCount = n
	} else {
		s.LastError = err.Error()
	}
	_ = c.saveSettings(s)
	return n, err
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
	_ = c.saveSettings(*s)
	return c.client.Watchlist(ctx, s.AccessToken, s.ClientID, kind)
}

// createFor 把 watchlist 条目逐个建为订阅（subCh.Create 幂等：同 source+source_id 不重复）。
func (c *TraktChain) createFor(items []trakt.WatchlistItem, typ string, s *TraktSettings) int {
	n := 0
	for _, it := range items {
		if it.Title == "" || it.TraktID == 0 {
			continue
		}
		sub := &models.Subscription{
			Source:       "trakt",
			SourceID:     strconv.Itoa(it.TraktID),
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
