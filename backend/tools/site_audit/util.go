package main

import (
	"net/url"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

func defHosts(def site.SiteMetadata) []string {
	var out []string
	for _, raw := range append(append([]string{}, def.URLs...), def.LegacyURLs...) {
		h := normalizedHost(raw)
		if h != "" {
			out = append(out, h)
		}
	}
	return unique(out)
}

func normalizedHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

func leadingComments(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			lines = append(lines, line)
			continue
		}
		break
	}
	return strings.Join(lines, "\n")
}

func headerTodos(header string) []string {
	var out []string
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if strings.HasPrefix(line, "- ") {
			out = append(out, strings.TrimPrefix(line, "- "))
		}
	}
	return out
}

func missingIDs(ref, ours map[string]bool) []string {
	var out []string
	for id := range ref {
		if !ours[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func countMissing(ref, ours map[string]bool) int {
	return len(missingIDs(ref, ours))
}

func filter(in []auditDef, fn func(auditDef) bool) []auditDef {
	var out []auditDef
	for _, d := range in {
		if fn(d) {
			out = append(out, d)
		}
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedReferenceGroupTitles(m map[string]map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func setOf(vals ...string) map[string]bool {
	out := map[string]bool{}
	for _, v := range vals {
		out[v] = true
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func backtickJoin(in []string) string {
	parts := make([]string, 0, len(in))
	for _, s := range in {
		parts = append(parts, "`"+s+"`")
	}
	return strings.Join(parts, ", ")
}
