// Package subauto 实现字幕自动下载（Bazarr / ChineseSubFinder 模式）：
// 监听媒体入库事件 transfer_done → 按语言 / provider 优先级搜索字幕 → 下载落盘到
// 媒体同目录（同名 .srt/.ass）→ 发通知。替代 Subtitle 页纯手动搜索，做到「整理完成即有字幕」。
//
// 设计：纯逻辑编排，外部依赖（事件总线 / 字幕搜索 / 配置 / 通知）经接口注入，便于单测。
// 异步处理——事件处理器收到 transfer_done 后立即 goroutine 跑搜索/下载，避免阻塞
// 事件总线 Publish（其内部 wg.Wait 会等所有 handler 返回）拖慢 transfer.record()。
//
// 包名用 subauto（非 automation）以避免与 internal/automation（后台 cron 服务）同名冲突。
package subauto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// EventTransferDone 整理完成事件名（与 notice.SourceTransferDone 对齐；由 chain.TransferChain.record 在成功时 Emit）。
const EventTransferDone = "transfer_done"

// configKey SystemConfig 存储键。
const configKey = "subtitle.automation.config"

// Config 字幕自动化配置（存 SystemConfig，JSON）。
type Config struct {
	Enabled       bool     `json:"enabled"`         // 总开关（默认关，用户显式开）
	Languages     []string `json:"languages"`       // 语言优先级（归一化 langcode：chs/cht/eng/bi；别名 zh→chs / en→eng 自动归一）
	Providers     []string `json:"providers"`       // provider 优先级（源 Name：assrt/opensubtitles/subhd/zimuku）；空=不限
	OnlyIfMissing bool     `json:"only_if_missing"` // 媒体目录已有任意字幕则跳过（默认 true，不覆盖）
	SearchTimeout int      `json:"search_timeout"`  // 单媒体搜+下载整体超时秒（默认 60）
}

// DefaultConfig 默认配置（关、简→繁→英、不覆盖已有）。
func DefaultConfig() Config {
	return Config{
		Enabled:       false,
		Languages:     []string{"chs", "cht", "eng"},
		Providers:     nil,
		OnlyIfMissing: true,
		SearchTimeout: 60,
	}
}

// KVStore 抽象 oper.SystemConfigOper 的 Get/Set（解耦，便于单测注入内存实现）。
type KVStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Load 读配置；缺失 / 解析失败回退默认（绝不因脏配置阻断整理）。
func Load(kv KVStore) Config {
	def := DefaultConfig()
	if kv == nil {
		return def
	}
	raw, err := kv.Get(configKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	c := def
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return def
	}
	return c.withDefaults()
}

// Save 写配置。
func Save(kv KVStore, c Config) error {
	if kv == nil {
		return nil
	}
	b, err := json.Marshal(c.withDefaults())
	if err != nil {
		return err
	}
	return kv.Set(configKey, string(b))
}

func (c Config) withDefaults() Config {
	out := c
	if len(out.Languages) == 0 {
		out.Languages = DefaultConfig().Languages
	}
	if out.SearchTimeout <= 0 {
		out.SearchTimeout = DefaultConfig().SearchTimeout
	}
	if out.Languages != nil {
		normalized := make([]string, 0, len(out.Languages))
		for _, l := range out.Languages {
			if cl := canonicalLang(l); cl != "" {
				normalized = append(normalized, cl)
			}
		}
		out.Languages = normalized
	}
	return out
}

// canonicalLang 把用户填的 zh/en/简体/繁体 等别名归一为 subtitle 包的 langcode（chs/cht/eng/bi）。
// subtitle.SubtitleChain.Search 用该 code 精确匹配 LangCode，故须归一。
func canonicalLang(lang string) string {
	r := strings.ToLower(strings.TrimSpace(lang))
	switch r {
	case "":
		return ""
	case "zh", "zh-cn", "zh_cn", "cn", "chi", "chinese", "简", "简体", "简体中文":
		return "chs"
	case "zh-tw", "zh_tw", "tw", "繁", "繁体", "繁体中文":
		return "cht"
	case "en", "eng", "english", "英", "英文", "英语":
		return "eng"
	case "bi", "双语", "中英", "英中":
		return "bi"
	}
	return r
}

// Searcher 字幕搜索 / 下载能力（*chain.SubtitleChain 结构化实现该接口，无需适配）。
type Searcher interface {
	Search(ctx context.Context, query, language string) ([]subtitle.Subtitle, error)
	Download(ctx context.Context, sub subtitle.Subtitle) ([]byte, string, error)
}

// Notifier 通知中心（*notice.Service 实现该接口；nil 跳过通知）。
type Notifier interface {
	Notice(ctx context.Context, in notice.Input, dedup bool) error
}

// Service 字幕自动化服务：订阅 transfer_done → 自动搜字幕落盘。
type Service struct {
	bus    *event.Bus
	search Searcher
	kv     KVStore
	notice Notifier
	handle int // 事件订阅 handle（便于扩展退订）
}

// New 构造。bus / search / kv 建议非 nil；notice 可 nil（仅不推送通知）。
func New(bus *event.Bus, search Searcher, kv KVStore, notice Notifier) *Service {
	return &Service{bus: bus, search: search, kv: kv, notice: notice}
}

// Config 当前生效配置（handler 回显用）。
func (s *Service) Config() Config { return Load(s.kv) }

// SetConfig 保存配置（即时生效：每次 process 现取）。
func (s *Service) SetConfig(c Config) error { return Save(s.kv, c) }

// Start 订阅 transfer_done 事件。应只调一次（重复调用会叠加订阅）。
func (s *Service) Start() {
	if s.bus == nil {
		return
	}
	s.handle = s.bus.Subscribe(EventTransferDone, 100, s.handleTransferDone)
}

// MediaInfo 从 transfer_done 事件负载提取的媒体信息（仅含自动化所需字段）。
type MediaInfo struct {
	Title string
	Year  int
	Type  string // movie | tv
	Files []string
}

// handleTransferDone 事件入口：同步提取负载 → 异步处理（字幕搜索/下载耗时数秒~数十秒，
// 不能阻塞事件总线 Publish 的 wg.Wait，否则拖慢 transfer.record）。
func (s *Service) handleTransferDone(ctx context.Context, e event.Event) {
	m, ok := extractMedia(e.Data)
	if !ok {
		return
	}
	// 用新 ctx：事件 ctx 可能随请求结束被取消，而字幕下载是后台副作用任务。
	go s.process(m)
}

// extractMedia 从事件 data 取媒体信息；无视频文件返回 ok=false。
func extractMedia(data map[string]any) (MediaInfo, bool) {
	if data == nil {
		return MediaInfo{}, false
	}
	var m MediaInfo
	if v, ok := data["title"].(string); ok {
		m.Title = v
	}
	switch v := data["year"].(type) {
	case int:
		m.Year = v
	case int64:
		m.Year = int(v)
	case float64:
		m.Year = int(v)
	}
	if v, ok := data["type"].(string); ok {
		m.Type = v
	}
	switch v := data["files"].(type) {
	case []string:
		m.Files = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				m.Files = append(m.Files, s)
			}
		}
	}
	if len(m.Files) == 0 {
		return MediaInfo{}, false
	}
	return m, true
}

// fetchResult 单文件处理结果。
type fetchResult int

const (
	resultNone fetchResult = iota
	resultDownloaded
	resultSkipped
	resultFailed
)

// process 对每个视频文件搜字幕并落盘。单文件失败不影响其他；含 recover 兜底。
func (s *Service) process(m MediaInfo) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("subtitle automation panic", "title", m.Title, "panic", r)
		}
	}()
	cfg := Load(s.kv)
	if !cfg.Enabled {
		return
	}
	timeout := time.Duration(cfg.SearchTimeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	var downloaded, skipped, failed int
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, video := range m.Files {
		if !isVideoFile(video) {
			continue
		}
		switch s.fetchForFile(ctx, cfg, video, m) {
		case resultDownloaded:
			downloaded++
		case resultSkipped:
			skipped++
		case resultFailed:
			failed++
		}
	}
	if s.notice != nil && (downloaded > 0 || failed > 0) {
		level := "success"
		if downloaded == 0 {
			level = "warning"
		}
		body := fmt.Sprintf("下载 %d / 跳过 %d / 失败 %d", downloaded, skipped, failed)
		_ = s.notice.Notice(context.Background(), notice.Input{
			Source: notice.SourceTransferDone,
			Key:    "subtitle_auto:" + m.Title + ":" + firstFile(m.Files),
			Title:  "字幕自动下载：" + m.Title,
			Body:   body,
			Level:  level,
		}, false)
	}
}

// fetchForFile 为单个视频文件搜一条字幕并落盘。
func (s *Service) fetchForFile(ctx context.Context, cfg Config, video string, m MediaInfo) fetchResult {
	dir := filepath.Dir(video)
	base := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))

	// 1. 幂等 / 不覆盖：视频同目录已有任意同 basename 字幕 → 跳过。
	existing := existingSubtitles(dir, base)
	if len(existing) > 0 && cfg.OnlyIfMissing {
		return resultSkipped
	}

	// 2. 按语言优先级搜索（首个命中即下载）。
	query := buildQuery(m, video)
	for _, lang := range cfg.Languages {
		subs, err := s.search.Search(ctx, query, lang)
		if err != nil {
			slog.Debug("subtitle automation search error", "query", query, "lang", lang, "err", err)
			continue
		}
		picked := pickSubtitle(subs, cfg)
		if picked == nil {
			continue
		}
		data, name, err := s.search.Download(ctx, *picked)
		if err != nil || len(data) == 0 {
			slog.Warn("subtitle automation download failed", "source", picked.Source, "source_id", picked.SourceID, "err", err)
			continue
		}
		subExt := subtitleExt(name, picked.Ext)
		target := subtitlePath(dir, base, picked.LangCode, subExt)
		if fileExists(target) {
			return resultSkipped // 同语言同格式已存在（幂等）
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Warn("subtitle automation mkdir failed", "dir", dir, "err", err)
			return resultFailed
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			slog.Warn("subtitle automation write failed", "path", target, "err", err)
			return resultFailed
		}
		slog.Info("subtitle automation downloaded", "video", video, "lang", picked.LangCode, "source", picked.Source, "path", target)
		return resultDownloaded
	}
	return resultFailed
}

// pickSubtitle 按 provider 优先级选字幕（结果已按 Score 降序，故无 provider 偏好时取首个）。
func pickSubtitle(subs []subtitle.Subtitle, cfg Config) *subtitle.Subtitle {
	if len(subs) == 0 {
		return nil
	}
	if len(cfg.Providers) > 0 {
		for _, want := range cfg.Providers {
			for i := range subs {
				if strings.EqualFold(subs[i].Source, want) {
					return &subs[i]
				}
			}
		}
	}
	return &subs[0]
}

// buildQuery 构造搜索关键字：电影用标题；剧集用「标题 S01E02」（SxxExx 从文件名解析）。
func buildQuery(m MediaInfo, video string) string {
	if m.Type == "tv" || m.Type == "" {
		if se := seasonEpisodeRe.FindString(filepath.Base(video)); se != "" {
			if m.Title != "" {
				return m.Title + " " + se
			}
			return se
		}
	}
	if m.Title != "" {
		return m.Title
	}
	return strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
}

