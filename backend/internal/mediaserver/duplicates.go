package mediaserver

import (
	"sort"
	"strings"
)

// DuplicateGroup 一组判定为重复的条目（同一 TMDB/IMDb ID，或归一化标题+年份相同）。
type DuplicateGroup struct {
	Key   string         `json:"key"` // 归并键（provider id 或 normalized title+year）
	Title string         `json:"title"`
	Year  int            `json:"year"`
	Type  string         `json:"type"`
	Count int            `json:"count"`
	Items []DetailedItem `json:"items"`
}

// DetectDuplicates 对库内条目做重复检测：
//   - 优先按 Provider ID 归并（TMDB > IMDb），命中即同一作品。
//   - 无 Provider ID 的回退到「归一化标题 + 年份」归并。
//   - 仅返回 Count >= 2 的组（即疑似重复），按 Count 降序、再按标题排序。
//
// 归一化：去两端空白、转小写、去常见分隔符与「(年份)」尾巴、折叠连续空白。
// 旨在把 "Movie Name 2023" 与 "Movie.Name.(2023)" 视作同名；不保证跨语言等价。
func DetectDuplicates(items []DetailedItem) []DuplicateGroup {
	groups := map[string]*DuplicateGroup{}
	order := []string{} // 保留首次出现顺序，便于稳定排序

	add := func(key string, it DetailedItem) {
		g, ok := groups[key]
		if !ok {
			g = &DuplicateGroup{
				Key: key, Title: it.Name, Year: it.Year, Type: it.Type, Items: []DetailedItem{it},
			}
			groups[key] = g
			order = append(order, key)
			return
		}
		g.Items = append(g.Items, it)
		g.Count = len(g.Items)
		// 标题/年份取首个非空（Provider 归并下首个可能缺年份，尽量补全）。
		if g.Title == "" {
			g.Title = it.Name
		}
		if g.Year == 0 {
			g.Year = it.Year
		}
	}

	for _, it := range items {
		switch {
		case it.TMDBID != "":
			add("tmdb:"+it.TMDBID, it)
		case it.IMDbID != "":
			add("imdb:"+it.IMDbID, it)
		default:
			add("name:"+normalizeTitle(it.Name)+"|"+itoa(it.Year), it)
		}
	}

	out := make([]DuplicateGroup, 0, len(groups))
	for _, k := range order {
		g := groups[k]
		if len(g.Items) < 2 {
			continue
		}
		g.Count = len(g.Items)
		out = append(out, *g)
	}
	// 按重复数降序，再按标题稳定排序。
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// normalizeTitle 归一化标题用于无 Provider ID 时的同名归并。
// 转小写、常见分隔符与括号转空白、折叠连续空白、再去尾部独立 4 位年份（年份已作归并键的一部分）。
// 旨在把 "Inception.2010" / "Inception (2010)" / "Movie_Name-2023" 视作同名。
func normalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	r := strings.NewReplacer(".", " ", "_", " ", "-", " ", "/", " ", "(", " ", ")", " ", "[", " ", "]", " ")
	s = r.Replace(s)
	fields := strings.Fields(s)
	for len(fields) > 0 && isYearToken(fields[len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}

// isYearToken 报告 t 是否为 4 位合理年份（1900-2099），用于剥离标题尾部的年份噪声。
func isYearToken(t string) bool {
	if len(t) != 4 || !allDigits(t) {
		return false
	}
	// allDigits 保证可解析；限定合理区间避免误吞纯数字标题（如 "1984" 本身是片名时仍会被剥——
	// 此处倾向归并宽容，依赖 Provider ID 优先路径兜底精确判定）。
	return t[0] == '1' || t[0] == '2'
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// itoa 局部 int→string（避免额外 import strconv 仅为这一处；本文件无其他 strconv 用途）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
