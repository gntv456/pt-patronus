// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/plugins/plexsuite"
)

// plex-suite：Plex 媒体服务器套件增强（本地化/跳片头/自动语言/IP 优选）。
// 凭据由插件配置承载（server_url + X-Plex-Token），不耦合 mediaserver 表，便于多 Plex 实例。
//

type plexSuitePlugin struct {
	hc *httpclient.Client
}

// NewPlexSuitePlugin 构造。
func NewPlexSuitePlugin() Plugin { return &plexSuitePlugin{} }

func (p *plexSuitePlugin) client() (*httpclient.Client, error) {
	if p.hc != nil {
		return p.hc, nil
	}
	// Plex 常部署在内网/环回，必须放行私网地址。
	c, err := httpclient.New(httpclient.Options{Timeout: 20 * time.Second, AllowPrivate: true})
	if err != nil {
		return nil, err
	}
	p.hc = c
	return c, nil
}

func (p *plexSuitePlugin) Manifest() Manifest {
	return Manifest{
		ID:          "ptp-plex-suite",
		Name:        "Plex 套件",
		Version:     "1.0.0",
		APIVersion:  "1",
		AppVersion:  ">=0.1.0",
		Author:      "PTPatronus Community",
		Description: "【Plex 增强】定时执行 Plex 媒体服务器套件：库本地化语言、触发片头分析（跳片头）、自动音频/字幕语言、多候选 IP 优选。凭据在下方配置（server_url + X-Plex-Token）。部分能力需 Plex Pass。",
		License:     "MIT",
		Keywords:    []string{"plex", "localization", "skip-intro", "language", "ip-optimize"},
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "log:write", "notice:write"},
		Actions: []ActionSpec{
			{Name: "apply", Label: "立即应用全部", Description: "对所有已启用特性执行一次（本地化/片头/语言/IP优选）。"},
			{Name: "ip_optimize", Label: "IP 优选探测", Description: "探测候选地址返回延迟，可选最快者。"},
			{Name: "test", Label: "测试连接", Description: "用当前 server_url+token 测试 Plex 可达性。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "server_url", Label: "Plex 地址", Type: "text", Group: "服务器", Help: "如 http://127.0.0.1:32400，IP 优选后会自动更新为最快地址。"},
			{Key: "token", Label: "X-Plex-Token", Type: "text", Group: "服务器", Required: true, Help: "Plex 鉴权令牌。"},
			{Key: "candidate_urls", Label: "候选地址（IP 优选）", Type: "textarea", Group: "IP 优选", Help: "每行一个候选 URL，逐个探测取最快写回上方地址。"},
			{Key: "enable_ip_optimize", Label: "启用 IP 优选", Type: "switch", Group: "IP 优选", Default: false},
			{Key: "enable_localization", Label: "启用本地化", Type: "switch", Group: "本地化", Default: false},
			{Key: "library_language", Label: "库语言", Type: "text", Group: "本地化", Help: "如 zh/en，用于设置 Plex 语言偏好。"},
			{Key: "enable_skip_intro", Label: "触发片头分析", Type: "switch", Group: "跳片头", Default: false, Help: "对各库触发片头分析（生成跳片头标记需 Plex Pass）。"},
			{Key: "enable_auto_language", Label: "启用自动语言", Type: "switch", Group: "自动语言", Default: false},
			{Key: "audio_language", Label: "音频语言", Type: "text", Group: "自动语言", Help: "如 zh/jpn/en。"},
			{Key: "subtitle_language", Label: "字幕语言", Type: "text", Group: "自动语言", Help: "如 zh/en，留空则不改字幕。"},
			{Key: "notify", Label: "执行结果通知", Type: "switch", Group: "通知", Default: true},
			{Key: "cron", Label: "定时表达式", Type: "text", Group: "调度", Default: "0 4 * * *", Help: "默认每天凌晨 4 点应用一次。留空则不自动。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "apply", Label: "每日应用 Plex 套件", Action: "apply", Cron: "0 4 * * *", CronConfigKey: "cron"},
		},
		Contributes: &ContributionSpec{
			Commands: []CommandContribution{{ID: "ptp-plex-suite.apply", Label: "Plex 套件：立即应用", Action: "apply"}},
			Menus:    []MenuContribution{{Location: "plugins/actions", Command: "ptp-plex-suite.apply"}},
		},
		ResourceAccess: &ResourceAccessSpec{
			NetHosts: []string{"用户配置的 Plex 服务器地址（通常为内网或本机）"},
		},
	}
}

func (p *plexSuitePlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"server_url": "", "token": "", "candidate_urls": "",
		"enable_ip_optimize": false, "enable_localization": false, "library_language": "",
		"enable_skip_intro": false, "enable_auto_language": false,
		"audio_language": "", "subtitle_language": "",
		"notify": true, "cron": "0 4 * * *",
	}
}

func (p *plexSuitePlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (p *plexSuitePlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	hc, err := p.client()
	if err != nil {
		return nil, err
	}
	engine := plexsuite.NewEngine(hc)
	s := readPlexSettings(rt.Config())

	switch action {
	case "test":
		// 用一个轻量库枚举校验可达 + 鉴权。
		probes := engine.ProbeAll(ctx, plexsuite.Settings{ServerURL: s.ServerURL, Token: s.Token, CandidateURLs: []string{s.ServerURL}})
		if len(probes) == 0 || !probes[0].OK {
			return nil, fmt.Errorf("Plex 不可达或鉴权失败：%s", s.ServerURL)
		}
		return map[string]any{"ok": true, "server_url": s.ServerURL}, nil
	case "ip_optimize":
		probes := engine.ProbeAll(ctx, s)
		return map[string]any{"ok": true, "probes": probes}, nil
	case "apply", "run_now":
		results := engine.Apply(ctx, s)
		// 若 IP 优选改写了 server_url，写回 config。
		if s2, changed := updatedServerURL(rt.Config(), s.ServerURL); changed {
			_ = rt.SetRuntimeConfig("server_url", s2)
		}
		if notify, _ := rt.Config()["notify"].(bool); notify {
			p.notifyApply(ctx, rt, results)
		}
		return map[string]any{"ok": true, "results": results, "server_url": s.ServerURL}, nil
	default:
		return nil, unknownActionErr(action)
	}
}

// notifyApply 汇总推送一次套件执行结果。
func (p *plexSuitePlugin) notifyApply(ctx context.Context, rt Runtime, results []plexsuite.FeatureResult) {
	ok, fail := 0, 0
	var sb strings.Builder
	for _, r := range results {
		if r.OK {
			ok++
		} else {
			fail++
			sb.WriteString(fmt.Sprintf("• %s：%s\n", r.Feature, r.Message))
		}
	}
	level := "success"
	title := fmt.Sprintf("Plex 套件应用完成：%d 成功 %d 失败", ok, fail)
	if fail > 0 {
		level = "warning"
	}
	body := strings.TrimSpace(sb.String())
	if body == "" {
		body = fmt.Sprintf("全部 %d 项特性执行成功。", ok)
	}
	_ = rt.Notice(ctx, NoticeInput{Title: title, Body: body, Level: level})
}

// readPlexSettings 从插件 config map 解析 plexsuite.Settings。
func readPlexSettings(cfg map[string]any) plexsuite.Settings {
	return plexsuite.Settings{
		ServerURL:          strings.TrimSpace(pluginString(cfg["server_url"])),
		Token:              strings.TrimSpace(pluginString(cfg["token"])),
		CandidateURLs:      splitNonEmptyLines(pluginString(cfg["candidate_urls"])),
		EnableIPOptimize:   boolVal(cfg["enable_ip_optimize"]),
		EnableLocalization: boolVal(cfg["enable_localization"]),
		LibraryLanguage:    strings.TrimSpace(pluginString(cfg["library_language"])),
		EnableSkipIntro:    boolVal(cfg["enable_skip_intro"]),
		EnableAutoLanguage: boolVal(cfg["enable_auto_language"]),
		AudioLanguage:      strings.TrimSpace(pluginString(cfg["audio_language"])),
		SubtitleLanguage:   strings.TrimSpace(pluginString(cfg["subtitle_language"])),
	}
}

// updatedServerURL 检测 IP 优选是否改写了地址，返回（新地址, 是否变更）。
func updatedServerURL(cfg map[string]any, newURL string) (string, bool) {
	old := strings.TrimSpace(pluginString(cfg["server_url"]))
	newURL = strings.TrimSpace(newURL)
	return newURL, newURL != "" && newURL != old
}

func splitNonEmptyLines(s string) []string {
	out := make([]string, 0, 4)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func boolVal(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
