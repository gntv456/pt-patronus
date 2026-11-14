// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"testing"
)

// 改版（2026-11）列表条目促销解析：promotion{id:2-7} 数值档，兼容旧 label 文本。

func TestParsePiggoTorrentPromotionID(t *testing.T) {
	cases := []struct {
		name     string
		promo    map[string]any
		wantFree bool
		wantDown float64
		wantUp   float64
	}{
		{"无促销", nil, false, 1, 1},
		{"普通档1", map[string]any{"id": float64(1), "label": "普通"}, false, 1, 1},
		{"免费档2", map[string]any{"id": float64(2), "label": "免费"}, true, 0, 1},
		{"2x上传档3", map[string]any{"id": float64(3), "label": "2x 上传"}, false, 1, 2},
		{"2x免费档4", map[string]any{"id": float64(4), "label": "2x 免费"}, true, 0, 2},
		{"50%档5", map[string]any{"id": float64(5), "label": "50% 下载"}, false, 0.5, 1},
		{"2x50%档6", map[string]any{"id": float64(6), "label": "2x 50%"}, false, 0.5, 2},
		{"30%档7", map[string]any{"id": float64(7), "label": "30% 下载"}, false, 0.3, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it := map[string]any{"id": float64(123), "name": "标题", "size": float64(1 << 30)}
			if tc.promo != nil {
				it["promotion"] = tc.promo
			}
			got := parsePiggoTorrent(it, "https://piggo.me")
			if got.Free != tc.wantFree {
				t.Fatalf("Free = %v, want %v", got.Free, tc.wantFree)
			}
			if got.DownloadVolumeFactor != tc.wantDown {
				t.Fatalf("down = %v, want %v", got.DownloadVolumeFactor, tc.wantDown)
			}
			if got.UploadVolumeFactor != tc.wantUp {
				t.Fatalf("up = %v, want %v", got.UploadVolumeFactor, tc.wantUp)
			}
			if got.DetailURL != "https://piggo.me/detail/123" {
				t.Fatalf("detail = %s", got.DetailURL)
			}
		})
	}
}

// 旧形态兼容：promotion.label 文本含「免费」仍判 FREE（旧字段 up 因子保留）。
func TestParsePiggoTorrentLegacyLabel(t *testing.T) {
	it := map[string]any{
		"id":   float64(9),
		"name": "旧条目",
		"promotion": map[string]any{
			"label": "免费",
			"up":    float64(2),
		},
	}
	got := parsePiggoTorrent(it, "https://piggo.me")
	if !got.Free || got.DownloadVolumeFactor != 0 {
		t.Fatalf("legacy free: free=%v down=%v", got.Free, got.DownloadVolumeFactor)
	}
	if got.UploadVolumeFactor != 2 {
		t.Fatalf("legacy up = %v, want 2", got.UploadVolumeFactor)
	}
}

// 全局促销兜底：单种无促销时 global_promotion 免费文案 → FREE。
func TestParsePiggoTorrentGlobalPromotion(t *testing.T) {
	it := map[string]any{
		"id":   float64(10),
		"name": "全局促销条目",
		"global_promotion": map[string]any{
			"phase": "active",
			"label": "全站免费",
		},
	}
	got := parsePiggoTorrent(it, "https://piggo.me")
	if !got.Free || got.DownloadVolumeFactor != 0 {
		t.Fatalf("global free: free=%v down=%v", got.Free, got.DownloadVolumeFactor)
	}
}

// pgPromotionID：缺 promotion / 非 map / id 缺失 → 0。
func TestPgPromotionID(t *testing.T) {
	if got := pgPromotionID(map[string]any{}); got != 0 {
		t.Fatalf("empty item: %d", got)
	}
	if got := pgPromotionID(map[string]any{"promotion": "bad"}); got != 0 {
		t.Fatalf("non-map promotion: %d", got)
	}
	if got := pgPromotionID(map[string]any{"promotion": map[string]any{"label": "x"}}); got != 0 {
		t.Fatalf("no id: %d", got)
	}
}
