package savept

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// openTimeOverrides 补 savept.icu 线上 data-opentime="未知" 的站（域名 → 开站日期/年份）。
// 值支持两种形态："YYYY-MM-DD"（精确日期，进站庆日历）或 "YYYY"（仅年份，展示+年份统计）。
// 来源标注在各行注释。savept.icu 修数据后可整体删除。
var openTimeOverrides = map[string]string{
	"bt.byr.cn":    "2003-09-26", // 北邮人 BYRBT（教育网最老牌 PT 之一，人工核实）
	"1337x.to":     "2007",       // 1337x（维基百科 Comparison_of_BitTorrent_sites：Launched 2007）
	"e-hentai.org": "2005-07-01", // E-Hentai（维基百科词条：Founded July 1, 2005）
}

// foundedYearOverrides 补 ptseek 独有站（无 community_profile 画像）的建站年份
// （域名 → 年份）。这些站不在 savept.icu 卡片里，只影响卡片 Year 显示与年份分组。
// 仅收编有可靠出处的年份（维基词条/多源一致），页脚版权年等弱信号不收。
var foundedYearOverrides = map[string]int{}

// applyOpenTimeOverrides 把本地已核实的开站日期回填到 data-opentime 为空/「未知」的卡片。
// 同时补 Year（savept 用 0000 标未知）与 DurationText，保持三字段自洽。
// 值为纯年份（如 "2007"）时：OpenTime 存年份、Year 同步、不推算运营时长（无精确日）。
func applyOpenTimeOverrides(cards []SaveptSiteCard) {
	for i := range cards {
		date, ok := openTimeOverrides[normalizeHost(cards[i].Domain)]
		if !ok {
			continue
		}
		if ot := strings.TrimSpace(cards[i].OpenTime); ot != "" && ot != "未知" {
			continue // 数据源已补上真实日期：以数据源为准，本地补丁不覆盖
		}
		cards[i].OpenTime = date
		if cards[i].Year == "" || cards[i].Year == "未知" {
			if y := strings.SplitN(date, "-", 2); len(y) == 2 {
				cards[i].Year = y[0]
			} else if len(date) == 4 {
				cards[i].Year = date
			}
		}
		if cards[i].DurationText == "" && len(date) == 10 {
			if years := yearsSince(date); years > 0 {
				cards[i].DurationText = strconv.Itoa(years) + " 年"
			}
		}
	}
}

// yearsSince 计算从 date（YYYY-MM-DD）到现在满几年（未到周年不计）。
func yearsSince(date string) int {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return 0
	}
	now := time.Now()
	y := now.Year() - t.Year()
	if now.YearDay() < t.YearDay() {
		y--
	}
	if y < 0 {
		return 0
	}
	return y
}

// parseSavept 把 savept.icu 首页 HTML 解析为站点卡片 + 公告。
// savept.icu 用 data-* 属性标注每张卡片，结构稳定；此处按该契约正则抽取。
// 真实页面结构若有变动，参照站点选择器校准工作流（另存页面 → fixture）逐项校准。
func parseSavept(html string) ([]SaveptSiteCard, []SaveptAlert) {
	cards := extractCards(html)
	alerts := extractAlerts(html)
	applyOpenTimeOverrides(cards)
	// 抢救时长（来自病危公告）回填到匹配卡片
	for _, fd := range extractFaultDurations(alerts) {
		for i := range cards {
			if faultNameMatch(cards[i].Name, fd.name) {
				if cards[i].FaultDuration == "" {
					cards[i].FaultDuration = fd.duration
				}
				// 病危通知里的站点即便 status-strip 显示「运行中」也属病危（正在抢救），
				// 覆盖为 critical，使「病危」筛选能命中这些站点；不覆盖已关站。
				if cards[i].Status != HealthClosed {
					cards[i].Status = HealthCritical
				}
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
	tagRe = regexp.MustCompile(`<[^>]+>`)
)

func extractCards(html string) []SaveptSiteCard {
	var out []SaveptSiteCard
	for _, block := range cardBlockRe.FindAllString(html, -1) {
		rawStatus := attr(block, "data-status")
		// 真实 savept.icu 的 status-strip 文本为空，生命周期状态（运行中/已关站/故障中）
		// 写在 title 属性里；优先取文本，为空时回退 title。
		statusText := stripTags(extractInner(block, "status-strip"))
		if strings.TrimSpace(statusText) == "" {
			statusText = extractStatusStripTitle(block)
		}
		c := SaveptSiteCard{
			Name:            strings.TrimSpace(decodeEntities(attr(block, "data-name"))),
			URL:             attr(block, "href"),
			Year:            normalizeYear(attr(block, "data-year")),
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

// extractStatusStripTitle 取 status-strip 元素的 title 属性。真实 savept.icu 的
// status-strip 标签内文本为空，站点生命周期状态（运行中/已关站/故障中）写在 title 里。
func extractStatusStripTitle(block string) string {
	re := regexp.MustCompile(`(?is)<[^>]*class="[^"]*status-strip[^"]*"[^>]*>`)
	tag := re.FindString(block)
	if tag == "" {
		return ""
	}
	return attr(tag, "title")
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
		text = strings.TrimPrefix(text, rawTitle)
		text = strings.TrimLeft(text, "：:\t\n ")
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
	case containsAny(s, "病危", "抢救", "濒临", "故障", "critical", "danger"):
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
		return strings.ToLower(rawURL)
	}
	host := normalizeHost(u.Host)
	return host
}

// normalizeHost 域名归一：小写、去端口、去 www. 前缀。域名是 ptseek 合并、definitions
// 平台补全、本地 overrides 的 map 键，大小写或端口不归一会静默失配（详情/图标/去重全丢）。
func normalizeHost(host string) string {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimPrefix(host, "www.")
}

// faultNameMatch 病危公告站名 → 卡片匹配。子串包含会把「HD」匹配到 HDT/HDHome（实测误伤），
// 故要求精确匹配，或公告名是卡片名以分隔符边界开头的完整前缀（「HDT」匹配「HDT - 官网」，
// 不匹配「HDHome」）。
func faultNameMatch(cardName, alertName string) bool {
	a, b := strings.ToLower(strings.TrimSpace(cardName)), strings.ToLower(strings.TrimSpace(alertName))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if !strings.HasPrefix(a, b) {
		return false
	}
	rest := a[len(b):]
	// 前缀后须跟分隔符（空格/横线/括号等），排除「HD」前缀命中「HDHome」这种字母延续
	return isNameSep(rest[0])
}

// isNameSep 站点名的名称分隔符（ASCII + 中文间隔号/全角，>=0x80 交给多字节 UTF-8 判定）。
func isNameSep(c byte) bool {
	switch {
	case c == '-' || c == '_' || c == ' ' || c == '(' || c == '[' || c == '·':
		return true
	case c >= 0x80:
		return true
	}
	return false
}

// normalizeYear 归一建站年份：空或 "0000"（savept.icu 用 0000 标记未知年份）→ "未知"。
func normalizeYear(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "0000" {
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
