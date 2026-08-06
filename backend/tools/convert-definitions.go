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
	} else if supportedSchema(schema) {
		b.WriteString("已由本项目运行时 schema adapter 支持。\n")
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

func supportedSchema(schema string) bool {
	switch schema {
	case "aidoruonline", "avistaz", "cgbtsource", "discuz", "f3nix", "filelist",
		"fsm", "gazelle", "gazelle_jsonapi", "luminance", "meantorrent", "mteam",
		"rartracker", "rousi", "tbdev", "tbsource", "tcg", "tnode", "unit3d",
		"xbtit", "yemapt":
		return true
	default:
		return false
	}
}
