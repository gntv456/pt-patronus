package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
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

// settingsStore 字幕配置读写口。#374 抽出窄接口是为了让 ApplySettings 的
// 「写到一半失败」能在测试里被确定性地造出来 —— 此前 cfg 是具体类型
// *oper.SystemConfigOper，测试只能整体 mock 掉，那等于把被测逻辑一起 mock 掉了。
type settingsStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// SubtitleSourceProvider 按 SystemConfig（回退 env 默认）构建并缓存字幕源；ApplySettings 后 rebuild。
type SubtitleSourceProvider struct {
	cfg       settingsStore
	defaults  SubtitleSettings
	hc        *httpclient.Client
	mu        sync.RWMutex
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
	// #374 这五个字段在界面上是**同一次提交的一个整体**（用户点一次「保存」），
	// 从前是逐键写、失败即 return：第 3 个键失败时前 2 个已经入库、后 2 个还是旧值，
	// 且 rebuild 没执行 ⇒ 界面显示的和实际生效的不是一回事，而用户完全不知道。
	//
	// 改成「先备份旧值 → 全写 → 任一失败则按逆序回滚」：失败时回到「什么都没变」，
	// 与界面呈现的那次保存一致。回滚本身也可能失败（同一个库正在出问题），
	// 那种情况把回滚错误一并带出，至少日志里能看出库里可能停在中间态。
	type kv struct{ k, v string }
	var pending []kv
	for k, v := range updates {
		if v == "" {
			continue // 空 = 不修改（既有约定，不因这次改动而变）
		}
		pending = append(pending, kv{k, v})
	}
	backups := make([]kv, 0, len(pending))
	for _, e := range pending {
		old, _ := p.cfg.Get(e.k) // 读失败按空串处理：原来就没有这个键
		backups = append(backups, kv{e.k, old})
		if err := p.cfg.Set(e.k, e.v); err != nil {
			var rollbackErrs []error
			for i := len(backups) - 2; i >= 0; i-- { // 逆序，失败的那一个不必回滚（没写进去）
				if rbErr := p.cfg.Set(backups[i].k, backups[i].v); rbErr != nil {
					rollbackErrs = append(rollbackErrs, rbErr)
				}
			}
			if len(rollbackErrs) > 0 {
				return fmt.Errorf("保存字幕源配置失败（%w），且回滚 %d 个已写入的键也失败，配置可能停在中间态：%w",
					err, len(rollbackErrs), errors.Join(rollbackErrs...))
			}
			return fmt.Errorf("保存字幕源配置失败，已回滚全部改动：%w", err)
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
	provider SubtitleProvider
}

// SubtitleProvider 链层用到的字幕源能力。真实现是 *SubtitleSourceProvider；
// 抽出来的原因是「全部源失败」这条判据必须能在不打外网的情况下被证伪——
// 从前只有具体类型可传，测试只能拿真公网源跑（在 CI 上时好时坏）。
type SubtitleProvider interface {
	Sources() []subtitle.Provider
	LoadSettings() SubtitleSettings
	ApplySettings(in SubtitleSettings) error
}

// ErrSubtitleSourcesFailed 参与进来的源一个都没答话。
// 它是哨兵而不是裸 fmt.Errorf：界面要按「上游没答话」回 502 + 具体原因，
// 不能继续用 500「服务内部错误，请稍后重试」打发（用户既不知道是网络、被挡还是配置问题）。
// 文案是人话而不是英文 id：handler 侧用 WithDetail 把这句话原样送进响应体。
var ErrSubtitleSourcesFailed = errors.New("字幕源全部查询失败")

func NewSubtitleChain(b Base, provider SubtitleProvider) *SubtitleChain {
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
		safego.Go("subtitle:137", func() {
			defer wg.Done()
			// per-source 隔离：崩掉的源要记成它自己的失败。零值槽 err=nil 会被下游当成
			// 「这个源没搜到字幕」，全崩时页面就是「没找到字幕」的假绿。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("subtitle source panicked", "source", p.Name(), "err", r)
					outs[i] = srcOut{err: fmt.Errorf("字幕源异常: %v", r)}
				}
			}()
			items, err := p.Search(ctx, query, opts)
			outs[i] = srcOut{items: items, err: err}
		})
	}
	wg.Wait()

	var all []subtitle.Subtitle
	attempted, responded := 0, 0
	// 逐源原因要留在错误里：从前只进 slog.Warn，界面拿到的是 500「服务内部错误」，
	// 用户看不出是超时、被 403 挡了还是没配 cookie/token。
	var failed []string
	for i, p := range providers {
		o := outs[i]
		if o.err != nil {
			if errors.Is(o.err, subtitle.ErrNotConfigured) {
				continue // 未配置：既不算参与也不算失败
			}
			attempted++
			// 逐源原因走 errtext 翻译口（超时/SSRF 等有中文口径），再钳长度：
			// 不把「为什么没答话」在聚合句里再抄一遍第三种说法。
			failed = append(failed, p.Name()+": "+errtext.RespSnippet([]byte(errtext.Text(o.err))))
			slog.Warn("subtitle source search failed", "source", p.Name(), "err", o.err)
			continue
		}
		attempted++
		responded++
		// 可选语言过滤（bi 双语始终保留）
		for _, s := range o.items {
			if language == "" || s.LangCode == language || s.LangCode == "bi" || s.LangCode == "und" {
				all = append(all, s)
			}
		}
	}
	// 参与进来的源一个都没答话：「没读回来」不能渲染成「没找到字幕」
	// （对齐 media_rating「全部 N 个评分源查询失败」的口径）。
	if attempted > 0 && responded == 0 {
		return nil, fmt.Errorf("%w（共 %d 个源，原因：%s）——请检查本机到字幕站的网络，或在字幕设置里补 cookie/token",
			ErrSubtitleSourcesFailed, attempted, strings.Join(failed, "；"))
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
		} else {
			// 转换失败仍返回原文（有字幕比没字幕好），但必须留痕：
			// 否则用户设了「简体」却拿到繁体，全程查不到是没转还是转失败。
			slog.Warn("subtitle convert failed, keeping original script", "source", sub.Source, "target", settings.TargetLang, "err", err)
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
