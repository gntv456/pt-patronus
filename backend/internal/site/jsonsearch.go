package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// jsonNavigate 沿点分路径遍历已解码的 JSON 节点（any）。
// 每段：map[string]any 按键取；[]any 按数字段索引。缺键/越界 → nil。
// 支持 data / torrentList / data.torrents（嵌套）/ data.results 等 PT-depiler JSON 路径。
func jsonNavigate(node any, path string) any {
	if path == "" {
		return node
	}
	cur := node
	for _, seg := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}
			cur = v[i]
		default:
			return nil
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}

// jsonStringify 把导航到的标量节点转为字符串。配合 json.Decoder.UseNumber()，
// 数字以 json.Number（字符串原样）返回，避免大字节数被 float64 科学计数法破坏。
func jsonStringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return string(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		if b, err := json.Marshal(x); err == nil {
			return string(b)
		}
		return fmt.Sprint(x)
	}
}

var jsonSearchSizeUnitRe = regexp.MustCompile(`(?i)(?:^|[\d\s.,])(?:bytes?|b|[kmgtpe]i?b?)\b`)

func normalizeJSONSearchSize(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || jsonSearchSizeUnitRe.MatchString(s) {
		return s
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil || n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return s
	}
	return FormatBytes(int64(n))
}

// LooksLikeJSON 报告 body 首个非空白字节是否为 { 或 [（GenericHTML 据此嗅探 HTML/JSON）。
func LooksLikeJSON(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}

// PathPlaceholderUsed 报告 search_entry 是否用路径占位符（关键字代入路径而非 query 参数）。
// 约定：结尾 /query（converter 为 torrentleech 保留）或含 {keyword}（手写）。
func PathPlaceholderUsed(entry string) bool {
	return strings.HasSuffix(entry, "/query") || strings.Contains(entry, "{keyword}")
}

// BuildJSONSearchURL 构造 JSON-API 搜索 URL。
// search_entry 用路径占位符（/query 或 {keyword}）→ 关键字代入路径；否则 ?search_param=kw。
func BuildJSONSearchURL(baseURL string, def *SiteMetadata, keyword string, page int) string {
	entry := def.SearchEntry
	if entry == "" {
		entry = "torrents.php"
	}
	placeholder := PathPlaceholderUsed(entry)
	if placeholder {
		kw := strings.TrimSpace(keyword)
		switch {
		case strings.HasSuffix(entry, "/query"):
			entry = strings.TrimSuffix(entry, "/query")
			if kw != "" {
				entry += "/" + url.PathEscape(kw)
			}
		default: // {keyword}
			entry = strings.ReplaceAll(entry, "{keyword}", url.PathEscape(kw))
		}
	}
	u := searchURLBase(baseURL, entry)
	parsed, err := url.Parse(u)
	if err != nil {
		q := url.Values{}
		if !placeholder {
			param := def.SearchParam
			if param == "" {
				param = "search"
			}
			q.Set(param, keyword)
		}
		if encoded := q.Encode(); encoded != "" {
			u += "?" + encoded
		}
		return u
	}
	q := parsed.Query()
	if !placeholder {
		param := def.SearchParam
		if param == "" {
			param = "search"
		}
		q.Set(param, keyword)
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	for k, v := range def.ExtraParam {
		q.Set(k, v)
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// ParseJSONSearch 纯函数：按 def 选择器从 JSON 响应解析种子列表。
// 镜像 ParseSearch（HTML）的语义，但行/字段用 JSON 点分路径而非 CSS。
// 支持 selector 单数与 selectors 复数回退（镜像 Extract）；:self 取整行 stringify；
// 抽到值过 ApplyFilters（parse_size/parse_time/regexp/replace 等复用）。
// 数字用 UseNumber 保留原样，避免大字节科学计数法。
func ParseJSONSearch(body []byte, def *SiteMetadata, baseURL string) []Torrent {
	if def == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil
	}
	rowSelectors := def.Selectors.Rows.Selectors
	if len(rowSelectors) == 0 && def.Selectors.Rows.Selector != "" {
		rowSelectors = []string{def.Selectors.Rows.Selector}
	}
	var rows []any
	for _, rowSelector := range rowSelectors {
		var rowsNode any
		if rowSelector == ":self" {
			rowsNode = root
		} else {
			rowsNode = jsonNavigate(root, rowSelector)
		}
		rows, _ = rowsNode.([]any)
		if len(rows) > 0 {
			break
		}
	}
	if rows == nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	out := []Torrent{}
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			continue
		}
		// get 按字段选择器抽值：selectors 复数回退优先，其次 selector 单数；首个非空胜出。
		get := func(f ElementQuery) string {
			if f.Data != "" {
				if v := jsonNavigate(row, f.Data); v != nil {
					return ApplyFilters(jsonStringify(v), f.Filters)
				}
			}
			sels := f.Selectors
			if len(sels) == 0 && f.Selector != "" {
				sels = []string{f.Selector}
			}
			for _, s := range sels {
				var str string
				if s == ":self" {
					str = jsonStringify(row)
				} else if v := jsonNavigate(row, s); v != nil {
					str = jsonStringify(v)
				}
				if str != "" {
					return ApplyFilters(str, f.Filters)
				}
			}
			if f.Text != "" {
				return ApplyFilters(f.Text, f.Filters)
			}
			return ""
		}
		f := def.Selectors.Fields
		t := Torrent{
			Title:     get(f["title"]),
			SubTitle:  get(f["subtitle"]),
			Size:      normalizeJSONSearchSize(get(f["size"])),
			Time:      get(f["time"]),
			Category:  get(f["category"]),
			Seeders:   ToInt(get(f["seeders"])),
			Leechers:  ToInt(get(f["leechers"])),
			Completed: ToInt(get(f["completed"])),
		}
		t.DetailURL = ResolveURL(base, get(f["detail"]))
		t.DownloadURL = ResolveURL(base, get(f["download"]))
		if get(f["free"]) != "" {
			t.Free = true
		}
		// 跳过无标题且无下载链接的空行
		if t.Title == "" && t.DownloadURL == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

func ParseJSONUserInfoFields(body []byte, fields map[string]ElementQuery, u *UserInfo) {
	if u == nil || len(fields) == 0 {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return
	}
	for key, q := range fields {
		raw := jsonExtract(root, q)
		if raw == "" {
			continue
		}
		switch key {
		case "id":
			u.ID = raw
		case "name":
			u.Name = raw
		case "email":
			u.Email = raw
		case "level":
			u.Level = raw
		case "level_id":
			u.LevelID = ToInt(raw)
		case "numeric_user_id":
			u.NumericUserID = ToInt(raw)
		case "avatar":
			u.Avatar = raw
		case "personal_avatar":
			u.PersonalAvatar = raw
		case "is_donor":
			u.IsDonor = ToBool(raw)
		case "join_date":
			u.JoinDate = raw
		case "last_access_at":
			u.LastAccessAt = raw
		case "uploaded":
			u.Uploaded = ParseBytes(raw)
		case "downloaded":
			u.Downloaded = ParseBytes(raw)
		case "ratio":
			u.Ratio = ToFloat64(raw)
		case "true_uploaded":
			u.TrueUploaded = ParseBytes(raw)
		case "true_downloaded":
			u.TrueDownloaded = ParseBytes(raw)
		case "true_ratio":
			u.TrueRatio = ToFloat64(raw)
		case "seeding":
			u.Seeding = ToInt(raw)
		case "leeching":
			u.Leeching = ToInt(raw)
		case "seeding_size":
			u.SeedingSize = ParseBytes(raw)
		case "uploads":
			u.Uploads = ToInt(raw)
		case "uploads_add":
			u.Uploads += ToInt(raw)
		case "adoptions":
			u.Adoptions = ToInt(raw)
		case "posts":
			u.Posts = ToInt(raw)
		case "snatches":
			u.Snatches = ToInt(raw)
		case "total_traffic":
			u.TotalTraffic = ParseBytes(raw)
		case "unique_groups":
			u.UniqueGroups = ToInt(raw)
		case "perfect_flac":
			u.PerfectFLAC = ToInt(raw)
		case "invited":
			u.Invited = ToInt(raw)
		case "bonus":
			u.Bonus = ToFloat64(raw)
		case "bonus_per_hour":
			u.BonusPerHour = ToFloat64(raw)
		case "seeding_bonus":
			u.SeedingBonus = ToFloat64(raw)
		case "seeding_bonus_per_hour":
			u.SeedingBonusPerHour = ToFloat64(raw)
		case "rescue_seeding_bonus_per_hour":
			u.RescueSeedingBonusPerHour = ToFloat64(raw)
		case "seeding_time":
			u.SeedingTime = int64(ToFloat64(raw))
		case "claim_count":
			u.ClaimCount = ToInt(raw)
		case "max_claim":
			u.MaxClaim = ToInt(raw)
		}
	}
}

func jsonExtract(root any, q ElementQuery) string {
	if q.Data != "" {
		if v := jsonNavigate(root, q.Data); v != nil {
			return ApplyFilters(jsonStringify(v), q.Filters)
		}
	}
	sels := q.Selectors
	if len(sels) == 0 && q.Selector != "" {
		sels = []string{q.Selector}
	}
	for _, sel := range sels {
		var str string
		if sel == ":self" {
			str = jsonStringify(root)
		} else if v := jsonNavigate(root, sel); v != nil {
			str = jsonStringify(v)
		}
		if str != "" {
			return ApplyFilters(str, q.Filters)
		}
	}
	if q.Text != "" {
		return ApplyFilters(q.Text, q.Filters)
	}
	return ""
}
