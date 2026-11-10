// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import "testing"

// TestPromoTypeOf：订阅促销类型多选过滤依赖 PromoTypeOf 归一化分类。
// 关键路径：因子 0 的"未设"约定（dvf=0 且非 Free→1；uvf=0→1），以及 dvf/uvf 组合的 5 类划分。
func TestPromoTypeOf(t *testing.T) {
	cases := []struct {
		name string
		t    Torrent
		want PromoType
	}{
		{"免费(Free标记)", Torrent{Free: true}, PromoFree},
		{"免费(因子0+Free)", Torrent{DownloadVolumeFactor: 0, Free: true}, PromoFree},
		{"2X免费", Torrent{DownloadVolumeFactor: 0, UploadVolumeFactor: 2, Free: true}, Promo2xFree},
		{"双倍上传", Torrent{DownloadVolumeFactor: 1, UploadVolumeFactor: 2}, Promo2x},
		{"折扣下载50%", Torrent{DownloadVolumeFactor: 0.5}, PromoDiscount},
		{"折扣下载30%", Torrent{DownloadVolumeFactor: 0.7}, PromoDiscount},
		{"2X+折扣", Torrent{DownloadVolumeFactor: 0.5, UploadVolumeFactor: 2}, Promo2xDiscount},
		{"无促销", Torrent{}, PromoNone},
		{"因子0且非Free(视为未设→正常)", Torrent{DownloadVolumeFactor: 0, Free: false}, PromoNone},
	}
	for _, c := range cases {
		if got := PromoTypeOf(c.t); got != c.want {
			t.Errorf("%s: PromoTypeOf=%v want %v (dvf=%v uvf=%v free=%v)",
				c.name, got, c.want, c.t.DownloadVolumeFactor, c.t.UploadVolumeFactor, c.t.Free)
		}
	}
}

// TestNexusPromotionClassFactor：NexusPHP pro_* class → 因子映射（80+ 站框架标准）。
// 钉死 pro_2up（纯双倍上传）能被识别——此前 img[class^="pro_free"] 完全忽略它。
func TestNexusPromotionClassFactor(t *testing.T) {
	cases := []struct {
		cls     string
		wantDvf float64
		wantUvf float64
		wantOk  bool
	}{
		{"pro_free", 0, 1, true},
		{"pro_free_forever", 0, 1, true},
		{"pro_free2up", 0, 2, true},
		{"pro_2up", 1, 2, true},
		{"pro_50pctdown", 0.5, 1, true},
		{"pro_halfdown", 0.5, 1, true},
		{"pro_2up50pctdown", 0.5, 2, true},
		{"pro_30pctdown", 0.7, 1, true},
		{"pro_unknown", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, c := range cases {
		dvf, uvf, ok := nexusPromotionClassFactor(c.cls)
		if dvf != c.wantDvf || uvf != c.wantUvf || ok != c.wantOk {
			t.Errorf("cls=%q: got (%v,%v,%v) want (%v,%v,%v)",
				c.cls, dvf, uvf, ok, c.wantDvf, c.wantUvf, c.wantOk)
		}
	}
}
