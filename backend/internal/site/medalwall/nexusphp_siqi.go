// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package medalwall

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var siQiMedalNameSuffixRe = regexp.MustCompile(`\s*\(#[0-9]+\)\s*$`)

func isSiQiSite(baseURL string) bool {
	return strings.Contains(strings.ToLower(baseURL), "si-qi.xyz")
}

// parseSiQiShopDoc 解析思齐自定义的 medal.php 卡片布局。
// 思齐没有通用 NexusPHP 的 data-name 属性，业务 ID 位于操作按钮或 owner 统计节点，
// 卡片标题里的 (#N) 是可靠的最终兜底；图片文件名是对象存储 hash，绝不能当作 ID。

func parseSiQiShopDoc(doc *goquery.Document, baseURL string) []Item {
	var out []Item
	doc.Find(".medal-card").Each(func(_ int, card *goquery.Selection) {
		id := firstNonEmpty(
			card.Find("input.buy[data-id], input.gift[data-id]").First().AttrOr("data-id", ""),
			card.Find("[data-medal-id]").First().AttrOr("data-medal-id", ""),
		)
		name := strings.TrimSpace(card.Find(".medal-card__title h2").First().Text())
		if name == "" {
			name = strings.TrimSpace(card.Find("img[alt]").First().AttrOr("alt", ""))
		}
		name = siQiMedalNameSuffixRe.ReplaceAllString(name, "")
		if id == "" {
			if m := siQiMedalNameSuffixRe.FindStringSubmatch(strings.TrimSpace(card.Find(".medal-card__title h2").First().Text())); len(m) > 0 {
				id = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(m[0]), ")"), "(#")
			}
		}
		if id == "" || name == "" {
			return
		}

		buyPrice := parsePrice(siqiCardMeta(card, "价格"))
		validity := strings.TrimSpace(siqiCardMeta(card, "购买后有效期"))
		stockText := strings.TrimSpace(siqiCardMeta(card, "库存"))
		stock := parseStock(stockText)
		it := Item{
			MedalID:          id,
			Name:             name,
			Description:      strings.TrimSpace(card.Find(".medal-card__desc").First().Text()),
			ImageSmall:       resolveImg(card.Find("img").First(), baseURL),
			OriginalImageURL: resolveImg(card.Find("img").First(), baseURL),
			Price:            buyPrice,
			OriginalPrice:    buyPrice,
			Validity:         validity,
			BonusRate:        strings.TrimSpace(siqiCardMeta(card, "魔力加成")),
			Currency:         "魔力",
			PurchaseStatus:   parseSiQiCardStatus(card),
			Stock:            stock,
			StockStatus:      stockText,
		}
		if sale := strings.TrimSpace(siqiCardMeta(card, "可购买时间")); sale != "" {
			parts := strings.SplitN(sale, "~", 2)
			if len(parts) == 2 {
				if start := strings.TrimSpace(parts[0]); start != "" && start != "不限" {
					it.SaleBeginTime = parseSaleTimeMs(start)
				}
				if end := strings.TrimSpace(parts[1]); end != "" && end != "不限" {
					it.SaleEndTime = parseSaleTimeMs(end)
				}
			}
		}
		applySaleWindow(&it)
		out = append(out, it)
	})
	return out
}

func siqiCardMeta(card *goquery.Selection, label string) string {
	var value string
	card.Find(".medal-card__meta > div").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		key := strings.TrimSpace(row.Find(".meta-label").First().Text())
		if !strings.Contains(key, label) {
			return true
		}
		value = strings.TrimSpace(row.Find(".meta-value").First().Text())
		return false
	})
	return value
}

func parseSiQiCardStatus(card *goquery.Selection) Status {
	if hasClassToken(card.AttrOr("class", ""), "owned") {
		return StatusOwned
	}
	action := strings.TrimSpace(card.Find(".medal-card__actions").Text())
	card.Find(".medal-card__actions input, .medal-card__actions button").Each(func(_ int, el *goquery.Selection) {
		action += " " + el.AttrOr("value", "") + " " + el.Text()
	})
	switch {
	case strings.Contains(action, "仅授予"):
		return StatusGrantOnly
	case strings.Contains(action, "已过可购买时间"), strings.Contains(action, "已过期"):
		return StatusDiscontinued
	case strings.Contains(action, "库存不足"), strings.Contains(action, "售罄"):
		return StatusOutOfStock
	case strings.Contains(action, "购买"):
		return StatusAvailable
	default:
		return StatusUnknown
	}
}

func parseSiQiOwnedHTML(html, baseURL string) []Owned {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}
	var out []Owned
	doc.Find(".medal-card.owned").Each(func(_ int, card *goquery.Selection) {
		name := siQiMedalNameSuffixRe.ReplaceAllString(strings.TrimSpace(card.Find(".medal-card__title h2").First().Text()), "")
		if name == "" {
			name = strings.TrimSpace(card.Find("img[alt]").First().AttrOr("alt", ""))
		}
		if name == "" {
			return
		}
		id := firstNonEmpty(
			card.Find("input.buy[data-id], input.gift[data-id]").First().AttrOr("data-id", ""),
			card.Find("[data-medal-id]").First().AttrOr("data-medal-id", ""),
		)
		out = append(out, Owned{
			MedalID: id,
			Name:    name,
			Image:   resolveImg(card.Find("img").First(), baseURL),
			// 思齐页面目前没有可验证的佩戴标记；拥有不等于佩戴。
			Wearing: false,
		})
	})
	return out
}
