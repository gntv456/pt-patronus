package main

import (
	"fmt"
	"strings"
)

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
	out := map[string]any{}
	if api := str(doc["userTorrentListApi"]); api != "" {
		out["user_torrent_list_api"] = api
	}
	if ui == nil {
		if len(out) > 0 {
			return out
		}
		return nil
	}
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
