// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package medalwall

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func parseShopTable(doc *goquery.Document, baseURL string) []Item {
	var tbl *goquery.Selection
	doc.Find("table").EachWithBreak(func(_ int, t *goquery.Selection) bool {
		if t.Find("table").Length() > 0 {
			return true // 跳过非叶子表（外层 main 表的后代查找会误含内层勋章表的 colhead）
		}
		heads := t.Find("td.colhead, th")
		if heads.Length() < 4 {
			return true
		}
		txt := heads.Text()
		if (strings.Contains(txt, "价格") || strings.Contains(txt, "價格")) &&
			(strings.Contains(txt, "库存") || strings.Contains(txt, "庫存") || strings.Contains(txt, "ID")) {
			tbl = t
			return false
		}
		return true
	})
	if tbl == nil {
		return nil
	}
	// 列头文本 → 列下标
	cols := map[string]int{}
	headRow := tbl.Find("thead tr").First()
	if headRow.Length() == 0 {
		headRow = tbl.Find("tr").First()
	}
	headRow.Find("td.colhead, th").Each(func(i int, td *goquery.Selection) {
		h := strings.TrimSpace(td.Text())
		switch {
		case h == "ID" || h == "id":
			cols["id"] = i
		case strings.Contains(h, "图片") || strings.Contains(h, "圖片") || strings.Contains(h, "图标") || strings.Contains(h, "圖標") || strings.Contains(h, "图像") || strings.Contains(h, "圖像"):
			cols["img"] = i
		case strings.Contains(h, "描述") || strings.Contains(h, "名称") || strings.Contains(h, "勋章") || strings.Contains(h, "标题"):
			cols["name"] = i
		case h == "价格" || h == "價格" || strings.Contains(h, "费用") || strings.Contains(h, "費用") || strings.Contains(h, "花费") || strings.Contains(h, "花費"):
			cols["price"] = i
		case strings.Contains(h, "库存") || strings.Contains(h, "庫存"):
			cols["stock"] = i
		case strings.Contains(h, "加成"):
			cols["bonus"] = i
		case strings.Contains(h, "有效期"):
			cols["validity"] = i
		case strings.Contains(h, "可购买时间") || strings.Contains(h, "可購買時間") || strings.Contains(h, "时间") || strings.Contains(h, "時間"):
			cols["time"] = i
		case h == "购买" || h == "購買" || (strings.Contains(h, "购买") && !strings.Contains(h, "可购买")) || (strings.Contains(h, "購買") && !strings.Contains(h, "可購買")):
			cols["buy"] = i
		}
	})
	if _, ok := cols["price"]; !ok {
		return nil // 无价格列 → 非勋章表
	}
	var out []Item
	// 部分 NexusPHP 站点直接把 tr 放在 table 下，不输出 tbody；所选 tbl 已是无嵌套的叶子表，
	// 因此遍历全部 tr 并显式跳过表头即可同时兼容两种结构。
	tbl.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		if tr.Find("td.colhead").Length() > 0 {
			return // 跳过混入 tbody 的表头行
		}
		tds := tr.Find("td")
		if tds.Length() < 3 {
			return
		}
		get := func(key string) *goquery.Selection {
			if i, ok := cols[key]; ok && i < tds.Length() {
				return tds.Eq(i)
			}
			return nil
		}
		it := Item{Currency: detectTableCurrency(headRow), PurchaseStatus: StatusAvailable, Stock: -1}
		if td := get("id"); td != nil {
			it.MedalID = strings.TrimSpace(td.Text())
		}
		if td := get("img"); td != nil {
			it.ImageSmall = resolveImg(td.Find("img"), baseURL)
		}
		if td := get("name"); td != nil {
			if h1 := td.Find("h1, h2, h3, .name").First(); h1.Length() > 0 {
				it.Name = strings.TrimSpace(h1.Text())
				it.Description = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(td.Text()), strings.TrimSpace(h1.Text())))
			} else {
				it.Name = strings.TrimSpace(td.Text())
			}
		}
		if td := get("price"); td != nil {
			it.Price = parsePrice(td.Text())
		}
		if td := get("stock"); td != nil {
			st := strings.TrimSpace(td.Text())
			it.Stock = parseStock(st)
			it.StockStatus = st
		}
		if td := get("bonus"); td != nil {
			it.BonusRate = strings.TrimSpace(td.Text())
		}
		if td := get("validity"); td != nil {
			it.Validity = strings.TrimSpace(td.Text())
		}
		if td := get("buy"); td != nil {
			it.PurchaseStatus = parseBuyCell(td)
		}
		if it.MedalID == "" {
			it.MedalID = resolveMedalID("", tr)
		}
		if it.Name == "" && it.MedalID == "" {
			return
		}
		applySaleWindow(&it)
		out = append(out, it)
	})
	return out
}

// parseBuyCell 从购买单元格 input 的 value/disabled 推断状态（蟹黄堡经典表）。
//
//	"购买"→AVAILABLE；"已经购买"/"已拥有"→OWNED；"库存不足"→OUT_OF_STOCK；
//	"已过"/"过期"→DISCONTINUED；"仅授予"/"不可购买"→NOT_ON_SALE；其它 disabled→UNKNOWN。

func parseBuyCell(td *goquery.Selection) Status {
	input := td.Find("input").First()
	if input.Length() == 0 {
		return StatusAvailable
	}
	val := input.AttrOr("value", "")
	_, disabled := input.Attr("disabled")
	switch {
	case strings.Contains(val, "已经购买") || strings.Contains(val, "已經購買") || strings.Contains(val, "已拥有") || strings.Contains(val, "已擁有") || strings.Contains(val, "已购") || strings.Contains(val, "已購"):
		return StatusOwned
	case strings.Contains(val, "库存不足") || strings.Contains(val, "庫存不足") || strings.Contains(val, "售罄"):
		return StatusOutOfStock
	case strings.Contains(val, "已过") || strings.Contains(val, "已過") || strings.Contains(val, "过期") || strings.Contains(val, "過期") || strings.Contains(val, "结束") || strings.Contains(val, "結束"):
		return StatusDiscontinued
	case strings.Contains(val, "仅授予") || strings.Contains(val, "僅授予"):
		return StatusGrantOnly
	case strings.Contains(val, "不可") || strings.Contains(val, "未开售") || strings.Contains(val, "未開售") || strings.Contains(val, "未到"):
		return StatusNotOnSale
	case strings.Contains(val, "购买") || strings.Contains(val, "購買"):
		if disabled {
			return StatusUnknown
		}
		return StatusAvailable
	}
	return StatusAvailable
}

// parseOwnedHTML 已拥有勋章。实测好学/财神/蟹黄堡 userdetails.php：
//   - img.nexus-username-medal（个人主页顶部小图）= 当前佩戴 → Wearing=true
//   - img.nexus-username-medal-big（勋章墙大图）= 拥有（未必佩戴）→ Wearing=false
//
// 同名按 name 去重并合并（小图优先，佩戴标记保留）。不从 src 抠 id（文件名 hash），靠名称匹配商店。
// uid 可选；传入时只接受与该 UID 用户节点关联的小图，避免把排行榜/邀请人/其他用户的佩戴图混入。
