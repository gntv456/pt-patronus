package v1

import (
	"net/url"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

func siteDefinitionsForUI(defs []*site.SiteMetadata) []*site.SiteMetadata {
	groups := make([][]*site.SiteMetadata, 0, len(defs))
	hostGroup := map[string]int{}
	for _, d := range defs {
		if d == nil {
			continue
		}
		hosts := definitionHosts(d)
		groupIdx := -1
		for _, host := range hosts {
			if idx, ok := hostGroup[host]; ok {
				groupIdx = idx
				break
			}
		}
		if groupIdx < 0 {
			groupIdx = len(groups)
			groups = append(groups, nil)
		}
		groups[groupIdx] = append(groups[groupIdx], d)
		for _, host := range hosts {
			hostGroup[host] = groupIdx
		}
	}

	out := make([]*site.SiteMetadata, 0, len(groups))
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			si, sj := definitionScore(group[i]), definitionScore(group[j])
			if si != sj {
				return si > sj
			}
			return group[i].ID < group[j].ID
		})
		kept := *group[0]
		kept.URLs = mergeDefinitionURLs(group)
		kept.AKA = mergeDefinitionAKA(&kept, group)
		kept.Tags = mergeStrings(append([]string{}, kept.Tags...), collectTags(group)...)
		out = append(out, &kept)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Dead != out[j].Dead {
			return !out[i].Dead
		}
		ni, nj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if ni != nj {
			return ni < nj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func definitionHosts(d *site.SiteMetadata) []string {
	urls := append([]string{}, d.URLs...)
	urls = append(urls, d.LegacyURLs...)
	hosts := make([]string, 0, len(urls))
	seen := map[string]bool{}
	for _, raw := range urls {
		host := normalizedURLHost(raw)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	if len(hosts) == 0 {
		return []string{"id:" + d.ID}
	}
	return hosts
}

func normalizedURLHost(raw string) string {
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
	return strings.TrimPrefix(host, "www.")
}

func definitionScore(d *site.SiteMetadata) int {
	if d == nil {
		return 0
	}
	score := 0
	if !d.Dead {
		score += 100000
	}
	score += len(d.Categories) * 20
	score += len(d.LevelRequirements) * 20
	score += len(d.Selectors.Fields) * 8
	score += len(d.UserInfo.Fields) * 8
	score += len(d.UserInfo.DetailFields) * 6
	score += len(d.Detail.Fields) * 4
	score += len(d.SearchCategories) * 4
	score += len(d.AKA) * 2
	score += len(d.Tags)
	score += len(d.URLs) + len(d.LegacyURLs)
	if d.Description != "" {
		score += 5
	}
	return score
}

func mergeDefinitionURLs(group []*site.SiteMetadata) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, d := range group {
		urls := append([]string{}, d.URLs...)
		urls = append(urls, d.LegacyURLs...)
		for _, raw := range urls {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			key := normalizedURLKey(raw)
			if key == "" {
				key = raw
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, raw)
		}
	}
	return out
}

func normalizedURLKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.TrimRight(u.EscapedPath(), "/")
	if path == "" {
		path = "/"
	}
	return strings.ToLower(u.Scheme) + "://" + host + path
}

func mergeDefinitionAKA(kept *site.SiteMetadata, group []*site.SiteMetadata) []string {
	out := append([]string{}, kept.AKA...)
	for _, d := range group {
		if d.Name != "" && d.Name != kept.Name {
			out = mergeStrings(out, d.Name)
		}
		out = mergeStrings(out, d.AKA...)
	}
	return out
}

func collectTags(group []*site.SiteMetadata) []string {
	out := []string{}
	for _, d := range group {
		out = append(out, d.Tags...)
	}
	return out
}

func mergeStrings(base []string, vals ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(vals))
	for _, v := range append(base, vals...) {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}
