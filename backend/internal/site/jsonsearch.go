package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
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
		return fmt.Sprint(x)
	}
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
	u := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(entry, "/")
	q := url.Values{}
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
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}
	return u
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
	rowsNode := jsonNavigate(root, def.Selectors.Rows.Selector)
	rows, _ := rowsNode.([]any)
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
			return ""
		}
		f := def.Selectors.Fields
		t := Torrent{
			Title:     get(f["title"]),
			SubTitle:  get(f["subtitle"]),
			Size:      get(f["size"]),
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
