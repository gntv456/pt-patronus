package site

import (
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
		// 无选择器：仅当带过滤链时才在整上下文上取值（"任意位置正则抽取"模式）；
		// 否则无可提取，返回空——避免无配置字段误返整页文本（详情页未映射字段 bug）。
		if len(q.Filters) == 0 {
			return ""
		}
		return readNode(ctx, q)
	}
	for _, sel := range sels {
		sub := ctx.Find(sel)
		if sub.Length() == 0 {
			continue
		}
		// 多匹配时只取首个（如多个 font.color_bonus，首个才是真魔力/积分）
		if got := readNode(sub.First(), q); got != "" {
			return got
		}
	}
	return ""
}

// readNode 在单个命中节点（已 First）上按 Parent/Attr/Filters 取值。
func readNode(s *goquery.Selection, q ElementQuery) string {
	if q.Parent {
		s = s.Parent()
		if s.Length() == 0 {
			return ""
		}
	}
	var raw string
	switch q.Attr {
	case "", "text":
		raw = s.Text()
	case "html":
		raw, _ = s.Html()
	default:
		v, ok := s.Attr(q.Attr)
		if !ok {
			return ""
		}
		raw = v
	}
	return ApplyFilters(raw, q.Filters)
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
		case "trim":
			s = strings.TrimSpace(s)
		case "lower":
			s = strings.ToLower(s)
		case "upper":
			s = strings.ToUpper(s)
		case "parse_number":
			s = extractNumber(s)
		case "parse_size":
			// "1.5 TB"/"512 MB" 等转字节数字符串（复用 ParseBytes）。
			s = strconv.FormatInt(ParseBytes(s), 10)
		case "queryparam":
			// args=参数名；从 s（URL/含 query 串）取该参数值（替代 PT-depiler querystring filter）。
			s = queryParam(s, f.Args)
		case "parse_time":
			// 解析时间串为 YYYY-MM-DD；支持 ISO/常见绝对格式与相对时间，无法识别保留原值。
			if t := parseTimeBestEffort(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		}
	}
	return strings.TrimSpace(s)
}

var numRe = regexp.MustCompile(`[-+]?[\d,]*\.?\d+`)

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
	case "KB":
		mult = 1 << 10
	case "MB":
		mult = 1 << 20
	case "GB":
		mult = 1 << 30
	case "TB":
		mult = 1 << 40
	case "PB":
		mult = 1 << 50
	}
	return int64(val * mult)
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
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
	"2006.01.02",
	time.RFC3339,
	"Jan 2, 2006",
	"2 Jan 2006",
}

// relativeTimeRe 英文相对时间：N min(s)/hour(s)/day(s)/week(s) [ago]。
var relativeTimeRe = regexp.MustCompile(`(?i)^(\d+)\s*(min(?:ute)?s?|hours?|days?|weeks?)\s*(?:ago)?$`)

// cnRelativeTimeRe 中文相对时间：N 天/小时/分钟/周 前。
var cnRelativeTimeRe = regexp.MustCompile(`(\d+)\s*(天|小时|小時|分钟|分鐘|周|週)前`)

// parseTimeBestEffort 尽力解析时间串：先试相对时间（N mins ago / N分钟前，相对 now），
// 再试常见绝对格式。无法识别返回零值（调用方据此保留原值）。
func parseTimeBestEffort(s string) time.Time {
	s = strings.TrimSpace(s)
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

// parseRelativeTime 解析相对时间串，相对 time.Now()。匹配返回 (时刻, true)。
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
	if m := relativeTimeRe.FindStringSubmatch(s); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		switch {
		case strings.HasPrefix(strings.ToLower(m[2]), "min"):
			return now.Add(-time.Duration(n) * time.Minute), true
		case strings.HasPrefix(strings.ToLower(m[2]), "hour"):
			return now.Add(-time.Duration(n) * time.Hour), true
		case strings.HasPrefix(strings.ToLower(m[2]), "day"):
			return now.AddDate(0, 0, -n), true
		case strings.HasPrefix(strings.ToLower(m[2]), "week"):
			return now.AddDate(0, 0, -7*n), true
		}
	}
	return time.Time{}, false
}
