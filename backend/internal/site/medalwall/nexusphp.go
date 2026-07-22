package medalwall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// NexusPHPAdapter NexusPHP 系勋章墙：GET medal.php?page={N} 分页解析（卡片/表格双布局）+
// GET userdetails.php?id=<uid> 抓已拥有；购买/佩戴/摘下走 AJAX POST（多端点回退）。
type NexusPHPAdapter struct{}

func (NexusPHPAdapter) Platform() string { return "nexusphp" }
func (NexusPHPAdapter) Supported() bool  { return true }

// 分页上限 + 关键词门槛（整页须含勋章关键词，防误识非勋章页）。
const (
	shopMaxPages = 20
)

var shopKeywordRe = regexp.MustCompile(`加成|火花|佩戴|購買|购买|庫存|库存|價格|价格|魔力|電力|金币|金幣|勋章|勛章|有效期`)

// FetchShop 分页抓商店勋章（medal.php?page=N），含防伪分页（委托 fetchShopPaged）。
// 末尾追加 LongPT 六十四卦合成勋章（pluginsMedalInfo/getList，仅 LongPT 有，其他站跳过）。
func (NexusPHPAdapter) FetchShop(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie string) ([]Item, error) {
	path := def.ShopPath
	if path == "" {
		path = "medal.php"
	}
	if isSiQiSite(baseURL) {
		return fetchSiQiShop(ctx, hc, baseURL, cookie, path)
	}
	items, err := fetchShopPaged(ctx, hc, baseURL, cookie, path, "page")
	items = append(items, fetchSynthesisMedals(ctx, hc, baseURL, cookie)...)
	if len(items) > 0 {
		return items, nil
	}
	return nil, err
}

func fetchSiQiShop(ctx context.Context, hc HTTPDoer, baseURL, cookie, path string) ([]Item, error) {
	u := joinBase(baseURL, path)
	resp, err := hc.Get(ctx, u, cookie, true)
	if err != nil {
		return nil, &ShopFetchError{Kind: "request", URL: u, Err: err}
	}
	if resp == nil {
		return nil, &ShopFetchError{Kind: "request", URL: u}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 || isLoginURL(resp.FinalURL) {
			return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
		}
		return nil, &ShopFetchError{Kind: "response", URL: u, StatusCode: resp.StatusCode}
	}
	if isLoginURL(resp.FinalURL) {
		return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
	}
	items := parseShopHTML(string(resp.Body), baseURL)
	if len(items) == 0 {
		return nil, &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode, Err: ErrNoMedals}
	}
	return items, nil
}

// fetchSynthesisMedals 抓 LongPT 六十四卦合成勋章（pluginsMedalInfo/getList，POST {medalUnit:3}）。
// 仅 LongPT 有此插件 API（host 含 longpt），其他站直接跳过不发请求。
// 已合成的（owned=true）标 OWNED，其余 SYNTHESIS（仅合成）；medalId 用 syn: 前缀避与商店 id 冲突。
func fetchSynthesisMedals(ctx context.Context, hc HTTPDoer, baseURL, cookie string) []Item {
	if !strings.Contains(strings.ToLower(baseURL), "longpt") {
		return nil
	}
	u := joinBase(baseURL, "pt-api/v1/plugins/medals/pluginsMedalInfo/getList")
	resp, err := hc.Do(ctx, "POST", u, []byte(`{"medalUnit":"3"}`),
		map[string]string{"Content-Type": "application/json", "X-Requested-With": "XMLHttpRequest"}, cookie, true)
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var env struct {
		Code int              `json:"code"`
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(resp.Body, &env) != nil || env.Code != 0 {
		return nil
	}
	out := make([]Item, 0, len(env.Data))
	for _, m := range env.Data {
		name, _ := m["medalName"].(string)
		if name == "" {
			continue
		}
		img, _ := m["medalImageUrl"].(string)
		bonus := ""
		if bf, ok := m["bonusAdditionFactor"]; ok && bf != nil {
			bonus = fmt.Sprintf("%v", bf)
		}
		st := StatusSynthesis
		if owned, _ := m["owned"].(bool); owned {
			st = StatusOwned
		}
		out = append(out, Item{
			MedalID:          "syn:" + fmt.Sprintf("%v", m["medalRealId"]),
			Name:             name,
			ImageSmall:       resolveAsset(baseURL, img),
			OriginalImageURL: resolveAsset(baseURL, img),
			BonusRate:        bonus,
			Currency:         "魔力",
			PurchaseStatus:   st,
		})
	}
	return out
}

// fetchShopPaged 通用分页抓取：先 GET {baseURL}/{path} 首页，再以 ?{param}={N} 翻页，含 4 重防伪分页
// （内容指纹去重 / 整页皆已见即停 / 行数骤降 <40% / 20 页上限）。NexusPHP(page) 与 TNode(p) 共用。
func fetchShopPaged(ctx context.Context, hc HTTPDoer, baseURL, cookie, path, param string) ([]Item, error) {
	shop := joinBase(baseURL, path)
	var all []Item
	seenFp := map[string]bool{}
	firstPageCount := 0
	for page := 0; page < shopMaxPages; page++ {
		u := shop
		if page > 0 {
			u = appendParam(shop, fmt.Sprintf("%s=%d", param, page))
		}
		resp, err := hc.Get(ctx, u, cookie, true)
		if resp != nil && isLoginURL(resp.FinalURL) && len(all) == 0 {
			return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
		}
		if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || len(resp.Body) < 200 {
			// 尚未抓到任何数据时才把失败上抛；翻页越界则静默停止。
			if len(all) == 0 {
				if err != nil {
					slog.Warn("medal shop fetch failed", "url", u, "err", err)
					return nil, &ShopFetchError{Kind: "request", URL: u, Err: err}
				} else if resp == nil {
					return nil, &ShopFetchError{Kind: "request", URL: u}
				} else if resp != nil {
					slog.Warn("medal shop fetch non-2xx/empty", "url", u, "status", resp.StatusCode, "bytes", len(resp.Body))
					if resp.StatusCode == 401 || resp.StatusCode == 403 || isLoginURL(resp.FinalURL) {
						return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
					}
					if resp.StatusCode < 200 || resp.StatusCode >= 300 {
						return nil, &ShopFetchError{Kind: "response", URL: u, StatusCode: resp.StatusCode}
					}
					if isLoginURL(resp.FinalURL) {
						return nil, &ShopFetchError{Kind: "auth", URL: u, StatusCode: resp.StatusCode}
					}
					return nil, &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode}
				}
			}
			break
		}
		items := parseShopHTML(string(resp.Body), baseURL)
		if len(items) == 0 {
			// 少数站点只在 ?page=1 返回首页，允许无参数页解析为空后再试一次。
			if page == 0 {
				continue
			}
			if len(all) == 0 {
				slog.Warn("medal shop parse empty", "url", u, "bytes", len(resp.Body))
				return nil, &ShopFetchError{Kind: "parse", URL: u, StatusCode: resp.StatusCode, Err: ErrNoMedals}
			}
			break
		}
		if firstPageCount == 0 {
			firstPageCount = len(items)
		}
		// 内容指纹：整页全是已见条目 → 翻页失效
		dup := 0
		var fps []string
		for _, it := range items {
			fp := medalFingerprint(it)
			fps = append(fps, fp)
			if seenFp[fp] {
				dup++
			}
		}
		if dup == len(items) {
			break
		}
		for _, fp := range fps {
			seenFp[fp] = true
		}
		// 行数骤降 < 首页 40% → 越界
		if len(all) > 0 && firstPageCount > 0 && float64(len(items)) < float64(firstPageCount)*0.4 {
			break
		}
		all = append(all, items...)
	}
	return all, nil
}

func isLoginURL(rawURL string) bool {
	u := strings.ToLower(rawURL)
	return strings.Contains(u, "login.php") || strings.Contains(u, "takelogin.php") || strings.Contains(u, "/login")
}

// FetchOwned 抓已拥有勋章；uid 为空返回 nil（chain 层负责发现 uid）。
func (NexusPHPAdapter) FetchOwned(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, uid string) ([]Owned, error) {
	// 思齐的 userdetails.php 是一个全站聚合页：其中的
	// img.nexus-username-medal 属于排行榜里的其他用户和玩偶，不能用来判断当前账号。
	// medal.php 的卡片才带有当前账号的 owned 状态；该页面不需要 uid。
	if isSiQiSite(baseURL) {
		resp, err := hc.Get(ctx, shopPath(def, baseURL), cookie, true)
		if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, nil
		}
		return parseSiQiOwnedHTML(string(resp.Body), baseURL), nil
	}
	if uid == "" {
		return nil, nil
	}
	u := fmt.Sprintf("%s?id=%s", ownPath(def, baseURL), uid)
	resp, err := hc.Get(ctx, u, cookie, true)
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	return parseOwnedHTML(string(resp.Body), baseURL, uid), nil
}

// Buy 购买：rousipro/通用走 ajax.php?action=buyMedal，否则回退 3 个 medal.php?action=buy 端点。
// 依序尝试，首个确认成功即返回（对齐旧版 4 端点策略）。
func (NexusPHPAdapter) Buy(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	if medalID == "" {
		return ActionResult{Success: false, Message: "缺少勋章 ID"}, nil
	}
	shop := shopPath(def, baseURL)
	hdrs := ajaxHeaders(shop)
	// 端点 1：ajax.php action=buyMedal
	form := url.Values{}
	form.Set("action", "buyMedal")
	form.Set("params[medal_id]", medalID)
	if r, err := hc.Do(ctx, "POST", joinBase(baseURL, "ajax.php"), []byte(form.Encode()), hdrs, cookie, true); err == nil && r != nil {
		if ok, msg := parseActionOK(r.Body); ok {
			return ActionResult{Success: true, Message: firstNonEmpty(msg, "购买成功")}, nil
		}
	}
	// 端点 2-4：medal.php?action=buy&id= / &medalid= / &medal_id=
	// body 同时带 action/id（Audiences medal_center.php 走 form hidden，后端读 $_POST 而非 query）。
	for _, key := range []string{"id", "medalid", "medal_id"} {
		u := appendParam(shop, fmt.Sprintf("action=buy&%s=%s", key, medalID))
		body := fmt.Sprintf("buy=1&action=buy&%s=%s", key, medalID)
		if r, err := hc.Do(ctx, "POST", u, []byte(body), hdrs, cookie, true); err == nil && r != nil {
			if ok, msg := parseActionOK(r.Body); ok {
				return ActionResult{Success: true, Message: firstNonEmpty(msg, "购买成功")}, nil
			}
		}
	}
	return ActionResult{Success: false, Message: "购买失败（站点未确认成功）"}, nil
}

// Wear 佩戴：POST medal.php?action=wear&id= body wear=1，HTTP 2xx 判定。
func (NexusPHPAdapter) Wear(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	return nexusFormAction(ctx, hc, shopPath(def, baseURL), cookie, medalID, "wear", "wear")
}

// TakeOff 摘下：POST medal.php?action=takeoff&id= body takeoff=1。
func (NexusPHPAdapter) TakeOff(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error) {
	return nexusFormAction(ctx, hc, shopPath(def, baseURL), cookie, medalID, "takeoff", "takeoff")
}

// nexusFormAction 佩戴/摘下通用：2xx 视为成功。
func nexusFormAction(ctx context.Context, hc HTTPDoer, shop, cookie, medalID, action, bodyKey string) (ActionResult, error) {
	u := appendParam(shop, fmt.Sprintf("action=%s&id=%s", action, medalID))
	r, err := hc.Do(ctx, "POST", u, []byte(bodyKey+"=1"), ajaxHeaders(shop), cookie, true)
	if err != nil {
		return ActionResult{Success: false, Message: "请求失败：" + err.Error()}, nil
	}
	if r != nil && r.StatusCode >= 200 && r.StatusCode < 300 {
		return ActionResult{Success: true, Message: "操作已提交"}, nil
	}
	return ActionResult{Success: false, Message: "操作失败"}, nil
}

// ── 商店页解析（卡片 + 表格双布局）──

// parseShopHTML 入口：关键词门槛 → goquery → parseShopDoc。
func parseShopHTML(html, baseURL string) []Item {
	if !shopKeywordRe.MatchString(html) {
		return nil // 非勋章页，防误识
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}
	if isSiQiSite(baseURL) {
		return parseSiQiShopDoc(doc, baseURL)
	}
	return parseShopDoc(doc, baseURL)
}

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
func parseOwnedHTML(html, baseURL string, uidOpt ...string) []Owned {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}
	uid := ""
	if len(uidOpt) > 0 {
		uid = strings.TrimSpace(uidOpt[0])
	}
	var out []Owned
	seen := map[string]int{} // name → out 下标
	add := func(name, image string, wearing bool) {
		if name == "" {
			return
		}
		if idx, ok := seen[name]; ok {
			if wearing {
				out[idx].Wearing = true
			}
			return
		}
		seen[name] = len(out)
		out = append(out, Owned{Name: name, Image: image, Wearing: wearing})
	}
	doc.Find("img.nexus-username-medal").Each(func(_ int, s *goquery.Selection) {
		if uid != "" && !medalBelongsToUID(s, uid) {
			return
		}
		add(strings.TrimSpace(firstNonEmpty(s.AttrOr("title", ""), s.AttrOr("alt", ""))), resolveAsset(baseURL, s.AttrOr("src", "")), true)
	})
	doc.Find("img.nexus-username-medal-big").Each(func(_ int, s *goquery.Selection) {
		add(strings.TrimSpace(firstNonEmpty(s.AttrOr("title", ""), s.AttrOr("alt", ""))), resolveAsset(baseURL, s.AttrOr("src", "")), false)
	})
	// LongPT userdetails: #medal-form .medal-card（用户拥有勋章；input.medal-status-checkbox checked=佩戴）
	doc.Find("#medal-form .medal-card").Each(func(_ int, card *goquery.Selection) {
		img := card.Find("img").First()
		name := strings.TrimSpace(firstNonEmpty(img.AttrOr("title", ""), img.AttrOr("alt", "")))
		if name == "" {
			return
		}
		wearing := false
		if cb := card.Find("input.medal-status-checkbox").First(); cb.Length() > 0 {
			if v, ok := cb.Attr("checked"); ok && (v == "" || strings.EqualFold(v, "checked")) {
				wearing = true
			}
		}
		add(name, resolveAsset(baseURL, img.AttrOr("src", "")), wearing)
	})
	doc.Find("img[class*='medal']").Not("img.nexus-username-medal, img.nexus-username-medal-big").Each(func(_ int, s *goquery.Selection) {
		add(strings.TrimSpace(firstNonEmpty(s.AttrOr("title", ""), s.AttrOr("alt", ""))), resolveAsset(baseURL, s.AttrOr("src", "")), false)
	})
	return out
}

// medalBelongsToUID 判断小勋章图片最近的用户节点是否链接到目标 UID。
// 只向上检查有限层级，避免把整页排行榜容器误当作单个用户节点。
func medalBelongsToUID(img *goquery.Selection, uid string) bool {
	cur := img.Parent()
	for depth := 0; depth < 4 && cur.Length() > 0; depth++ {
		matched := false
		foundUser := false
		cur.Find("a[href*='userdetails.php']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			foundUser = true
			if m := hrefIDRe.FindStringSubmatch(a.AttrOr("href", "")); len(m) > 1 && m[1] == uid {
				matched = true
				return false
			}
			return true
		})
		if matched {
			return true
		}
		if foundUser {
			return false
		}
		cur = cur.Parent()
	}
	return false
}

// ── helpers ──

func shopPath(def Def, baseURL string) string {
	p := def.ShopPath
	if p == "" {
		p = "medal.php"
	}
	return joinBase(baseURL, p)
}

func ownPath(def Def, baseURL string) string {
	p := def.OwnPath
	if p == "" {
		p = "userdetails.php"
	}
	return joinBase(baseURL, p)
}

func joinBase(base, path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/" + path
}

// appendParam 给 rawURL 追加一个查询参数（如 "page=2"），按是否已含 '?' 选分隔符，
// 支持站点商店路径自带 query（青蛙 medal.php?type=-1 → medal.php?type=-1&page=2）。
func appendParam(rawURL, kv string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + kv
}

func resolveImg(sel *goquery.Selection, baseURL string) string {
	if sel.Length() == 0 {
		return ""
	}
	src := sel.First().AttrOr("src", "")
	if src == "" {
		src = sel.First().AttrOr("data-src", "")
	}
	return resolveAsset(baseURL, src)
}

func resolveAsset(baseURL, u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.HasPrefix(strings.ToLower(u), "data:") {
		return u
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return u
	}
	base := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/index.php")
	if strings.HasPrefix(u, "/") {
		return base + u
	}
	return base + "/" + u
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var (
	digitRe           = regexp.MustCompile(`(\d+)`)
	priceRe           = regexp.MustCompile(`([\d.]+)`)
	buyMedalOnclickRe = regexp.MustCompile(`buyMedal\(\s*['"]?(\d+)['"]?`)
	hrefIDRe          = regexp.MustCompile(`[?&](?:amp;)?(?:medal_)?id=(\d+)`)
	imgIDRe           = regexp.MustCompile(`(\d+)\.(?:png|jpe?g|gif|webp)`)
	whitespaceRe      = regexp.MustCompile(`\s+`)
	// 写操作响应解析
	jsonRetRe         = regexp.MustCompile(`"ret"\s*:\s*"?(-?\d+)"?`)
	jsonCodeRe        = regexp.MustCompile(`"code"\s*:\s*"?(-?\d+)"?`)
	jsonSuccessTrueRe = regexp.MustCompile(`"success"\s*:\s*(true|1)`)
	jsonMsgRe         = regexp.MustCompile(`"msg"\s*:\s*"([^"]*)"`)
	buyOkRe           = regexp.MustCompile(`成功|兑换|success|OK`)
	buyFailRe         = regexp.MustCompile(`失败|不足|已拥有|未开售|售罄|已过期|error|fail`)
)

func parsePrice(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	m := priceRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	f, _ := strconv.ParseFloat(m[1], 64)
	return f
}

func parseStock(s string) int {
	s = strings.ReplaceAll(s, ",", "")
	if strings.Contains(s, "无限") || strings.Contains(s, "無限") || strings.Contains(s, "充足") || strings.Contains(s, "不限") {
		return -1
	}
	m := digitRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func detectTableCurrency(headRow *goquery.Selection) string {
	text := headRow.Text()
	switch {
	case strings.Contains(text, "金币"):
		return "金币"
	case strings.Contains(text, "金幣"):
		return "金幣"
	case strings.Contains(text, "电力"):
		return "电力"
	case strings.Contains(text, "電力"):
		return "電力"
	case strings.Contains(text, "蝌蚪"):
		return "蝌蚪"
	default:
		return "魔力"
	}
}

// resolveMedalID 从 input.buy[data-id] / input[name=id]value / onclick buyMedal(N) / href ?id=N / 图片文件名抠真实 id。
func resolveMedalID(cur string, s *goquery.Selection) string {
	if cur != "" {
		return cur
	}
	// 蟹黄堡等经典表：input.buy/.gift[data-id]（真实 medalId）
	if v := s.Find("input.buy, input.gift, button.buy, button.gift, input[data-id], button[data-id]").AttrOr("data-id", ""); v != "" {
		return v
	}
	if v := s.Find("input[name='id'], input[name='medal_id'], input[name='medalid']").AttrOr("value", ""); v != "" {
		return v
	}
	html, _ := s.Html()
	if m := buyMedalOnclickRe.FindStringSubmatch(html); len(m) > 1 {
		return m[1]
	}
	if m := hrefIDRe.FindStringSubmatch(html); len(m) > 1 {
		return m[1]
	}
	if m := imgIDRe.FindStringSubmatch(html); len(m) > 1 {
		return m[1]
	}
	return ""
}

// applySaleWindow 按开售/库存修正状态（未开售/缺货）。
// 仅对未明确判定的 AVAILABLE/UNKNOWN 修正；站点按钮已判定的（OWNED/GRANT_ONLY/NOT_ON_SALE/
// DISCONTINUED/SYNTHESIS/OUT_OF_STOCK）原样保留，避免被 Stock==0 误覆盖（如青蛙「仅授予」库存 0）。
// 不判 sale-end 过期为「已下架」：好学等站过期勋章可溢价购买（data-buy-price），并非真下架；
// 真正下架（如蟹黄堡/青蛙「已过可购买时间」）由 parseBuyCell/parseCardStatus 按按钮文本判定，不在此覆盖。
func applySaleWindow(it *Item) {
	if it.PurchaseStatus != StatusAvailable && it.PurchaseStatus != StatusUnknown {
		return
	}
	now := time.Now().UnixMilli()
	if it.SaleBeginTime != nil && *it.SaleBeginTime > now {
		it.PurchaseStatus = StatusNotOnSale
		return
	}
	if it.Stock == 0 {
		it.PurchaseStatus = StatusOutOfStock
	}
}

func parseSaleTimeMs(s string) *int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if ms := parseEpochMs(s); ms > 0 {
		return &ms
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02", "2006/01/02 15:04:05", "2006/01/02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			ms := t.UnixMilli()
			return &ms
		}
	}
	return nil
}

// parseEpochMs 纯数字（秒/毫秒）→ 毫秒；非法返回 0。
func parseEpochMs(s string) int64 {
	if !regexp.MustCompile(`^\d+$`).MatchString(s) {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	if n < 1e12 { // 秒
		return n * 1000
	}
	return n
}

func medalFingerprint(it Item) string {
	return normalizeName(it.Name) + "|" + strconv.FormatFloat(it.Price, 'f', -1, 64)
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return whitespaceRe.ReplaceAllString(s, "")
}

func ajaxHeaders(referer string) map[string]string {
	return map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Accept":           "application/json, text/javascript, */*; q=0.01",
		"Referer":          referer,
	}
}

// parseActionOK 解析购买响应：JSON {ret:0}/{code:0}/{success:true} 或 HTML 成功关键词。
func parseActionOK(body []byte) (bool, string) {
	s := string(body)
	if jsonSuccessTrueRe.MatchString(s) {
		return true, extractMsg(s)
	}
	if m := jsonRetRe.FindStringSubmatch(s); len(m) > 1 && m[1] == "0" {
		return true, extractMsg(s)
	}
	if m := jsonCodeRe.FindStringSubmatch(s); len(m) > 1 && (m[1] == "0" || m[1] == "200") {
		return true, extractMsg(s)
	}
	// 非 JSON：HTML 关键词（先排失败再认成功）
	if !strings.Contains(s, "{") {
		if buyFailRe.MatchString(s) {
			return false, ""
		}
		if buyOkRe.MatchString(s) {
			return true, ""
		}
	}
	return false, ""
}

func extractMsg(s string) string {
	if m := jsonMsgRe.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}
