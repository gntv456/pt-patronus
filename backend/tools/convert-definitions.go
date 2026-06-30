// Command convert-definitions 把 export-definitions.mjs 产出的 dump/*.json 翻译为
// PTPatronus 的 SiteMetadata YAML（输出到 tools/out/<id>.yaml）。
//
// 运行：cd /d/PTPatronus/backend && go run ./tools/convert-definitions.go
//
// 翻译规则见计划 Phase 1.2。无法自动翻译的字段（elementProcess/case/匿名 filter/外部常量引用）
// 跳过并记入文件头 TODO 清单 + calibration: pending 标记，供 M2+ 人工处理。
// NexusPHP 站的 selectors/user_info 若继承自 SchemaMetadata（dump 里只有 __spread），
// 本项目 EnsureDefaults 会自填校准过的默认，故无需输出。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DUMP_DIR = "tools/dump"
	OUT_DIR  = "tools/out"
)

func main() {
	if err := os.MkdirAll(OUT_DIR, 0o755); err != nil {
	fatal(err)
	}
	entries, err := os.ReadDir(DUMP_DIR)
	if err != nil {
		fatal(err)
	}
	ok, pending := 0, 0
	var noMeta []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		raw, err := os.ReadFile(filepath.Join(DUMP_DIR, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", id, err)
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", id, err)
			continue
		}
		body, todos, hasMeta := convert(id, doc)
		if !hasMeta {
			noMeta = append(noMeta, id)
			continue
		}
		yb, err := yaml.Marshal(body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal %s: %v\n", id, err)
			continue
		}
		header := buildHeader(id, doc, todos)
		out := header + string(yb)
		if err := os.WriteFile(filepath.Join(OUT_DIR, id+".yaml"), []byte(out), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", id, err)
			continue
		}
		ok++
		if len(todos) > 0 {
			pending++
		}
	}
	fmt.Printf("converted %d sites → %s (%d with pending TODOs)\n", ok, OUT_DIR, pending)
	if len(noMeta) > 0 {
		fmt.Printf("skipped (no usable metadata): %s\n", strings.Join(noMeta, ", "))
	}
}

// convert 把一份 dump JSON 转为本项目 SiteMetadata 形状的 map + TODO 清单。
// 返回 (body, todos, hasMeta)。
func convert(id string, doc map[string]any) (map[string]any, []string, bool) {
	if doc["id"] == nil {
		return nil, nil, false
	}
	var todos []string
	body := map[string]any{}

	body["id"] = str(doc["id"])
	if v := str(doc["name"]); v != "" {
		body["name"] = v
	}
	if v := schemaNormalize(str(doc["schema"])); v != "" {
		body["schema"] = v
	}
	if aka := strSlice(doc["aka"]); len(aka) > 0 {
		body["aka"] = aka
	}
	if d := str(doc["description"]); d != "" {
		body["description"] = d
	}
	if t := strSlice(doc["tags"]); len(t) > 0 {
		body["tags"] = t
	}
	if tz := str(doc["timezoneOffset"]); tz != "" {
		body["timezone_offset"] = tz
	}
	if urls := restoreURLs(doc["urls"]); len(urls) > 0 {
		body["urls"] = urls
	}
	if lu := restoreURLs(doc["legacyUrls"]); len(lu) > 0 {
		body["legacy_urls"] = lu
	}
	if dead, _ := doc["isDead"].(bool); dead {
		body["dead"] = true
	}

	// 搜索入口：优先 searchEntry，其次 search.requestConfig
	if se := pickSearchEntry(doc); se != "" {
		body["search_entry"] = se
	}
	if sp := pickSearchParam(doc); sp != "" {
		body["search_param"] = sp
	}

	// selectors / user_info / categories / level_requirements
	if sel := translateSearchSelectors(doc, &todos); sel != nil {
		body["selectors"] = sel
	}
	if ui := translateUserInfo(doc, &todos); ui != nil {
		body["user_info"] = ui
	}
	if cats := translateCategory(doc); cats != nil {
		body["categories"] = cats
	}
	if lr := translateLevelRequirements(doc); lr != nil {
		body["level_requirements"] = lr
	}

	// noLoginAssert → no_login（best-effort，结构映射）
	if nl := translateNoLogin(doc); nl != nil {
		body["no_login"] = nl
	}

	return body, todos, true
}

// buildHeader 生成文件头注释：来源、calibration 状态、schema、TODO 清单。
func buildHeader(id string, doc map[string]any, todos []string) string {
	var b strings.Builder
	schema := schemaNormalize(str(doc["schema"]))
	fmt.Fprintf(&b, "# %s — 自动生成自 PT-depiler（tools/convert-definitions.go）。\n", id)
	fmt.Fprintf(&b, "# schema: %s。", schema)
	if schema == "nexusphp" || schema == "" {
		b.WriteString("selectors/user_info 若未列出则由 EnsureDefaults 自填校准默认。\n")
	} else {
		b.WriteString("⚠ 本项目暂无该 schema 适配器，待 M3+ 实现。\n")
	}
	if len(todos) > 0 {
		b.WriteString("# calibration: pending — 以下字段含 elementProcess/case/匿名函数，需人工翻译：\n")
		// 去重保序
		seen := map[string]bool{}
		for _, t := range todos {
			if !seen[t] {
				seen[t] = true
				fmt.Fprintf(&b, "#   - %s\n", t)
			}
		}
	} else {
		b.WriteString("# calibration: pending — 翻译自 PT-depiler，待真实页面校准。\n")
	}
	b.WriteString("\n")
	return b.String()
}

// ---- 字段翻译 ----

// translateSearchSelectors 翻译 search.selectors → 本项目 selectors{rows,fields}。
func translateSearchSelectors(doc map[string]any, todos *[]string) map[string]any {
	search, _ := doc["search"].(map[string]any)
	if search == nil {
		return nil
	}
	sel, _ := search["selectors"].(map[string]any)
	if sel == nil {
		return nil
	}
	out := map[string]any{}
	if rows := translateQuery(sel["rows"], "search.rows", todos); rows != nil {
		out["rows"] = rows
	}
	fields := map[string]any{}
	for ptKey, ev := range fieldKeyMap() {
		if v, ok := sel[ptKey].(map[string]any); ok {
			if q := translateQuery(v, "search."+ptKey, todos); q != nil {
				fields[ev] = q
			}
		}
	}
	if len(fields) > 0 {
		out["fields"] = fields
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fieldKeyMap PT-depiler search 字段名 → 本项目 SiteSelectors.Fields 约定键。
func fieldKeyMap() map[string]string {
	return map[string]string{
		"title": "title", "subTitle": "subtitle",
		"url": "detail", "link": "download",
		"size": "size", "seeders": "seeders", "leechers": "leechers",
		"completed": "completed", "time": "time", "category": "category",
	}
}

// translateUserInfo 翻译 userInfo.selectors → 本项目 user_info{path,fields,detail_path,detail_fields}。
func translateUserInfo(doc map[string]any, todos *[]string) map[string]any {
	ui, _ := doc["userInfo"].(map[string]any)
	if ui == nil {
		return nil
	}
	out := map[string]any{}
	if sels, ok := ui["selectors"].(map[string]any); ok {
		fields := map[string]any{}
		for ptKey, ev := range userInfoKeyMap() {
			if v, ok := sels[ptKey].(map[string]any); ok {
				if q := translateQuery(v, "userInfo."+ptKey, todos); q != nil {
					fields[ev] = q
				}
			}
		}
		if len(fields) > 0 {
			out["fields"] = fields
		}
	}
	if proc, _ := ui["process"].([]any); len(proc) > 0 {
		*todos = append(*todos, "userInfo.process（多步抓取流程，需适配器实现）")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func userInfoKeyMap() map[string]string {
	return map[string]string{
		"uploaded": "uploaded", "downloaded": "downloaded", "ratio": "ratio",
		"seeding": "seeding", "leeching": "leeching", "bonus": "bonus",
		"seedingSize": "seeding_size", "uploads": "uploads",
		"joinTime": "join_date", "levelName": "level",
		"bonusPerHour": "bonus_per_hour", "name": "name", "id": "id",
	}
}

// translateQuery 翻译单个 IElementQuery（PT-depiler）→ 本项目 ElementQuery。
// 含 elementProcess/case/__fn/__call/__tpl/__regex/__ref 的记 TODO。
func translateQuery(q any, path string, todos *[]string) map[string]any {
	m, ok := q.(map[string]any)
	if !ok {
		return nil
	}
	if hasUntranslatable(m) {
		kinds := untranslatableKinds(m)
		*todos = append(*todos, fmt.Sprintf("%s（%s）", path, kinds))
	}
	out := map[string]any{}
	// PT-depiler 用 selector 字段存 string 或 string[]（数组即回退链）；本项目 selector（单）/ selectors（链）
	switch sv := m["selector"].(type) {
	case string:
		if s := normSelector(sv); s != "" {
			out["selector"] = s
		}
	case []any:
		if sels := cleanSelectors(sv); len(sels) > 0 {
			out["selectors"] = sels
		}
	}
	if sels := cleanSelectors(m["selectors"]); len(sels) > 0 {
		out["selectors"] = sels
	}
	if a := str(m["attr"]); a != "" {
		out["attr"] = a
	}
	if p, _ := m["parent"].(bool); p {
		out["parent"] = true
	}
	if r := str(m["remove"]); r != "" {
		out["remove"] = r
	}
	if fs := translateFilters(m["filters"], path, todos); len(fs) > 0 {
		out["filters"] = fs
	}
	return out
}

// hasUntranslatable 检测 IElementQuery 是否含本项目无法直译的子字段。
func hasUntranslatable(m map[string]any) bool {
	for _, k := range []string{"elementProcess", "case", "switchFilters", "data", "text", "__fn", "__call", "__tpl", "__regex", "__ref", "__raw"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func untranslatableKinds(m map[string]any) string {
	var kinds []string
	for _, k := range []string{"elementProcess", "case", "switchFilters", "data", "text", "__fn", "__call", "__tpl", "__regex", "__ref", "__raw"} {
		if _, ok := m[k]; ok {
			kinds = append(kinds, k)
		}
	}
	return strings.Join(kinds, "/")
}

// translateFilters 翻译 filters 数组。识别的 name 映射，匿名函数记 TODO。
func translateFilters(v any, path string, todos *[]string) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, f := range arr {
		fm, ok := f.(map[string]any)
		if !ok {
			// 匿名函数 filter
			*todos = append(*todos, fmt.Sprintf("%s.filters（匿名函数）", path))
			continue
		}
		name := str(fm["name"])
		if name == "" {
			// 整个 filter 是函数对象
			if _, ok := fm["__fn"]; ok {
				*todos = append(*todos, fmt.Sprintf("%s.filters（匿名函数）", path))
			}
			continue
		}
		mapped, ok := filterNameMap()[name]
		if !ok {
			*todos = append(*todos, fmt.Sprintf("%s.filters[%s]", path, name))
			continue
		}
		entry := map[string]any{"name": mapped}
		if a := filterArgs(name, fm["args"]); a != "" {
			entry["args"] = a
		}
		out = append(out, entry)
	}
	return out
}

func filterNameMap() map[string]string {
	return map[string]string{
		"querystring": "queryparam",
		"parseNumber": "parse_number",
		"parseSize":   "parse_size",
		"parseTime":   "parse_time",
		"parseTTL":    "parse_time",
		"parseDuration": "parse_time",
		"parseFuzzyTime": "parse_time",
		"toLower": "lower", "toUpper": "upper",
		"replace": "replace", "trim": "trim",
		"regexp":  "regexp", "regex": "regexp",
	}
}

// filterArgs 归一 filter args：querystring 的 args 是 ["id"]，本项目 queryparam 用 "id"。
func filterArgs(name string, v any) string {
	switch name {
	case "querystring":
		if s := strSliceClean(v); len(s) > 0 {
			return s[0]
		}
	}
	return str(v)
}

// translateLevelRequirements 翻译 levelRequirements 数组 → 本项目 level_requirements。
func translateLevelRequirements(doc map[string]any) []map[string]any {
	arr, ok := doc["levelRequirements"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		entry := map[string]any{}
		putNum(entry, "id", m["id"])
		putStr(entry, "name", str(m["name"]))
		putSlice(entry, "name_aka", strSlice(m["nameAka"]))
		putStr(entry, "interval", str(m["interval"]))
		putStr(entry, "downloaded", str(m["downloaded"]))
		putStr(entry, "uploaded", str(m["uploaded"]))
		putNum(entry, "ratio", m["ratio"])
		putNum(entry, "bonus", m["bonus"])
		putNum(entry, "seeding_bonus", m["seedingBonus"])
		putStr(entry, "seeding_time", str(m["seedingTime"]))
		putNum(entry, "uploads", m["uploads"])
		if k, _ := m["isKept"].(bool); k {
			entry["is_kept"] = true
		}
		putStr(entry, "group_type", str(m["groupType"]))
		putStr(entry, "privilege", str(m["privilege"]))
		out = append(out, entry)
	}
	return out
}

// translateCategory：把 PT-depiler category 中 key=="cat" 的 options 转为本项目 categories。
// 其余维度（medium/codec/...）是搜索过滤，本项目暂不用。
func translateCategory(doc map[string]any) []map[string]any {
	arr, ok := doc["category"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := m["__ref"]; ok {
			continue // 外部常量引用（CategoryIncldead 等），跳过
		}
		if str(m["key"]) != "cat" {
			continue
		}
		opts, _ := m["options"].([]any)
		for _, o := range opts {
			om, ok := o.(map[string]any)
			if !ok {
				continue
			}
			id := fmt.Sprintf("%v", om["value"])
			out = append(out, map[string]any{"id": id, "name": str(om["name"])})
		}
		if len(out) > 0 {
			break
		}
	}
	return out
}

// translateNoLogin：noLoginAssert → no_login（best-effort）。
func translateNoLogin(doc map[string]any) map[string]any {
	nl, _ := doc["noLoginAssert"].(map[string]any)
	if nl == nil {
		return nil
	}
	out := map[string]any{}
	if s := strSliceClean(nl["httpStatusCodes"]); len(s) > 0 {
		out["http_status"] = s
	}
	if s := strSliceClean(nl["urlPatterns"]); len(s) > 0 {
		out["url_pattern"] = s
	}
	if s := strSliceClean(nl["matchSelectors"]); len(s) > 0 {
		out["selector"] = strings.Join(s, " ")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ---- 搜索入口解析 ----

func pickSearchEntry(doc map[string]any) string {
	// searchEntry 优先（多入口配置）
	if se, ok := doc["searchEntry"].(map[string]any); ok {
		var keys []string
		for k := range se {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if e, ok := se[k].(map[string]any); ok {
				if u := str(e["url"]); u != "" {
					return u
				}
			}
		}
	}
	if search, _ := doc["search"].(map[string]any); search != nil {
		if rc, _ := search["requestConfig"].(map[string]any); rc != nil {
			if u := str(rc["url"]); u != "" {
				return u
			}
		}
	}
	return ""
}

func pickSearchParam(doc map[string]any) string {
	if search, _ := doc["search"].(map[string]any); search != nil {
		if s := str(search["keywordPath"]); s != "" {
			// 形如 "params.searchstr" → 取末段
			if i := strings.LastIndex(s, "."); i >= 0 {
				return s[i+1:]
			}
			return s
		}
	}
	return ""
}

// ---- 工具函数 ----

func schemaNormalize(s string) string {
	switch strings.ToLower(s) {
	case "nexusphp":
		return "nexusphp"
	case "unit3d":
		return "unit3d"
	case "mtorrent":
		return "mteam"
	case "gazellejsonapi":
		return "gazelle_jsonapi"
	case "gazelle", "luminance", "rartracker", "tcg", "tbsource", "xbtit", "discuz":
		return strings.ToLower(s)
	case "avistaznetwork":
		return "avistaz"
	case "abstractprivatesite", "abstractbittorrentsite", "":
		return ""
	default:
		return strings.ToLower(s)
	}
}

// restoreURLs 解密 ROT13 URL（uggc 开头）。
func restoreURLs(v any) []string {
	raw := strSlice(v)
	var out []string
	for _, u := range raw {
		out = append(out, restoreSecureLink(u))
	}
	return out
}

func restoreSecureLink(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "uggc") {
		return rot13(u)
	}
	return u
}

func rot13(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteRune('A' + (c-'A'+13)%26)
		case c >= 'a' && c <= 'z':
			b.WriteRune('a' + (c-'a'+13)%26)
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// strSliceClean 从 any 取字符串切片，过滤掉 map（含 __ 标记）元素。
func strSliceClean(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// normSelector 归一单个选择器：PT-depiler 的 "> td" 是相对当前上下文的直接子代（Sizzle 语法），
// goquery 不支持裸 ">" 开头；本项目 Extract 的 ctx 已是行（tr），去 "> " 前缀转为后代选择器等价。
func normSelector(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "> ") || strings.HasPrefix(s, ">") {
		s = strings.TrimSpace(strings.TrimPrefix(s, ">"))
	}
	return s
}

// cleanSelectors 从 any 取字符串数组并归一每个选择器。
func cleanSelectors(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range arr {
		if s, ok := e.(string); ok {
			if n := normSelector(s); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func strSlice(v any) []string {
	if s, ok := v.(string); ok {
		return []string{s}
	}
	return strSliceClean(v)
}

func putStr(m map[string]any, k string, v string) {
	if v != "" {
		m[k] = v
	}
}

func putNum(m map[string]any, k string, v any) {
	if v == nil {
		return
	}
	f, ok := v.(float64)
	if !ok {
		return // 非数值（表达式/对象/区间数组）：跳过，避免破坏 LevelRequirement 数值字段的 yaml 类型
	}
	if f == 0 {
		return
	}
	// 整数值存 int，避免 yaml 科学计数法（如 1e+06）
	if f == float64(int64(f)) {
		m[k] = int64(f)
	} else {
		m[k] = f
	}
}

func putSlice(m map[string]any, k string, v []string) {
	if len(v) > 0 {
		m[k] = v
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
