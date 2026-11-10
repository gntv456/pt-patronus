// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

func boolPtr(b bool) *bool { return &b }

// filterByOfficial 仅保留 site.IsOfficial 命中的种子（标题/标签含「官方/官种/官组」）。
func TestFilterByOfficial(t *testing.T) {
	cases := []struct {
		name  string
		title string
		tags  []string
		keep  bool
	}{
		{"title 含官方", "某电影 2025 官方版本 2160p", nil, true},
		{"title 含官种", "某剧 第一季 1080p 官种", nil, true},
		{"labels 含官组命中（title 不含）", "某电影 2025 1080p BluRay", []string{"官组"}, true},
		{"非官种过滤", "某电影 2025 1080p BluRay-PTer", nil, false},
		{"空 labels 回退 title 未命中", "某电影 2025 1080p", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := []site.Torrent{{Title: tc.title, Labels: tc.tags}}
			got := filterByOfficial(ts)
			kept := len(got) == 1
			if kept != tc.keep {
				t.Errorf("filterByOfficial: kept=%v want %v", kept, tc.keep)
			}
		})
	}
}

// resolveEffective：订阅 OfficialOnly 非 nil 用订阅值，nil 回退全局默认（复刻 EnableRatingGuard）。
func TestResolveEffectiveOfficialOnly(t *testing.T) {
	cases := []struct {
		name string
		sub  *bool
		gd   bool
		want bool
	}{
		{"nil 回退全局开", nil, true, true},
		{"nil 回退全局关", nil, false, false},
		{"订阅开覆盖全局关", boolPtr(true), false, true},
		{"订阅关覆盖全局开", boolPtr(false), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := &models.Subscription{OfficialOnly: tc.sub}
			gd := GlobalDefaults{OfficialOnly: tc.gd}
			if eff := resolveEffective(sub, gd); eff.OfficialOnly != tc.want {
				t.Errorf("eff.OfficialOnly = %v, want %v", eff.OfficialOnly, tc.want)
			}
		})
	}
}
