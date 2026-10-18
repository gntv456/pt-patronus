package chain

import (
	"fmt"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/media"
)

// MediaSettings 是媒体源的可配置凭据/端点。任一字段为空 → 回退环境变量默认
// （config.go 加载的公开凭据，如 TMDB 公开 key / api.themoviedb.org）。
type MediaSettings struct {
	TMDBAPIKey     string
	TMDBAPIDomain  string
	DoubanSecret   string
	DoubanAppKey   string
	BangumiToken   string
	CunguangjiBase string // 寸光集（短剧）站点根，默认 https://dj.hxpt.org
	OMDbAPIKey     string // OMDb（IMDb 评分源）API Key，空则跳过 IMDb
}

// SystemConfig 键（KV 存储）。与 IYUUTokenKey 同范式：键名归该特性所有。
const (
	cfgKeyTMDBAPIKey     = "media.tmdb_api_key"
	cfgKeyTMDBAPIDomain  = "media.tmdb_api_domain"
	cfgKeyDoubanSecret   = "media.douban_secret"
	cfgKeyDoubanAppKey   = "media.douban_app_key"
	cfgKeyBangumiToken   = "media.bangumi_token"
	cfgKeyCunguangjiBase = "media.cunguangji_base"
	cfgKeyOMDbAPIKey     = "media.omdb_api_key"
)

// MediaSourceProvider 按 SystemConfig（回退环境变量默认）构建并缓存媒体源；
// ApplySettings 后 rebuild，使 UI 改动对搜索/推荐即时生效（无需重启）。
// MediaChain 与 RecommendChain 共用同一个 provider，故两处同步生效。
//
// 设计权衡：源（TMDB/Douban/Bangumi）在构造时捕获 key/domain/token，不会按请求重读，
// 故「写后即生效」靠 rebuild 整个切片实现（廉价：4 个轻量结构体 + http.Client），
// 而非给每个源加动态重读（侵入大、并发复杂）。参考 IYUU token「用时即读」的更简范式不适用此处，
// 因源把凭据烤进了构造器。
type MediaSourceProvider struct {
	cfg      *oper.SystemConfigOper
	defaults MediaSettings
	mu       sync.RWMutex
	sources  []media.MediaSource
}

// NewMediaSourceProvider 构造；立即按当前 SystemConfig + 默认构建一次源。
func NewMediaSourceProvider(cfg *oper.SystemConfigOper, defaults MediaSettings) *MediaSourceProvider {
	p := &MediaSourceProvider{cfg: cfg, defaults: defaults}
	p.rebuild()
	return p
}

// Sources 返回当前源切片（读锁；调用方不应修改）。
func (p *MediaSourceProvider) Sources() []media.MediaSource {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sources
}

// LoadSettings 从 SystemConfig 读各字段，空则回退 defaults。返回当前生效的完整设置。
func (p *MediaSourceProvider) LoadSettings() MediaSettings {
	get := func(key, def string) string {
		if v, err := p.cfg.Get(key); err == nil && v != "" {
			return v
		}
		return def
	}
	return MediaSettings{
		TMDBAPIKey:    get(cfgKeyTMDBAPIKey, p.defaults.TMDBAPIKey),
		TMDBAPIDomain: get(cfgKeyTMDBAPIDomain, p.defaults.TMDBAPIDomain),
		DoubanSecret:  get(cfgKeyDoubanSecret, p.defaults.DoubanSecret),
		DoubanAppKey:  get(cfgKeyDoubanAppKey, p.defaults.DoubanAppKey),
		BangumiToken:  get(cfgKeyBangumiToken, p.defaults.BangumiToken),
		CunguangjiBase: func() string {
			if v := get(cfgKeyCunguangjiBase, ""); v != "" {
				return v
			}
			return get("media.xingguangji_base", p.defaults.CunguangjiBase)
		}(),
		OMDbAPIKey: get(cfgKeyOMDbAPIKey, p.defaults.OMDbAPIKey),
	}
}

// ApplySettings 写入 SystemConfig（空字段跳过=不修改，便于 UI 只改部分项而不必重输全部），
// 然后 rebuild 源切片使改动即时生效。
func (p *MediaSourceProvider) ApplySettings(in MediaSettings) error {
	// rebuild 必须在返回前跑一次，无论写到第几项失败：
	// 原先只在全部写成功后 rebuild ⇒ 中途失败时 SystemConfig 已改、运行中的源仍是旧凭据，
	// 而设置页读的是 SystemConfig（显示新值），界面与真实上游两套数同时存在。
	defer p.rebuild()
	updates := map[string]string{
		cfgKeyTMDBAPIKey:     in.TMDBAPIKey,
		cfgKeyTMDBAPIDomain:  in.TMDBAPIDomain,
		cfgKeyDoubanSecret:   in.DoubanSecret,
		cfgKeyDoubanAppKey:   in.DoubanAppKey,
		cfgKeyBangumiToken:   in.BangumiToken,
		cfgKeyCunguangjiBase: in.CunguangjiBase,
		cfgKeyOMDbAPIKey:     in.OMDbAPIKey,
	}
	for k, v := range updates {
		if v == "" {
			continue // 空=不修改
		}
		if err := p.cfg.Set(k, v); err != nil {
			return fmt.Errorf("写入媒体源配置 %s 失败：%w", k, err)
		}
	}
	return nil
}

// rebuild 按当前 LoadSettings 重建源切片（写锁）。
func (p *MediaSourceProvider) rebuild() {
	s := p.LoadSettings()
	srcs := []media.MediaSource{
		media.NewTMDB(s.TMDBAPIKey, s.TMDBAPIDomain),
		media.NewDouban(s.DoubanSecret, s.DoubanAppKey),
		media.NewBangumi(s.BangumiToken),
		media.NewOMDb(s.OMDbAPIKey),
		media.NewCunguangji(s.CunguangjiBase),
		media.NewMusicBrainz(),
	}
	p.mu.Lock()
	p.sources = srcs
	p.mu.Unlock()
}
