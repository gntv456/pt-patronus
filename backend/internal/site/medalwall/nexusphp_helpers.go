package medalwall

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

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
