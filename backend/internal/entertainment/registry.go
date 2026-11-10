// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"strings"
)

// Registry 按站点 host 注册游戏。
type Registry struct {
	games map[string]map[string]LotteryGame // host -> gameID -> game
}

// NewRegistry 构造。
func NewRegistry() *Registry {
	return &Registry{games: map[string]map[string]LotteryGame{}}
}

// Register 为某 host 注册一个游戏。
func (r *Registry) Register(host string, g LotteryGame) {
	h := normalizeHost(host)
	if r.games[h] == nil {
		r.games[h] = map[string]LotteryGame{}
	}
	r.games[h][g.ID()] = g
}

// Games 返回某 host 的全部游戏。
// host 匹配放宽：站点 base_url 常带 www（www.hxpt.org）而注册用裸域（hxpt.org），
// 按后缀匹配视为同一站点，避免「娱乐页空白」。
func (r *Registry) Games(host string) []LotteryGame {
	site := normalizeHost(host)
	out := make([]LotteryGame, 0)
	for regHost, games := range r.games {
		if hostMatch(site, regHost) {
			for _, g := range games {
				out = append(out, g)
			}
		}
	}
	return out
}

// Game 取某 host 的指定游戏（同样按后缀匹配）。
func (r *Registry) Game(host, gameID string) (LotteryGame, bool) {
	site := normalizeHost(host)
	for regHost, games := range r.games {
		if hostMatch(site, regHost) {
			if g, ok := games[gameID]; ok {
				return g, true
			}
		}
	}
	return nil, false
}

// hostMatch 报告 siteHost 是否对应 regHost：精确相等，或 siteHost 是 regHost 的子域
// （如 www.hxpt.org ↔ hxpt.org）。
func hostMatch(siteHost, regHost string) bool {
	if siteHost == regHost {
		return true
	}
	return strings.HasSuffix(siteHost, "."+regHost)
}

// Hosts 返回已注册的全部 host。
func (r *Registry) Hosts() []string {
	out := make([]string, 0, len(r.games))
	for h := range r.games {
		out = append(out, h)
	}
	return out
}

// normalizeHost：去 scheme/path，小写，去端口。
func normalizeHost(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		s = s[:i] // 去端口
	}
	return strings.ToLower(s)
}
