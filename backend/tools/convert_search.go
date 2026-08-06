package main

import (
	"sort"
	"strings"
)

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
