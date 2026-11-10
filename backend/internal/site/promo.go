// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PromoType 促销类型（对齐 NexusPHP spstate 标准 + Rousi 分类）。空串=无促销/正常。
// 订阅「促销类型多选」按此枚举过滤：种子被归入某类型 ∈ 订阅所选集合才推送。
type PromoType string

const (
	PromoNone       PromoType = ""           // 无促销/正常（下载上传均不计优惠）
	PromoFree       PromoType = "free"       // 免费（下载因子=0）
	Promo2xFree     PromoType = "2xfree"     // 2X免费（免费+双倍上传）
	Promo2x         PromoType = "2x"         // 双倍上传（不免费）
	PromoDiscount   PromoType = "discount"   // 折扣下载（50%/30%等，0<因子<1）
	Promo2xDiscount PromoType = "2xdiscount" // 2X+折扣（双倍上传+折扣下载）
)

// PromoTypeOf 按 Torrent 的下载/上传因子 + Free 标记判定促销类型。
// 因子 0 的约定（与 filter.torrentDownloadFactor 一致）：DownloadVolumeFactor=0 且 Free=false
// 视为"未设"→归一化为 1（正常）；UploadVolumeFactor=0 视为"未设"→1（schema 未填充时降级，
// 此时只能识别下载端促销，上传倍数类促销会被漏判——由各 schema 填充 UploadVolumeFactor 补全）。
func PromoTypeOf(t Torrent) PromoType {
	dvf := t.DownloadVolumeFactor
	if dvf == 0 && !t.Free {
		dvf = 1
	}
	uvf := t.UploadVolumeFactor
	if uvf == 0 {
		uvf = 1
	}
	twoX := uvf >= 2
	switch {
	case dvf == 0 && twoX:
		return Promo2xFree
	case dvf == 0:
		return PromoFree
	case dvf < 1 && twoX:
		return Promo2xDiscount
	case twoX:
		return Promo2x
	case dvf < 1:
		return PromoDiscount
	default:
		return PromoNone
	}
}

// nexusPromotionClassFactor NexusPHP 促销图标 class → (下载因子, 上传因子)。
// class 名是 NexusPHP 框架标准（spstate），80+ 站共用；未命中返回 ok=false。
func nexusPromotionClassFactor(cls string) (dvf, uvf float64, ok bool) {
	switch cls {
	case "pro_free", "pro_free_forever":
		return 0, 1, true
	case "pro_free2up":
		return 0, 2, true
	case "pro_2up":
		return 1, 2, true
	case "pro_50pctdown", "pro_halfdown":
		return 0.5, 1, true
	case "pro_2up50pctdown", "pro_2uphalfdown":
		return 0.5, 2, true
	case "pro_30pctdown":
		return 0.7, 1, true
	}
	return 0, 0, false
}

// nexusPromotionFactor 从种子行提取首个 pro_* 促销图标的因子。
// NexusPHP 种子列表用 <img class="pro_free/pro_2up/..."> 标记促销；取首个命中标准 class 的图标。
// class 可能含多段（如 "pro_free icon"），逐段匹配 pro_ 前缀，命中即停。
func nexusPromotionFactor(row *goquery.Selection) (dvf, uvf float64, ok bool) {
	row.Find(`img[class^="pro_"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		c, _ := s.Attr("class")
		for _, part := range strings.Fields(c) {
			if !strings.HasPrefix(part, "pro_") {
				continue
			}
			if d, u, hit := nexusPromotionClassFactor(part); hit {
				dvf, uvf, ok = d, u, true
				return false
			}
		}
		return true
	})
	return dvf, uvf, ok
}
