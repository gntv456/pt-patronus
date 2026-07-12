package chain

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// SubtitleSettings 字幕源凭据 + 转换偏好。空 token/key → 该源跳过（ErrNotConfigured）。
type SubtitleSettings struct {
	AssrtToken       string
	OpenSubtitlesKey string
	SubHDCookie      string
	ZimukuCookie     string
	TargetLang       string // chs | cht | ""（不转换）
}

// SystemConfig 键。
const (
	cfgKeyAssrtToken       = "subtitle.assrt_token"
	cfgKeyOpenSubtitlesKey = "subtitle.opensubtitles_key"
	cfgKeySubHDCookie      = "subtitle.subhd_cookie"
	cfgKeyZimukuCookie     = "subtitle.zimuku_cookie"
	cfgKeySubtitleTarget   = "subtitle.target_lang"
)

// SubtitleSourceProvider 按 SystemConfig（回退 env 默认）构建并缓存字幕源；ApplySettings 后 rebuild。
type SubtitleSourceProvider struct {
	cfg      *oper.SystemConfigOper
	defaults SubtitleSettings
	hc       *httpclient.Client
	mu       sync.RWMutex
	providers []subtitle.Provider
}

// NewSubtitleSourceProvider 构造；内部建 SSRF 守卫的公网 HTTP 客户端（字幕站均为公网）。
func NewSubtitleSourceProvider(cfg *oper.SystemConfigOper, defaults SubtitleSettings) (*SubtitleSourceProvider, error) {
	hc, err := httpclient.New(httpclient.Options{Timeout: 20 * time.Second, MaxRetries: 2})
	if err != nil {
		return nil, err
	}
	p := &SubtitleSourceProvider{cfg: cfg, defaults: defaults, hc: hc}
	p.rebuild()
	return p, nil
}

func (p *SubtitleSourceProvider) Sources() []subtitle.Provider {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.providers
}

func (p *SubtitleSourceProvider) LoadSettings() SubtitleSettings {
	get := func(key, def string) string {
		if v, err := p.cfg.Get(key); err == nil && v != "" {
			return v
		}
		return def
	}
	return SubtitleSettings{
		AssrtToken:       get(cfgKeyAssrtToken, p.defaults.AssrtToken),
		OpenSubtitlesKey: get(cfgKeyOpenSubtitlesKey, p.defaults.OpenSubtitlesKey),
		SubHDCookie:      get(cfgKeySubHDCookie, p.defaults.SubHDCookie),
		ZimukuCookie:     get(cfgKeyZimukuCookie, p.defaults.ZimukuCookie),
		TargetLang:       get(cfgKeySubtitleTarget, p.defaults.TargetLang),
	}
}

// ApplySettings 写 SystemConfig（空字段跳过），rebuild 源使改动即时生效。
func (p *SubtitleSourceProvider) ApplySettings(in SubtitleSettings) error {
	updates := map[string]string{
		cfgKeyAssrtToken:       in.AssrtToken,
		cfgKeyOpenSubtitlesKey: in.OpenSubtitlesKey,
		cfgKeySubHDCookie:      in.SubHDCookie,
		cfgKeyZimukuCookie:     in.ZimukuCookie,
		cfgKeySubtitleTarget:   in.TargetLang,
	}
	for k, v := range updates {
		if v == "" {
			continue
		}
		if err := p.cfg.Set(k, v); err != nil {
			return err
		}
	}
	p.rebuild()
	return nil
}

func (p *SubtitleSourceProvider) rebuild() {
	s := p.LoadSettings()
	srcs := []subtitle.Provider{
		subtitle.NewAssrt(s.AssrtToken, p.hc),
		subtitle.NewOpenSubtitles(s.OpenSubtitlesKey, p.hc),
		subtitle.NewSubHD(s.SubHDCookie, p.hc),
		subtitle.NewZimuku(s.ZimukuCookie, p.hc),
	}
	p.mu.Lock()
	p.providers = srcs
	p.mu.Unlock()
}

// SubtitleChain 编排字幕搜索（并发跨源聚合）+ 下载（解 zip + 编码转 UTF-8 + 可选繁简）。
type SubtitleChain struct {
	Base
	provider *SubtitleSourceProvider
}

func NewSubtitleChain(b Base, provider *SubtitleSourceProvider) *SubtitleChain {
	return &SubtitleChain{Base: b, provider: provider}
}

// Search 并发跨源搜索（单源失败不阻断），按 Score 降序 + 可选语言过滤。
func (c *SubtitleChain) Search(ctx context.Context, query string, language string) ([]subtitle.Subtitle, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	providers := c.provider.Sources()
	opts := subtitle.SearchOpts{Language: language, Limit: 20}

	type srcOut struct {
		items []subtitle.Subtitle
		err   error
	}
	outs := make([]srcOut, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(i int, p subtitle.Provider) {
			defer wg.Done()
			items, err := p.Search(ctx, query, opts)
			outs[i] = srcOut{items: items, err: err}
		}(i, p)
	}
	wg.Wait()

	var all []subtitle.Subtitle
	for i, p := range providers {
		o := outs[i]
		if o.err != nil {
			if errors.Is(o.err, subtitle.ErrNotConfigured) {
				continue
			}
			slog.Warn("subtitle source search failed", "source", p.Name(), "err", o.err)
			continue
		}
		// 可选语言过滤（bi 双语始终保留）
		for _, s := range o.items {
			if language == "" || s.LangCode == language || s.LangCode == "bi" || s.LangCode == "und" {
				all = append(all, s)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	return all, nil
}

// Download 解析 sub.Source 对应源下载 → 解 zip → 转 UTF-8 → 可选繁简。
// 返回最终字幕字节 + 文件名。targetLang 由 provider 配置决定。
func (c *SubtitleChain) Download(ctx context.Context, sub subtitle.Subtitle) ([]byte, string, error) {
	var target subtitle.Provider
	for _, p := range c.provider.Sources() {
		if p.Name() == sub.Source {
			target = p
			break
		}
	}
	if target == nil {
		return nil, "", errors.New("未找到字幕源: " + sub.Source)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, name, err := target.Download(ctx, sub)
	if err != nil {
		return nil, "", err
	}
	data, name, err = subtitle.Extract(data, name)
	if err != nil {
		return nil, "", err
	}
	// 编码转 UTF-8
	decoded := decodeSub(data)
	// 繁简转换（仅文本字幕 + 配置了目标语言 + 源语言与目标不同）
	settings := c.provider.LoadSettings()
	if shouldConvert(path.Ext(name), sub.LangCode, settings.TargetLang) {
		if conv, err := subtitle.Convert(string(decoded), settings.TargetLang); err == nil {
			decoded = []byte(conv)
		}
	}
	return decoded, name, nil
}

// Config 当前生效设置（供 handler 脱敏回显）。
func (c *SubtitleChain) Config() SubtitleSettings { return c.provider.LoadSettings() }

// SetConfig 保存并即时 rebuild（委托 provider）。
func (c *SubtitleChain) SetConfig(in SubtitleSettings) error { return c.provider.ApplySettings(in) }

// decodeSub 暴露包内解码（薄封装，便于单测）。
func decodeSub(data []byte) []byte { return subtitle.DecodeUTF8(data) }

// shouldConvert 是否需要繁简转换：文本字幕 + 目标语言是 chs/cht + 与源语言不同（bi/und 不转）。
func shouldConvert(ext, srcLang, target string) bool {
	if target != "chs" && target != "cht" {
		return false
	}
	ext = strings.ToLower(ext)
	if ext != ".srt" && ext != ".ass" && ext != ".ssa" && ext != ".vtt" {
		return false
	}
	if srcLang != "chs" && srcLang != "cht" {
		return false // 双语/未知/英文不转
	}
	return srcLang != target
}
