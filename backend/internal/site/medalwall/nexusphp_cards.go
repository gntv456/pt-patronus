package medalwall

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func parseShopDoc(doc *goquery.Document, baseURL string) []Item {
	if items := parseShopCards(doc, baseURL); len(items) >= 2 {
		return items
	}
	if items := parseShopTable(doc, baseURL); len(items) > 0 {
		return items
	}
	// 卡片不足 2 也作 best-effort 返回
	if items := parseShopCards(doc, baseURL); len(items) > 0 {
		return items
	}
	return nil
}

// parseShopCards 卡片式布局（好学 .medal-card + view-detail 按钮 data-* / Audiences article.medal-item）。
// 数据源优先 card 内带 data-name 的元素（好学 view-detail 按钮集中 data-id/data-name/data-price/
// data-bonus/data-duration/data-inventory/data-sale-begin/end/data-image 全部字段），否则用 card 本身。

func parseShopCards(doc *goquery.Document, baseURL string) []Item {
	var out []Item
	// 不含 div[data-medal-id]：财神 worn-medal-item（顶部「当前佩戴」栏）是 div[data-medal-id]，
	// 会误匹配；合法商店 card 都有具体 class（.medal-card/.medal-item）或 article[data-medal-id]。
	doc.Find(".medal-card, .card-item, .medal-item, article[data-medal-id]").Each(func(_ int, s *goquery.Selection) {
		// 数据源：好学 view-detail 按钮带 data-name，优先；否则 card 本身（Audiences article）
		src := s
		if el := s.Find("[data-name]"); el.Length() > 0 {
			src = el.First()
		}
		id := firstNonEmpty(src.AttrOr("data-id", ""), s.AttrOr("data-id", ""), src.AttrOr("data-medal-id", ""), s.AttrOr("data-medal-id", ""))
		// 名称：优先 data-name（干净），否则 .medal-title 等（克隆后移除 owned-badge 防污染）
		name := strings.TrimSpace(src.AttrOr("data-name", ""))
		if name == "" {
			nameSel := s.Find(".medal-title, .medal-name, .medal-item__name, .card-title, .item-name, .name, h2, h3, h4, .title").First().Clone()
			nameSel.Find(".owned-badge, .badge, span[title]").Remove()
			name = strings.TrimSpace(nameSel.Text())
		}
		if id == "" && name == "" {
			return
		}
		origPrice := parsePrice(firstNonEmpty(src.AttrOr("data-price", ""), s.Find(".medal-price, .price").First().Text(), detailItem(s, "定价"), detailItem(s, "价格")))
		buyPrice := parsePrice(firstNonEmpty(src.AttrOr("data-buy-price", ""), detailItem(s, "购买价")))
		if buyPrice == 0 {
			buyPrice = origPrice
		}
		it := Item{
			MedalID:        strings.TrimSpace(id),
			Name:           name,
			Description:    strings.TrimSpace(firstNonEmpty(s.Find(".medal-item__desc, .medal-description, .medal-desc").First().Text(), detailItem(s, "描述"))),
			ImageSmall:     firstNonEmpty(src.AttrOr("data-image", ""), s.Find("[data-original]").AttrOr("data-original", ""), resolveImg(s.Find("img"), baseURL)),
			Price:          buyPrice,
			OriginalPrice:  origPrice,
			Validity:       strings.TrimSpace(firstNonEmpty(src.AttrOr("data-duration", ""), src.AttrOr("data-validity", ""), s.Find(".medal-validity").Text(), detailItem(s, "时长"), detailItem(s, "有效期"), detailItem(s, "天数"))),
			BonusRate:      strings.TrimSpace(firstNonEmpty(src.AttrOr("data-bonus", ""), s.Find(".medal-bonus, .medal-item__buff").Text(), detailItem(s, "加成"))),
			Group:          strings.TrimSpace(firstNonEmpty(src.AttrOr("data-group", ""), s.Find(".medal-group, .group").Text())),
			Currency:       detectCurrency(s),
			PurchaseStatus: parseCardStatus(s),
			Stock:          -1,
		}
		if st := firstNonEmpty(src.AttrOr("data-inventory", ""), src.AttrOr("data-stock", ""), s.Find(".medal-stock, .stock").Text(), detailItem(s, "库存")); st != "" {
			it.Stock = parseStock(st)
		}
		if b := firstNonEmpty(src.AttrOr("data-sale-begin", ""), src.AttrOr("data-saleBegin", "")); b != "" && b != "不限" {
			it.SaleBeginTime = parseSaleTimeMs(b)
		}
		if e := firstNonEmpty(src.AttrOr("data-sale-end", ""), src.AttrOr("data-saleEnd", "")); e != "" && e != "不限" {
			it.SaleEndTime = parseSaleTimeMs(e)
		}
		it.MedalID = resolveMedalID(it.MedalID, s)
		applySaleWindow(&it)
		out = append(out, it)
	})
	return out
}

// parseCardStatus 从卡片推断购买状态。
//
//	好学：owned-badge / buy 按钮"已拥有"（disabled）；财神：purchased/locked class；Audiences：is-soldout/data-owned；
//	UBits：unpurchased class（≠purchased，须 token 精确匹配）+ 按钮 value 购买/仅授予/已过可购买时间。

func parseCardStatus(s *goquery.Selection) Status {
	if s.Find(".owned-badge").Length() > 0 {
		return StatusOwned
	}
	cls, _ := s.Attr("class")
	// token 精确匹配（非子串）：避免 "purchased" 误中 UBits "unpurchased"、"owned" 误中 "unowned"。
	if hasClassToken(cls, "is-owned", "purchased", "owned") {
		return StatusOwned
	}
	if hasClassToken(cls, "locked") {
		return StatusNotOnSale
	}
	if hasClassToken(cls, "is-soldout", "sold-out", "out-of-stock") {
		return StatusOutOfStock
	}
	if v := firstNonEmpty(s.AttrOr("data-owned", ""), s.AttrOr("data-is-owned", "")); v == "1" || strings.EqualFold(v, "true") {
		return StatusOwned
	}
	// 操作按钮 value/text 判定（好学 buy/LongPT input value/蟹黄堡 input 各自文案）
	status := StatusAvailable
	s.Find("input[type='button'], button").Each(func(_ int, el *goquery.Selection) {
		if status != StatusAvailable {
			return
		}
		combined := el.Text() + el.AttrOr("value", "")
		switch {
		case strings.Contains(combined, "已拥有") || strings.Contains(combined, "已擁有") || strings.Contains(combined, "已购") || strings.Contains(combined, "已購") || strings.Contains(combined, "已经购买") || strings.Contains(combined, "已經購買"):
			status = StatusOwned
		case strings.Contains(combined, "售罄") || strings.Contains(combined, "库存不足") || strings.Contains(combined, "庫存不足"):
			status = StatusOutOfStock
		case strings.Contains(combined, "仅授予") || strings.Contains(combined, "僅授予"):
			status = StatusGrantOnly
		case strings.Contains(combined, "不可") || strings.Contains(combined, "未开售") || strings.Contains(combined, "未開售") || strings.Contains(combined, "未到"):
			status = StatusNotOnSale
		case strings.Contains(combined, "已过") || strings.Contains(combined, "已過") || strings.Contains(combined, "过期") || strings.Contains(combined, "過期") || strings.Contains(combined, "结束") || strings.Contains(combined, "結束"):
			status = StatusDiscontinued
		}
	})
	return status
}

// hasClassToken 检查 class 字符串是否含任一确切 token（空格分隔），避免子串误匹配
// （如 "purchased" 误中 UBits "unpurchased"、"owned" 误中 "unowned"）。

func hasClassToken(cls string, tokens ...string) bool {
	for _, c := range strings.Fields(cls) {
		for _, t := range tokens {
			if c == t {
				return true
			}
		}
	}
	return false
}

// detailItem 从 .medal-detail-item / .stat-item / Audiences ul.medal-item__meta>li / UBits .medal-info>div
// 按 label 找值，跨多种 label/value 摆放方式鲁棒：
//   - 好学 <span>label:</span><strong>VALUE</strong>（strong=value，无冒号）
//   - 财神 .stat-item 文本>.stat-value / LongPT .detail-label+.detail-value / Audiences .meta-k+.meta-v
//   - UBits <strong>LABEL：</strong>value（strong=label 含冒号，value=剩余裸文本，与好学相反）
//   - 青蛙 table.medal-details <tr><td>LABEL</td><td>VALUE</td></tr>（it.Is("tr") 取第二列）

func detailItem(s *goquery.Selection, label string) string {
	var val string
	s.Find(".medal-detail-item, .detail-item, .stat-item, .medal-item__meta li, .medal-info > div:has(strong), .medal-details tbody tr").Each(func(_ int, it *goquery.Selection) {
		if val != "" {
			return
		}
		if !strings.Contains(it.Text(), label) {
			return
		}
		// tr 布局（青蛙 medal-details：<tr><td>label</td><td>value</td></tr>）→ 取第二列
		if it.Is("tr") {
			if tds := it.Find("td"); tds.Length() >= 2 {
				val = strings.TrimSpace(tds.Last().Text())
			}
			return
		}
		// value 元素优先（财神 .stat-value / LongPT .detail-value / Audiences .medal-item__meta-v / 通用 .value/b）
		if v := it.Find(".stat-value, .detail-value, .value, .medal-item__meta-v, b").First(); v.Length() > 0 {
			val = strings.TrimSpace(v.Text())
			return
		}
		// strong 二义：含冒号=label（UBits，取去掉 label 后的剩余文本），无冒号=value（好学）
		if strong := it.Find("strong").First(); strong.Length() > 0 {
			st := strong.Text()
			if strings.ContainsAny(st, "：:") {
				val = strings.TrimSpace(strings.TrimPrefix(it.Text(), st))
			} else {
				val = strings.TrimSpace(st)
			}
		}
	})
	return val
}

// detectCurrency 从卡片 meta label 推断货币单位：含「爆米花」→爆米花（Audiences）、「U币」→U币（UBits）、
// 「魔力」→魔力（LongPT/好学/财神），「蝌蚪」→蝌蚪（青蛙，魔力别称），「金币/Karma」同理；无货币词默认魔力。

func detectCurrency(s *goquery.Selection) string {
	cur := ""
	s.Find(".medal-item__meta-k, .medal-detail-item span, .stat-item, .detail-label, .detail-item, .medal-info strong, .medal-info td").Each(func(_ int, el *goquery.Selection) {
		if cur != "" {
			return
		}
		switch t := el.Text(); {
		case strings.Contains(t, "爆米花"):
			cur = "爆米花"
		case strings.Contains(t, "U币"):
			cur = "U币"
		case strings.Contains(t, "蝌蚪"):
			cur = "蝌蚪"
		case strings.Contains(t, "魔力"):
			cur = "魔力"
		case strings.Contains(t, "金币"):
			cur = "金币"
		case strings.Contains(t, "金幣"):
			cur = "金幣"
		case strings.Contains(t, "Karma"):
			cur = "Karma"
		}
	})
	if cur == "" {
		cur = "魔力"
	}
	return cur
}

// parseShopTable 表格布局（经典 NexusPHP 勋章表，如蟹黄堡）。用列头 td.colhead 文本映射列下标
// （ID/图片/描述/可购买时间/有效期/加成/价格/库存/购买/赠送），按列名精确取值，跨列序变动鲁棒。
// 购买状态从「购买」列 input 的 value/disabled 推断（购买/已经购买/库存不足/仅授予/已过）。
