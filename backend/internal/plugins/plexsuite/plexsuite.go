// Package plexsuite 实现 Plex 媒体服务器套件增强：本地化、跳片头、自动语言、IP 优选。
// 自包含（对标 forumsignin/groupchat 子包模式），避免 plugins 包直接持有 Plex 业务逻辑。
// 端点字段基于 Plex Media Server 公开 REST 契约，部分能力需 Plex Pass，真实联调留用户。
package plexsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// Settings 是 plex-suite 插件的配置视图（由插件 config map 解析）。
type Settings struct {
	ServerURL string `json:"server_url"` // 如 http://127.0.0.1:32400
	Token     string `json:"token"`      // X-Plex-Token
	// IP 优选：候选地址（每行一个 URL），插件逐个探测取最快，写回 ServerURL。
	CandidateURLs []string `json:"candidate_urls"`
	// 自动语言：默认音频/字幕语言（Plex 语言代码如 zh, en, jpn）+ 是否开启自动选择。
	AudioLanguage    string `json:"audio_language"`
	SubtitleLanguage string `json:"subtitle_language"`
	// 各特性开关
	EnableLocalization bool   `json:"enable_localization"`  // 库本地化（语言/国家）
	EnableSkipIntro    bool   `json:"enable_skip_intro"`    // 触发片头分析
	EnableAutoLanguage bool   `json:"enable_auto_language"` // 设置自动语言偏好
	EnableIPOptimize   bool   `json:"enable_ip_optimize"`   // IP 优选探测
	LibraryLanguage    string `json:"library_language"`     // 本地化：库语言（zh/en…）
}

// FeatureResult 是单个特性一次执行的产出。
type FeatureResult struct {
	Feature string `json:"feature"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// ProbeResult 是 IP 优选的一次候选探测。
type ProbeResult struct {
	URL     string `json:"url"`
	OK      bool   `json:"ok"`
	Latency int    `json:"latency_ms"`
	Error   string `json:"error,omitempty"`
}

// Engine 编排 Plex 套件特性。hc 复用宿主 httpclient（Plex 多为内网，AllowPrivate=true）。
type Engine struct {
	hc *httpclient.Client
}

// NewEngine 构造。hc 必须允许内网（Plex 常部署在 127.0.0.1/LAN）。
func NewEngine(hc *httpclient.Client) *Engine { return &Engine{hc: hc} }

// headers 构造 Plex 鉴权头。
func headers(token string) map[string]string {
	return map[string]string{
		"X-Plex-Token": token,
		"Accept":       "application/json",
	}
}

// Apply 执行所有已启用特性，返回每项结果。单特性失败不阻断其它。
func (e *Engine) Apply(ctx context.Context, s Settings) []FeatureResult {
	results := make([]FeatureResult, 0, 4)
	if s.ServerURL == "" || s.Token == "" {
		return []FeatureResult{{Feature: "config", OK: false, Message: "缺少 server_url 或 token"}}
	}
	if s.EnableIPOptimize {
		results = append(results, e.ipOptimize(ctx, &s))
	}
	if s.EnableLocalization {
		results = append(results, e.applyLocalization(ctx, s))
	}
	if s.EnableSkipIntro {
		results = append(results, e.applySkipIntro(ctx, s))
	}
	if s.EnableAutoLanguage {
		results = append(results, e.applyAutoLanguage(ctx, s))
	}
	if len(results) == 0 {
		results = append(results, FeatureResult{Feature: "noop", OK: true, Message: "未启用任何特性"})
	}
	return results
}

// applyLocalization 设置服务端语言偏好（Plex /:/prefs?SettingKey=Value）。
// TODO 联调：Plex 服务端 pref 键名（LanguageIndicator/Language 等）随版本变化，真实键名待校准。
func (e *Engine) applyLocalization(ctx context.Context, s Settings) FeatureResult {
	if s.LibraryLanguage == "" {
		return FeatureResult{Feature: "localization", OK: false, Message: "未配置 library_language"}
	}
	// Plex 设置偏好的写法：PUT /:/prefs?<key>=<value>（GET 读取当前值校验可达）。
	u := fmt.Sprintf("%s/:/prefs?Language=%s", strings.TrimRight(s.ServerURL, "/"), url.QueryEscape(s.LibraryLanguage))
	if err := e.put(ctx, u, s.Token); err != nil {
		return FeatureResult{Feature: "localization", OK: false, Message: "设置语言偏好失败：" + err.Error()}
	}
	return FeatureResult{Feature: "localization", OK: true, Message: "已设置库语言：" + s.LibraryLanguage}
}

// applySkipIntro 触发各库片头分析（GET /library/sections/<id>/analyze）。
// 片头标记本身由 Plex Pass 服务端生成，此处仅触发/重跑分析。
// TODO 联调：需 Plex Pass；分析为异步任务，返回仅表示已提交。
func (e *Engine) applySkipIntro(ctx context.Context, s Settings) FeatureResult {
	libs, err := e.libraries(ctx, s)
	if err != nil {
		return FeatureResult{Feature: "skip_intro", OK: false, Message: "枚举库失败：" + err.Error()}
	}
	triggered := 0
	for _, lib := range libs {
		u := fmt.Sprintf("%s/library/sections/%s/analyze", strings.TrimRight(s.ServerURL, "/"), esc(lib.ID))
		if err := e.get(ctx, u, s.Token, nil); err == nil {
			triggered++
		}
	}
	return FeatureResult{
		Feature: "skip_intro", OK: triggered > 0,
		Message: fmt.Sprintf("已对 %d/%d 个库触发片头分析", triggered, len(libs)),
	}
}

// applyAutoLanguage 设置用户自动音频/字幕语言偏好（Plex /:/prefs 自动语言相关键）。
// TODO 螁调：Plex 自动语言 pref 键名（AutoSelectAudioStream/AudioStreamLanguage 等）待真实校准。
func (e *Engine) applyAutoLanguage(ctx context.Context, s Settings) FeatureResult {
	if s.AudioLanguage == "" && s.SubtitleLanguage == "" {
		return FeatureResult{Feature: "auto_language", OK: false, Message: "未配置音频/字幕语言"}
	}
	parts := make([]string, 0, 2)
	if s.AudioLanguage != "" {
		parts = append(parts, "AudioStreamLanguage="+url.QueryEscape(s.AudioLanguage))
	}
	if s.SubtitleLanguage != "" {
		parts = append(parts, "SubtitleStreamLanguage="+url.QueryEscape(s.SubtitleLanguage))
	}
	u := fmt.Sprintf("%s/:/prefs?%s", strings.TrimRight(s.ServerURL, "/"), strings.Join(parts, "&"))
	if err := e.put(ctx, u, s.Token); err != nil {
		return FeatureResult{Feature: "auto_language", OK: false, Message: "设置自动语言失败：" + err.Error()}
	}
	return FeatureResult{Feature: "auto_language", OK: true, Message: "已设置音频/字幕语言偏好"}
}

// ipOptimize 逐个探测候选地址，取最快可达者写回 Settings.ServerURL（内存；持久化由插件写回 config）。
func (e *Engine) ipOptimize(ctx context.Context, s *Settings) FeatureResult {
	if len(s.CandidateURLs) == 0 {
		return FeatureResult{Feature: "ip_optimize", OK: false, Message: "未配置候选地址"}
	}
	probes := e.probeAll(ctx, *s)
	best := pickFastest(probes)
	r := FeatureResult{Feature: "ip_optimize"}
	if best != nil && best.OK {
		s.ServerURL = best.URL
		r.OK = true
		r.Message = fmt.Sprintf("优选到 %s（%dms）", best.URL, best.Latency)
		r.Detail = serializeProbes(probes)
	} else {
		r.OK = false
		r.Message = "所有候选地址均不可达"
		r.Detail = serializeProbes(probes)
	}
	return r
}

// ProbeAll 逐个探测候选（供 action 单独调用）。
func (e *Engine) ProbeAll(ctx context.Context, s Settings) []ProbeResult { return e.probeAll(ctx, s) }

func (e *Engine) probeAll(ctx context.Context, s Settings) []ProbeResult {
	var mu sync.Mutex
	results := make([]ProbeResult, 0, len(s.CandidateURLs))
	var wg sync.WaitGroup
	for _, cand := range s.CandidateURLs {
		cand = strings.TrimSpace(cand)
		if cand == "" {
			continue
		}
		wg.Add(1)
		safego.Go("plexsuite:182", func() {
			u := cand
			defer wg.Done()
			pr := ProbeResult{URL: u}
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			start := time.Now()
			if err := e.get(cctx, strings.TrimRight(u, "/")+"/", s.Token, nil); err != nil {
				pr.Error = err.Error()
			} else {
				pr.OK = true
				pr.Latency = int(time.Since(start) / time.Millisecond)
			}
			mu.Lock()
			results = append(results, pr)
			mu.Unlock()
		})
	}
	wg.Wait()
	// 稳定排序：可达优先，按延迟升序
	sort.SliceStable(results, func(i, j int) bool {
		ri, rj := results[i], results[j]
		if ri.OK != rj.OK {
			return ri.OK
		}
		if ri.Latency != rj.Latency {
			return ri.Latency < rj.Latency
		}
		return ri.URL < rj.URL
	})
	return results
}

// pickFastest 返回可达候选中最快者；无可达返回 nil。
func pickFastest(probes []ProbeResult) *ProbeResult {
	for i := range probes {
		if probes[i].OK {
			return &probes[i]
		}
	}
	return nil
}

func serializeProbes(probes []ProbeResult) string {
	out := make([]string, 0, len(probes))
	for _, p := range probes {
		if p.OK {
			out = append(out, fmt.Sprintf("%s ✓ %dms", p.URL, p.Latency))
		} else {
			out = append(out, fmt.Sprintf("%s ✗ %s", p.URL, p.Error))
		}
	}
	return strings.Join(out, "\n")
}

// Library 是 Plex 库的统一视图。
type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func (e *Engine) libraries(ctx context.Context, s Settings) ([]Library, error) {
	var res struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	u := strings.TrimRight(s.ServerURL, "/") + "/library/sections"
	if err := e.get(ctx, u, s.Token, &res); err != nil {
		return nil, err
	}
	out := make([]Library, 0, len(res.MediaContainer.Directory))
	for _, d := range res.MediaContainer.Directory {
		out = append(out, Library{ID: sectionID(d.Key), Name: d.Title, Type: d.Type})
	}
	return out, nil
}

// ===== 低层 HTTP =====

func (e *Engine) get(ctx context.Context, u, token string, out any) error {
	// X-Plex-Token 必须随请求带（header）；Accept: application/json 让 Plex 返 JSON。
	resp, err := e.hc.Do(ctx, "GET", u, nil, headers(token), "", false) // Plex 常内网，不走 SSRF
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("plex %d: %s", resp.StatusCode, snippet(resp.Body))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(resp.Body, out)
}

// put 用 PUT 请求 Plex 端点（Plex 设置偏好用 PUT）。httpclient.Do 通用方法足以。
func (e *Engine) put(ctx context.Context, u, token string) error {
	resp, err := e.hc.Do(ctx, "PUT", u, nil, headers(token), "", false)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("plex %d: %s", resp.StatusCode, snippet(resp.Body))
	}
	return nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// sectionID 从 Plex Key（"/library/sections/2"）取末段作库 id。
func sectionID(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			return key[i+1:]
		}
	}
	return key
}

// esc 对路径段做 URL 转义（仅对纯数字 id 通常无副作用，统一处理）。
func esc(s string) string { return strconv.QuoteToASCII(s) }
