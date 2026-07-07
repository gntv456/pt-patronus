package site

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Extract 在上下文 selection 上按 ElementQuery 提取单个字符串。
//
// 支持回退选择器（q.Selectors，优先于 q.Selector）：按序逐个尝试，首个非空结果胜出。
// q.Parent=true 时，命中节点先爬到父节点再取文本/属性——为 font.color_* 锚点设计，
// 因为统计值是 label 字体的兄弟文本节点（如 <font class="color_uploaded">上传量:</font> 104.564 TB），
// 取父节点文本才能拿到「上传量: 104.564 TB」整串交由 filters 正则抽值。
func Extract(ctx *goquery.Selection, q ElementQuery) string {
	if ctx == nil || ctx.Length() == 0 {
		if q.Text != "" {
			return ApplyFilters(q.Text, q.Filters)
		}
		return ""
	}
	// Remove 在整个上下文上预先移除（与历史行为一致：广告/图标在 Find 前清掉）
	if q.Remove != "" {
		ctx.Find(q.Remove).Remove()
	}
	sels := q.Selectors
	if len(sels) == 0 && q.Selector != "" {
		sels = []string{q.Selector}
	}
	if len(sels) == 0 {
		if q.Text != "" && q.Data == "" {
			return ApplyFilters(q.Text, q.Filters)
		}
		// 无选择器：仅当带过滤链时才在整上下文上取值（"任意位置正则抽取"模式）；
		// 否则无可提取，返回空——避免无配置字段误返整页文本（详情页未映射字段 bug）。
		if len(q.Filters) == 0 {
			return ""
		}
		return readNode(ctx, q)
	}
	for _, sel := range sels {
		sub := findCompat(ctx, sel)
		if sel == ":self" {
			sub = ctx
		}
		if sub.Length() == 0 {
			continue
		}
		switch q.Attr {
		case "count":
			return ApplyFilters(strconv.Itoa(sub.Length()), q.Filters)
		case "text_all":
			return ApplyFilters(sub.Text(), q.Filters)
		case "html_all":
			html, _ := sub.Html()
			return ApplyFilters(html, q.Filters)
		}
		// 多匹配时只取首个（如多个 font.color_bonus，首个才是真魔力/积分）
		if got := readNode(scopeNode(sub.First(), q), q); got != "" {
			return got
		}
	}
	if q.Text != "" {
		return ApplyFilters(q.Text, q.Filters)
	}
	return ""
}

func findCompat(ctx *goquery.Selection, sel string) *goquery.Selection {
	if ctx == nil {
		return &goquery.Selection{}
	}
	if !needsCompatSelector(sel) {
		return ctx.Find(sel)
	}
	steps := splitSelectorSteps(sel)
	if len(steps) == 0 {
		return ctx.Find(sel)
	}
	cur := ctx
	for _, step := range steps {
		base, pseudo, idx := stripJQueryPseudo(step.selector)
		if base == "" {
			base = "*"
		}
		switch step.relation {
		case "child":
			cur = cur.ChildrenFiltered(base)
		case "adjacent":
			cur = cur.NextFiltered(base)
		case "sibling":
			cur = cur.NextAllFiltered(base)
		default:
			cur = cur.Find(base)
		}
		switch pseudo {
		case "first":
			cur = cur.First()
		case "last":
			cur = cur.Last()
		case "eq":
			cur = cur.Eq(idx)
		case "gt":
			if idx+1 >= cur.Length() {
				cur = cur.Slice(cur.Length(), cur.Length())
			} else {
				cur = cur.Slice(idx+1, cur.Length())
			}
		case "lt":
			if idx <= 0 {
				cur = cur.Slice(0, 0)
			} else if idx > cur.Length() {
				cur = cur.Slice(0, cur.Length())
			} else {
				cur = cur.Slice(0, idx)
			}
		}
		if cur.Length() == 0 {
			return cur
		}
	}
	return cur
}

func needsCompatSelector(sel string) bool {
	trimmed := strings.TrimSpace(sel)
	return strings.HasPrefix(trimmed, ">") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "~") ||
		strings.Contains(sel, ":eq(") || strings.Contains(sel, ":gt(") || strings.Contains(sel, ":lt(") ||
		strings.Contains(sel, ":first") || strings.Contains(sel, ":last")
}

type selectorStep struct {
	selector string
	relation string
}

func splitSelectorSteps(sel string) []selectorStep {
	var out []selectorStep
	var b strings.Builder
	relation := ""
	depth := 0
	var quote rune
	flush := func() {
		token := strings.TrimSpace(b.String())
		if token != "" {
			out = append(out, selectorStep{selector: token, relation: relation})
			relation = ""
		}
		b.Reset()
	}
	for _, r := range sel {
		if quote != 0 {
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			b.WriteRune(r)
		case '(', '[':
			depth++
			b.WriteRune(r)
		case ')', ']':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
		case '>':
			if depth == 0 {
				flush()
				relation = "child"
				continue
			}
			b.WriteRune(r)
		case '+':
			if depth == 0 {
				flush()
				relation = "adjacent"
				continue
			}
			b.WriteRune(r)
		case '~':
			if depth == 0 {
				flush()
				relation = "sibling"
				continue
			}
			b.WriteRune(r)
		case ' ', '\t', '\n', '\r':
			if depth == 0 {
				flush()
				continue
			}
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

func stripJQueryPseudo(sel string) (base, pseudo string, idx int) {
	sel = strings.TrimSpace(sel)
	if strings.HasSuffix(sel, ":first") {
		return strings.TrimSpace(strings.TrimSuffix(sel, ":first")), "first", 0
	}
	if strings.HasSuffix(sel, ":last") {
		return strings.TrimSpace(strings.TrimSuffix(sel, ":last")), "last", 0
	}
	if strings.HasSuffix(sel, ")") {
		if i := topLevelSuffixIndex(sel, ":eq("); i >= 0 {
			raw := strings.TrimSuffix(sel[i+4:], ")")
			n, _ := strconv.Atoi(strings.TrimSpace(raw))
			return strings.TrimSpace(sel[:i]), "eq", n
		}
		if i := topLevelSuffixIndex(sel, ":gt("); i >= 0 {
			raw := strings.TrimSuffix(sel[i+4:], ")")
			n, _ := strconv.Atoi(strings.TrimSpace(raw))
			return strings.TrimSpace(sel[:i]), "gt", n
		}
		if i := topLevelSuffixIndex(sel, ":lt("); i >= 0 {
			raw := strings.TrimSuffix(sel[i+4:], ")")
			n, _ := strconv.Atoi(strings.TrimSpace(raw))
			return strings.TrimSpace(sel[:i]), "lt", n
		}
	}
	return sel, "", 0
}

func topLevelSuffixIndex(sel, marker string) int {
	depth := 0
	var quote rune
	for i, r := range sel {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(', '[':
			if strings.HasPrefix(sel[i:], marker) && depth == 0 {
				return i
			}
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && strings.HasPrefix(sel[i:], marker) {
				return i
			}
		}
	}
	return -1
}

func scopeNode(s *goquery.Selection, q ElementQuery) *goquery.Selection {
	if s == nil || s.Length() == 0 {
		return s
	}
	if q.Ancestor != "" {
		scoped := s.Closest(q.Ancestor)
		if scoped.Length() == 0 {
			return scoped
		}
		s = scoped
	}
	if q.Find != "" {
		return s.Find(q.Find).First()
	}
	return s
}

// readNode 在单个命中节点（已 First）上按 Parent/Attr/Filters 取值。
func readNode(s *goquery.Selection, q ElementQuery) string {
	if s == nil || s.Length() == 0 {
		return ""
	}
	if q.Parent {
		s = s.Parent()
		if s.Length() == 0 {
			return ""
		}
	}
	var raw string
	if q.Data != "" {
		v, ok := s.Attr(dataAttrName(q.Data))
		if !ok {
			return ""
		}
		return ApplyFilters(v, q.Filters)
	}
	switch q.Attr {
	case "", "text":
		raw = s.Text()
	case "html":
		raw, _ = s.Html()
	case "title_or_text":
		raw = firstAttrOrText(s, "title")
	case "datetime_or_title_or_text":
		raw = firstAttrOrText(s, "datetime", "title")
	default:
		v, ok := s.Attr(q.Attr)
		if !ok {
			return ""
		}
		raw = v
	}
	return ApplyFilters(raw, q.Filters)
}

func firstAttrOrText(s *goquery.Selection, attrs ...string) string {
	for _, attr := range attrs {
		if v, ok := s.Attr(attr); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return s.Text()
}

func dataAttrName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "data-") {
		return name
	}
	var b strings.Builder
	b.WriteString("data-")
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		if r == '_' {
			b.WriteByte('-')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ApplyFilters 顺序应用过滤链。
func ApplyFilters(raw string, fs []Filter) string {
	s := strings.TrimSpace(raw)
	for _, f := range fs {
		switch f.Name {
		case "regexp":
			// args 形如 "pattern:::N"：取第 N 个捕获组（默认 1；0 取全匹配）
			pat := f.Args
			groupIdx := 1
			if i := strings.Index(pat, ":::"); i >= 0 {
				if gi, err := strconv.Atoi(pat[i+3:]); err == nil && gi >= 0 {
					groupIdx = gi
					pat = pat[:i]
				}
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			m := re.FindStringSubmatch(s)
			switch {
			case len(m) == 0:
				// 不匹配：regexp 的职责就是抽出特定模式，未命中即「无可提取」→ 返回空。
				// 不能保留原值——否则下游 ToFloat64/ParseBytes 会从整段父节点文本里误抓无关数字
				// （典型：分享率显示 "---"/"无限" 时 ratio 正则失配，却把魔力值 18889 当成分享率）。
				return ""
			case groupIdx == 0:
				s = m[0]
			case len(m) > groupIdx:
				s = m[groupIdx]
			case len(m) > 1:
				s = m[1] // 组号越界：回退组 1
			default:
				s = m[0]
			}
		case "replace":
			parts := strings.SplitN(f.Args, ":::", 2)
			if len(parts) == 2 {
				s = strings.ReplaceAll(s, parts[0], parts[1])
			}
		case "map":
			for _, entry := range strings.Split(f.Args, ":::") {
				parts := strings.SplitN(entry, "=", 2)
				if len(parts) == 2 && s == parts[0] {
					s = parts[1]
					break
				}
			}
		case "contains_map":
			for _, entry := range strings.Split(f.Args, ":::") {
				parts := strings.SplitN(entry, "=", 2)
				if len(parts) == 2 && strings.Contains(s, parts[0]) {
					s = parts[1]
					break
				}
			}
		case "prepend":
			s = f.Args + s
		case "append":
			s += f.Args
		case "const":
			s = f.Args
		case "json_format":
			if formatted := formatJSONTemplate(s, f.Args); formatted != "" {
				s = formatted
			}
		case "split":
			parts := strings.SplitN(f.Args, ":::", 2)
			if len(parts) == 2 {
				if idx, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					chunks := strings.Split(s, parts[0])
					if idx >= 0 && idx < len(chunks) {
						s = chunks[idx]
					}
				}
			}
		case "trim":
			s = strings.TrimSpace(s)
		case "collapse_space":
			s = strings.Join(strings.Fields(s), " ")
		case "collapse_join":
			sep := f.Args
			if sep == "" {
				sep = " "
			}
			s = strings.Join(strings.Fields(s), sep)
		case "strip_html":
			s = stripHTMLText(s)
		case "html_line":
			idx := 0
			if n, err := strconv.Atoi(strings.TrimSpace(f.Args)); err == nil && n >= 0 {
				idx = n
			}
			parts := htmlBreakRe.Split(s, -1)
			if idx < len(parts) {
				s = stripHTMLText(parts[idx])
			} else {
				s = ""
			}
		case "lower":
			s = strings.ToLower(s)
		case "upper":
			s = strings.ToUpper(s)
		case "title_case":
			s = titleCaseWords(s)
		case "url_decode":
			if decoded, err := url.QueryUnescape(s); err == nil {
				s = decoded
			}
		case "parse_number":
			s = extractNumber(s)
		case "bool", "parse_bool":
			if parseBoolish(s) {
				s = "true"
			} else {
				s = "false"
			}
		case "clean_avatar":
			s = cleanAvatarValue(s)
		case "claim_count":
			count, _, ok := ParseClaimPair(s)
			if !ok {
				return ""
			}
			s = strconv.Itoa(count)
		case "claim_max":
			_, max, ok := ParseClaimPair(s)
			if !ok {
				return ""
			}
			s = strconv.Itoa(max)
		case "divide":
			if v, err := strconv.ParseFloat(strings.ReplaceAll(extractNumber(s), ",", ""), 64); err == nil {
				if d, err := strconv.ParseFloat(strings.TrimSpace(f.Args), 64); err == nil && d != 0 {
					s = strconv.FormatFloat(v/d, 'f', -1, 64)
				}
			}
		case "multiply":
			if v, err := strconv.ParseFloat(strings.ReplaceAll(extractNumber(s), ",", ""), 64); err == nil {
				if m, err := strconv.ParseFloat(strings.TrimSpace(f.Args), 64); err == nil {
					s = strconv.FormatFloat(v*m, 'f', -1, 64)
				}
			}
		case "add":
			if v, err := strconv.ParseFloat(strings.ReplaceAll(extractNumber(s), ",", ""), 64); err == nil {
				if delta, err := strconv.ParseFloat(strings.TrimSpace(f.Args), 64); err == nil {
					s = strconv.FormatFloat(v+delta, 'f', -1, 64)
				}
			}
		case "parse_size":
			// "1.5 TB"/"512 MB" 等转字节数字符串（复用 ParseBytes）。
			s = strconv.FormatInt(ParseBytes(s), 10)
		case "last_size":
			s = strconv.FormatInt(LastBytes(s), 10)
		case "sum_size":
			s = strconv.FormatInt(SumBytes(s), 10)
		case "sum_matches":
			pat := f.Args
			groupIdx := 1
			if i := strings.Index(pat, ":::"); i >= 0 {
				if gi, err := strconv.Atoi(pat[i+3:]); err == nil && gi >= 0 {
					groupIdx = gi
					pat = pat[:i]
				}
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			var total float64
			for _, m := range re.FindAllStringSubmatch(s, -1) {
				if len(m) <= groupIdx {
					continue
				}
				if v, err := strconv.ParseFloat(strings.ReplaceAll(m[groupIdx], ",", ""), 64); err == nil {
					total += v
				}
			}
			s = strconv.FormatFloat(total, 'f', -1, 64)
		case "queryparam":
			// args=参数名；从 s（URL/含 query 串）取该参数值（替代 PT-depiler querystring filter）。
			s = queryParam(s, f.Args)
		case "parse_time":
			// 解析时间串为 YYYY-MM-DD；支持 ISO/常见绝对格式与相对时间，无法识别保留原值。
			if t := parseTimeBestEffort(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "dmy_hms":
			if t := parseDMYHMS(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "unix_seconds":
			if v, err := strconv.ParseInt(extractNumber(s), 10, 64); err == nil && v > 0 {
				s = time.Unix(v, 0).Format("2006-01-02")
			}
		case "unix_millis":
			if v, err := strconv.ParseInt(extractNumber(s), 10, 64); err == nil && v > 0 {
				s = time.UnixMilli(v).Format("2006-01-02")
			}
		case "hdbits_title":
			if title := hdbitsTitle(s); title != "" {
				s = title
			}
		case "karagarga_subtitle":
			s = karagargaSubtitle(s)
		case "retroflix_subtitle":
			s = retroflixSubtitle(s)
		case "bitpt_subtitle":
			s = bitptSubtitle(s)
		case "hdhome_bonus_per_hour":
			s = formatFloatFilter(hdhomeBonusPerHour(s))
		case "hdfans_bonus_per_hour":
			s = formatFloatFilter(labelledBonusPerHour(s))
		case "hdsky_bonus_per_hour":
			s = formatFloatFilter(labelledBonusPerHour(s))
		case "hhanclub_rescue_bonus_per_hour":
			s = formatFloatFilter(hhanclubRescueBonusPerHour(s))
		case "pter_upload_search_count":
			s = strconv.Itoa(pterUploadSearchCount(s))
		case "anisource_time":
			if t := anisourceTime(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "btetree_time":
			if t := btetreeTime(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		case "myanonamouse_seeding":
			s = strconv.Itoa(myanonamouseCountSum(s, "seedUnsat", "seedHnr", "sSat", "upAct"))
		case "myanonamouse_seeding_size":
			s = strconv.FormatInt(myanonamouseSizeSum(s, "seedUnsat", "seedHnr", "sSat", "upAct"), 10)
		case "myanonamouse_uploads":
			s = strconv.Itoa(myanonamouseCountSum(s, "upAct", "upInact"))
		}
	}
	return strings.TrimSpace(s)
}

var htmlBreakRe = regexp.MustCompile(`(?i)<br\s*/?>`)
var htmlTagTokenRe = regexp.MustCompile(`<[^>]+>`)

func stripHTMLText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + s + "</div>"))
	if err != nil {
		return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
	}
	return strings.Join(strings.Fields(doc.Text()), " ")
}

func hdbitsTitle(raw string) string {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var node map[string]any
	if err := dec.Decode(&node); err != nil {
		return ""
	}
	name := jsonStringify(node["name"])
	filename := strings.TrimSuffix(jsonStringify(node["filename"]), ".torrent")
	typeCategory := fmt.Sprint(node["type_category"])
	typeMedium := fmt.Sprint(node["type_medium"])
	if typeCategory != "7" && typeMedium != "1" && filename != "" {
		return filename
	}
	return name
}

func karagargaSubtitle(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table><tbody><tr>" + raw + "</tr></tbody></table>"))
	if err != nil {
		return ""
	}
	row := doc.Find("tr").First()
	cell := func(i int) string {
		return strings.Join(strings.Fields(row.Find("td:nth-child("+strconv.Itoa(i)+")").Text()), " ")
	}
	parts := []string{
		cell(3),
		cell(4),
		strings.Join(strings.Fields(row.Find(`a[href^="browse.php?country="] img`).AttrOr("title", "")), " "),
		cell(5),
		strings.Join(strings.Fields(row.Find("td:nth-child(2) span").Text()), " "),
	}
	return joinNonEmpty(parts, " / ")
}

func retroflixSubtitle(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + raw + "</div>"))
	if err != nil {
		return ""
	}
	var genres []string
	doc.Find(`div[class]:contains("Genre:") > a[href^="/browse"]`).Each(func(_ int, s *goquery.Selection) {
		if txt := strings.Join(strings.Fields(s.Text()), " "); txt != "" {
			genres = append(genres, txt)
		}
	})
	var props []string
	doc.Find(`a[href^="/browse/t/"]`).First().NextAllFiltered("a").Each(func(_ int, s *goquery.Selection) {
		if txt := strings.Join(strings.Fields(s.Text()), " "); txt != "" {
			props = append(props, txt)
		}
	})
	parts := append(genres, props...)
	out := joinNonEmpty(parts, " / ")
	if out != "" && len(genres) > 0 {
		return "Genres: " + out
	}
	return out
}

func bitptSubtitle(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + raw + "</div>"))
	if err != nil {
		return ""
	}
	var parts []string
	doc.Find("span.t_t_d, span.t_t_d_n").Each(func(_ int, s *goquery.Selection) {
		txt := strings.TrimSpace(strings.ReplaceAll(s.Text(), "**", ""))
		txt = strings.Trim(txt, "* \t\r\n")
		if txt = strings.Join(strings.Fields(txt), " "); txt != "" {
			parts = append(parts, txt)
		}
	})
	return strings.Join(parts, " ")
}

func hdhomeBonusPerHour(raw string) float64 {
	nums := numericTokens(raw)
	if len(nums) == 0 {
		return 0
	}
	if len(nums) == 10 {
		return nums[0] + nums[5] + nums[8]
	}
	return nums[0]
}

var labelledBonusPerHourRe = regexp.MustCompile("(?i)(?:\u6bcf\u5c0f\u65f6\u83b7\u5f97\u7684\u5408\u8ba1\u9b54\u529b\u503c|\u6bcf\u5c0f\u6642\u7372\u5f97\u7684\u5408\u8a08\u9b54\u529b\u503c|\u4f60\u5f53\u524d\u6bcf\u5c0f\u65f6\u80fd\u83b7\u53d6|\u4f60\u7576\u524d\u6bcf\u5c0f\u6642\u80fd\u7372\u53d6|currently\\s+getting)[^0-9+.-]{0,1000}([-+]?[\\d,]+\\.?\\d*)")

func labelledBonusPerHour(raw string) float64 {
	raw = strings.ReplaceAll(raw, ",", "")
	if m := labelledBonusPerHourRe.FindStringSubmatch(raw); len(m) > 1 {
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			return f
		}
	}
	nums := numericTokens(raw)
	if len(nums) == 0 {
		return 0
	}
	return nums[0]
}

func hhanclubRescueBonusPerHour(raw string) float64 {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return 0
	}
	var out float64
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		rowHTML, _ := row.Html()
		if !strings.Contains(rowHTML, "userdetails.php") {
			return
		}
		cells := row.Find("td")
		if cells.Length() < 6 {
			return
		}
		cell := cells.Eq(5)
		text := cell.Text()
		if strings.TrimSpace(text) == "" {
			text, _ = cell.Html()
		}
		value := ToFloat64(extractNumber(text))
		if value > 0 {
			out = math.Round((value/24)*1000) / 1000
		}
	})
	return out
}

func pterUploadSearchCount(raw string) int {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return 0
	}
	text := strings.TrimSpace(doc.Text())
	if regexp.MustCompile(`(?i)No record|没有种子|没有记录|沒有記錄`).MatchString(text) {
		return 0
	}
	pager := doc.Find("p.np-pager").First().Find("b").Last()
	if pager.Length() > 0 {
		if n := ToInt(pager.Text()); n > 0 {
			return n
		}
	}
	table := doc.Find("table.torrents").Last()
	if table.Length() == 0 {
		return 0
	}
	rows := table.Find("tbody > tr")
	if rows.Length() == 0 {
		rows = table.Find("tr")
	}
	if rows.Length() <= 1 {
		return 0
	}
	return rows.Length() - 1
}

func numericTokens(raw string) []float64 {
	cleaned := html.UnescapeString(htmlTagTokenRe.ReplaceAllString(raw, " "))
	matches := numRe.FindAllString(strings.ReplaceAll(cleaned, ",", ""), -1)
	out := make([]float64, 0, len(matches))
	for _, m := range matches {
		f, err := strconv.ParseFloat(m, 64)
		if err != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

func formatFloatFilter(f float64) string {
	if f <= 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func joinNonEmpty(parts []string, sep string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func titleCaseWords(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, word := range words {
		if word == "vr" {
			words[i] = "VR"
			continue
		}
		runes := []rune(word)
		if len(runes) == 0 {
			continue
		}
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

var jsonTemplateFieldRe = regexp.MustCompile(`\{([^{}]+)\}`)

func formatJSONTemplate(raw, tmpl string) string {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(tmpl) == "" {
		return ""
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var node any
	if err := dec.Decode(&node); err != nil {
		return ""
	}
	missing := false
	out := jsonTemplateFieldRe.ReplaceAllStringFunc(tmpl, func(token string) string {
		key := strings.Trim(token, "{}")
		got := jsonStringify(jsonNavigate(node, key))
		if got == "" {
			missing = true
		}
		return got
	})
	if missing {
		return ""
	}
	return out
}

var numRe = regexp.MustCompile(`[-+]?[\d,]*\.?\d+`)
var (
	claimPairLabeledRe = regexp.MustCompile(`(?i)(?:认领|認領|Claim)[^0-9]{0,80}(\d[\d,]*)\s*/\s*(\d[\d,]*)`)
	claimPairRe        = regexp.MustCompile(`(\d[\d,]*)\s*/\s*(\d[\d,]*)`)
)

// extractNumber 抽取首个数字串（去千分位逗号）。
func extractNumber(s string) string {
	m := numRe.FindString(s)
	if m == "" {
		return ""
	}
	return strings.ReplaceAll(m, ",", "")
}

// ToInt 把字符串解析为 int（失败返回 0）。
func ToInt(s string) int {
	n, _ := strconv.Atoi(extractNumber(s))
	return n
}

// ToFloat64 解析为 float64。
func ToFloat64(s string) float64 {
	f, _ := strconv.ParseFloat(extractNumber(s), 64)
	return f
}

func ToBool(s string) bool {
	return parseBoolish(s)
}

func parseBoolish(s string) bool {
	normalized := strings.TrimSpace(strings.ToLower(s))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "1", "true", "yes", "y", "on", "donor", "vip", "贵宾", "捐赠者", "捐贈者":
		return true
	case "0", "false", "no", "n", "off", "none", "null", "nil", "-":
		return false
	}
	return strings.Contains(normalized, "donor") ||
		strings.Contains(normalized, "vip") ||
		strings.Contains(normalized, "捐赠") ||
		strings.Contains(normalized, "捐贈")
}

func cleanAvatarValue(s string) string {
	s = strings.TrimSpace(html.UnescapeString(s))
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "default_avatar") || strings.Contains(lower, "defaultavatar") {
		return ""
	}
	return s
}

func ParseClaimPair(s string) (count, max int, ok bool) {
	s = strings.Join(strings.Fields(s), " ")
	m := claimPairLabeledRe.FindStringSubmatch(s)
	if len(m) < 3 {
		m = claimPairRe.FindStringSubmatch(s)
	}
	if len(m) < 3 {
		return 0, 0, false
	}
	count = ToInt(m[1])
	max = ToInt(m[2])
	return count, max, count > 0 || max > 0
}

var bytesRe = regexp.MustCompile(`(?i)([\d.,]+)\s*([KMGTPE]?i?B?)`)

// ParseBytes 把 "12.4 TB" / "1,234 GB" / "512 MB" / "5 GiB" 解析为字节数（int64）。
// 兼容二进制单位 TiB/GiB（统一去 i 归一到 TB/GB…）。
func ParseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	m := bytesRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	val, err := strconv.ParseFloat(extractNumber(m[1]), 64)
	if err != nil {
		return 0
	}
	// 归一单位：TiB→TB、GiB→GB（去掉二进制标记 i）
	unit := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(m[2])), "I", "")
	var mult float64 = 1
	switch unit {
	case "K", "KB":
		mult = 1 << 10
	case "M", "MB":
		mult = 1 << 20
	case "G", "GB":
		mult = 1 << 30
	case "T", "TB":
		mult = 1 << 40
	case "P", "PB":
		mult = 1 << 50
	case "E", "EB":
		mult = 1 << 60
	}
	return int64(val * mult)
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// SumBytes sums all recognizable size tokens in a string.
func SumBytes(s string) int64 {
	var total int64
	for _, m := range bytesRe.FindAllStringSubmatch(s, -1) {
		if len(m) < 3 {
			continue
		}
		total += ParseBytes(m[0])
	}
	return total
}

// LastBytes returns the final recognizable size token in a string.
func LastBytes(s string) int64 {
	matches := bytesRe.FindAllStringSubmatch(s, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if len(matches[i]) >= 3 {
			return ParseBytes(matches[i][0])
		}
	}
	return 0
}

// queryParam 从 s（URL 或含 query 的串）取指定参数名首个值。
// 先走 url.Parse（标准 URL），取空或失败再正则兜底（[?&]name=value）。
// 取不到则原样返回 s（供后续 filter 继续处理）。
func queryParam(s, name string) string {
	if q, err := url.Parse(s); err == nil {
		if v := q.Query().Get(name); v != "" {
			return v
		}
	}
	re := regexp.MustCompile(`[?&]` + regexp.QuoteMeta(name) + `=([^&#]+)`)
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return s
}

// absTimeFormats 常见绝对时间格式（中英文 PT 站 usercp/userdetails 页）。
var absTimeFormats = []string{
	"2006-01-02 15:04:05",
	"2006-01-0215:04:05",
	"2006-01-0215:04:05 -07:00",
	"2006-01-02 15:04 -0700",
	"2006-01-02 15:04 -07:00",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
	"02/01/2006 15:04:05",
	"02/01/2006",
	"2006年1月2日 15:04:05",
	"02-01-2006, 15:04:05",
	"02-01-2006 15:04:05",
	"02_01_2006",
	"2006.01.02",
	time.RFC3339,
	"January 02, 2006, 15:04:05",
	"January 2, 2006, 15:04:05",
	"January 02, 2006, 15:04",
	"January 2, 2006, 15:04",
	"Jan 02 2006, 15:04",
	"Jan 2 2006, 15:04",
	"Jan 2, 2006",
	"2 Jan 2006",
	"15:04 02-Jan-2006",
	"010206 15:04:05",
}

var anisourceTimeRe = regexp.MustCompile(`(?i)Date:\s*(\d{4}-\d{2}-\d{2})\s+at\s+(\d{1,2}:\d{2})`)

func anisourceTime(raw string) time.Time {
	m := anisourceTimeRe.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) != 3 {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02 15:04 -0700", m[1]+" "+m[2]+" -0900")
	if err != nil {
		return time.Time{}
	}
	return t
}

var btetreeTorrentIDRe = regexp.MustCompile(`(?i)torrentId=(\d+)`)

var btetreeYearThresholds = [][2]int{
	{2004, 2204},
	{2005, 10368},
	{2006, 21306},
	{2007, 502958},
	{2008, 512029},
	{2009, 521346},
	{2010, 531218},
	{2011, 541379},
	{2012, 551321},
	{2013, 561096},
	{2014, 569309},
	{2015, 577205},
	{2016, 584158},
	{2017, 590609},
	{2018, 597003},
	{2019, 603585},
	{2020, 608148},
	{2021, 611609},
	{2022, 614780},
	{2023, 617776},
	{2024, 621061},
	{2025, 623102},
	{2026, 625197},
}

func btetreeTime(raw string) time.Time {
	m := btetreeTorrentIDRe.FindStringSubmatch(raw)
	if len(m) != 2 {
		return time.Time{}
	}
	torrentID, _ := strconv.Atoi(m[1])
	year := 2003
	for _, threshold := range btetreeYearThresholds {
		if torrentID < threshold[1] {
			break
		}
		year = threshold[0]
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table><tbody><tr>" + raw + "</tr></tbody></table>"))
	if err != nil {
		return time.Time{}
	}
	mdhm := strings.TrimSpace(doc.Find("td:nth-child(5)").First().Text())
	if mdhm == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006/01/02 15:04", fmt.Sprintf("%04d/%s", year, mdhm))
	if err != nil {
		return time.Time{}
	}
	return t
}

type myanonamouseLoadItem struct {
	Count int         `json:"count"`
	Size  json.Number `json:"size"`
}

func myanonamouseLoadMap(raw string) map[string]myanonamouseLoadItem {
	var nodes map[string]json.RawMessage
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&nodes); err != nil {
		return nil
	}
	out := make(map[string]myanonamouseLoadItem, len(nodes))
	for key, body := range nodes {
		var item myanonamouseLoadItem
		dec := json.NewDecoder(strings.NewReader(string(body)))
		dec.UseNumber()
		if err := dec.Decode(&item); err != nil {
			continue
		}
		out[key] = item
	}
	return out
}

func myanonamouseCountSum(raw string, keys ...string) int {
	items := myanonamouseLoadMap(raw)
	var total int
	for _, key := range keys {
		total += items[key].Count
	}
	return total
}

func myanonamouseSizeSum(raw string, keys ...string) int64 {
	items := myanonamouseLoadMap(raw)
	var total int64
	for _, key := range keys {
		if size, err := items[key].Size.Int64(); err == nil {
			total += size
		}
	}
	return total
}

// relativeTimeTokenRe 英文相对时间片段：N min(s)/hour(s)/day(s)/week(s)。
// 支持 "1.5 hours ago"、"1 hour and 50 minutes ago" 等 PT-depiler parseTTL 常见输出。
var relativeTimeTokenRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(seconds?|secs?|minutes?|mins?|hours?|days?|weeks?)`)

// cnRelativeTimeRe 中文相对时间：N 天/小时/分钟/周 前。
var cnRelativeTimeRe = regexp.MustCompile(`(\d+)\s*(天|小时|小時|分钟|分鐘|周|週)前`)

// parseTimeBestEffort 尽力解析时间串：先试相对时间（N mins ago / N分钟前，相对 now），
// 再试常见绝对格式。无法识别返回零值（调用方据此保留原值）。
func parseTimeBestEffort(s string) time.Time {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Date/time submitted:")
	s = strings.TrimSpace(s)
	if t, ok := parseTodayYesterdayAt(s); ok {
		return t
	}
	if t, ok := parseRelativeTime(s); ok {
		return t
	}
	for _, layout := range absTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseTodayYesterdayAt(s string) (time.Time, bool) {
	m := todayYesterdayAtRe.FindStringSubmatch(s)
	if len(m) != 5 {
		return time.Time{}, false
	}
	hh, _ := strconv.Atoi(m[2])
	mm, _ := strconv.Atoi(m[3])
	ss := 0
	if m[4] != "" {
		ss, _ = strconv.Atoi(m[4])
	}
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, ss, 0, now.Location())
	if strings.EqualFold(m[1], "yesterday") {
		day = day.AddDate(0, 0, -1)
	}
	return day, true
}

// parseRelativeTime 解析相对时间串，相对 time.Now()。匹配返回 (时刻, true)。
var dmyHMSRe = regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2})\s*(\d{2})/(\d{2})/(\d{4})`)
var todayYesterdayAtRe = regexp.MustCompile(`(?i)^(today|yesterday)\s+(?:at\s+)?(\d{1,2}):(\d{2})(?::(\d{2}))?$`)

func parseDMYHMS(s string) time.Time {
	m := dmyHMSRe.FindStringSubmatch(s)
	if len(m) != 7 {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02 15:04:05", m[6]+"-"+m[5]+"-"+m[4]+" "+m[1]+":"+m[2]+":"+m[3])
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseRelativeTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	now := time.Now()
	if m := cnRelativeTimeRe.FindStringSubmatch(s); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "天":
			return now.AddDate(0, 0, -n), true
		case "小时", "小時":
			return now.Add(-time.Duration(n) * time.Hour), true
		case "分钟", "分鐘":
			return now.Add(-time.Duration(n) * time.Minute), true
		case "周", "週":
			return now.AddDate(0, 0, -7*n), true
		}
	}
	matches := relativeTimeTokenRe.FindAllStringSubmatch(s, -1)
	if len(matches) > 0 && (strings.Contains(strings.ToLower(s), "ago") || len(matches[0][0]) == len(s)) {
		var dur time.Duration
		for _, m := range matches {
			n, _ := strconv.ParseFloat(m[1], 64)
			unit := strings.ToLower(m[2])
			switch {
			case strings.HasPrefix(unit, "sec"):
				dur += time.Duration(n * float64(time.Second))
			case strings.HasPrefix(unit, "min"):
				dur += time.Duration(n * float64(time.Minute))
			case strings.HasPrefix(unit, "hour"):
				dur += time.Duration(n * float64(time.Hour))
			case strings.HasPrefix(unit, "day"):
				dur += time.Duration(n * float64(24*time.Hour))
			case strings.HasPrefix(unit, "week"):
				dur += time.Duration(n * float64(7*24*time.Hour))
			}
		}
		if dur > 0 {
			return now.Add(-dur), true
		}
	}
	return time.Time{}, false
}
