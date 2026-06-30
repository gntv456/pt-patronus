package savept

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// parseSavept 把 savept.icu 首页 HTML 解析为站点卡片 + 公告。
// savept.icu 用 data-* 属性标注每张卡片，结构稳定；此处按该契约正则抽取。
// 真实页面结构若有变动，参照站点选择器校准工作流（另存页面 → fixture）逐项校准。
func parseSavept(html string) ([]SaveptSiteCard, []SaveptAlert) {
	cards := extractCards(html)
	alerts := extractAlerts(html)
	// 抢救时长（来自病危公告）回填到匹配卡片
	for _, fd := range extractFaultDurations(alerts) {
		for i := range cards {
			if nameFuzzyMatch(cards[i].Name, fd.name) && cards[i].FaultDuration == "" {
				cards[i].FaultDuration = fd.duration
			}
		}
	}
	return cards, alerts
}

// cardBlockRe 匹配一张卡片锚块（<a class="card-link" ...>...</a>，含 data-* 属性）。
var cardBlockRe = regexp.MustCompile(`(?is)<a[^>]*class="[^"]*card-link[^"]*"[^>]*>.*?</a>`)

// attrRe 抽取某个 data-*（或常规）属性的值。
func attr(tag, name string) string {
	re := regexp.MustCompile(`(?is)\b` + name + `\s*=\s*"([^"]*)"`)
	if m := re.FindStringSubmatch(tag); len(m) > 1 {
		return strings.TrimSpace(decodeEntities(m[1]))
	}
	return ""
}

var (
	tagRe       = regexp.MustCompile(`<[^>]+>`)
	yearGroupRe = regexp.MustCompile(`(?s)>(\d{4}|未知)\s*年\s*<.*?\((\d+)\s*个站点\)`)
)

func extractCards(html string) []SaveptSiteCard {
	var out []SaveptSiteCard
	for _, block := range cardBlockRe.FindAllString(html, -1) {
		rawStatus := attr(block, "data-status")
		statusText := stripTags(extractInner(block, "status-strip"))
		c := SaveptSiteCard{
			Name:            strings.TrimSpace(decodeEntities(attr(block, "data-name"))),
			URL:             attr(block, "href"),
			Year:            orUnknown(attr(block, "data-year")),
			Status:          normalizeStatus(rawStatus, statusText),
			StatusText:      statusText,
			SiteType:        normalizeType(attr(block, "data-type")),
			Search:          attr(block, "data-search"),
			AnniversaryText: stripTags(extractInner(block, "anniversary-badge")),
			AnniversaryFlag: attr(block, "data-anniversary"),
			DurationText:    stripTags(extractInner(block, "duration-badge")),
			OpenTime:        attr(block, "data-opentime"),
			CloseTime:       attr(block, "data-closetime"),
			Icon:            extractImg(block),
		}
		if c.URL != "" {
			c.Domain = extractDomain(c.URL)
		}
		if c.Name == "" {
			c.Name = stripTags(extractInner(block, "card-title"))
		}
		if c.Name == "" && c.URL == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// extractInner 取锚块内某个 class 标签的文本（如 status-strip / card-title）。
func extractInner(block, cls string) string {
	re := regexp.MustCompile(`(?is)<[^>]*class="[^"]*` + cls + `[^"]*"[^>]*>(.*?)<`)
	m := re.FindStringSubmatch(block)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractImg(block string) string {
	re := regexp.MustCompile(`(?is)<img[^>]*src="([^"]*)"`)
	if m := re.FindStringSubmatch(block); len(m) > 1 {
		return m[1]
	}
	return ""
}

// alertTitleMap 公告标题 → 类型。savept.icu 用 <strong>标题：</strong> 标注每条公告，
// 取该 strong 的父容器文本即为正文（每条公告独立容器，不会跨区卷入页脚联系方式等无关内容）。
// 必须按 <strong> 标签定位，而非全页面关键词扫描——否则页脚/说明区出现的标题字样会误匹配，
// 并把其后到下一个关键词之间的整段页面文字（如联系方式）错误归入公告。
var alertTitleMap = map[string]AlertType{
	"病危通知": AlertDanger,
	"死亡讣告": AlertObituary,
	"恢复公告": AlertWarning,
	"站庆预告": AlertInfo,
	"今日站庆": AlertCelebration,
}

func extractAlerts(html string) []SaveptAlert {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	var out []SaveptAlert
	doc.Find("strong").Each(func(_ int, s *goquery.Selection) {
		rawTitle := compactSpaces(s.Text())
		title := strings.TrimRight(rawTitle, "：: \t")
		typ, ok := alertTitleMap[title]
		if !ok {
			return
		}
		text := compactSpaces(s.Parent().Text())
		if strings.HasPrefix(text, rawTitle) {
			text = text[len(rawTitle):]
		}
		text = strings.TrimLeft(text, "：: \t\n ")
		if text != "" {
			out = append(out, SaveptAlert{Type: typ, Title: title, Content: text})
		}
	})
	return out
}

// faultDur 从病危公告里抽取「XX 已抢救 N 小时/天/分钟」。
type faultDuration struct {
	name     string
	duration string
}

var faultDurRe = regexp.MustCompile(`(\S+?)\s*已抢救\s*(\d+)\s*(小时|天|分钟)`)

func extractFaultDurations(alerts []SaveptAlert) []faultDuration {
	var out []faultDuration
	for _, a := range alerts {
		if a.Type != AlertDanger {
			continue
		}
		for _, m := range faultDurRe.FindAllStringSubmatch(a.Content, -1) {
			n, _ := strconv.Atoi(m[2])
			out = append(out, faultDuration{name: m[1], duration: formatFault(n, m[3])})
		}
	}
	return out
}

// formatFault 把抢救时长归一为小时后格式化为「X天Y小时」。
func formatFault(n int, unit string) string {
	hours := n
	switch unit {
	case "天":
		hours = n * 24
	case "分钟":
		return "抢救中（" + strconv.Itoa(n) + "分钟）"
	}
	days := hours / 24
	h := hours % 24
	switch {
	case days > 0 && h > 0:
		return strconv.Itoa(days) + "天" + strconv.Itoa(h) + "小时"
	case days > 0:
		return strconv.Itoa(days) + "天"
	default:
		return strconv.Itoa(h) + "小时"
	}
}

// ── 小工具 ──

func normalizeStatus(raw, text string) SiteHealthStatus {
	s := strings.ToLower(raw + " " + text)
	switch {
	case containsAny(s, "运行", "健康", "正常", "healthy", "online"):
		return HealthHealthy
	case containsAny(s, "病危", "抢救", "濒临", "critical", "danger"):
		return HealthCritical
	case containsAny(s, "已关站", "关站", "死亡", "关闭", "closed", "dead"):
		return HealthClosed
	}
	return HealthUnknown
}

func normalizeType(raw string) SiteType {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "internal", "内站", "国内":
		return TypeInternal
	case "external", "外站", "国外":
		return TypeExternal
	}
	return TypeUnknown
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	host := u.Host
	host = strings.TrimPrefix(host, "www.")
	return host
}

func nameFuzzyMatch(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "未知"
	}
	return s
}

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

func compactSpaces(s string) string {
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))
}

func decodeEntities(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
	return r.Replace(s)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
