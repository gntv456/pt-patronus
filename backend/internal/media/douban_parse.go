// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import (
	"regexp"
	"strings"
)

// parseMinutes 从 durations（如 "110分钟" / "110 min"）解析分钟数；取第一条有效值。
func parseMinutes(ds []string) int {
	for _, d := range ds {
		n := 0
		for _, r := range d {
			if r >= '0' && r <= '9' {
				n = n*10 + int(r-'0')
			} else if n > 0 {
				return n
			}
		}
		if n > 0 {
			return n
		}
	}
	return 0
}

// doubanSubjectIDRe 匹配 celebrities actor.uri 里的 personage subject_id。
var doubanSubjectIDRe = regexp.MustCompile(`subject_id=(\d+)`)

// doubanSubjectID 从 celebrities actor 的 uri（douban://douban.com/celebrity/{cid}?subject_id={pid}）
// 提取 personage subject_id，用于调 /elessar/subject 拉演员简介。无则空。
func doubanSubjectID(uri string) string {
	if m := doubanSubjectIDRe.FindStringSubmatch(uri); m != nil {
		return m[1]
	}
	return ""
}

// parseCardSubtitle 拆豆瓣榜单 card_subtitle 的「年 / 产地 / 类型 [ / 导演 [ / 主演] ]」
// 斜杠分段。类型段按空格拆为多个 genre；段数不足时按位置尽力解析。全不匹配返回空。
func parseCardSubtitle(s string) (genres, countries []string, names []string) {
	parts := strings.Split(s, "/")
	if len(parts) < 3 {
		return nil, nil, nil // 至少 年/产地/类型 三段才可结构化
	}
	trim := func(i int) string { return strings.TrimSpace(parts[i]) }
	for _, g := range strings.Fields(trim(2)) {
		if g != "" {
			genres = append(genres, g)
		}
	}
	if trim(1) != "" {
		countries = append(countries, trim(1))
	}
	if len(parts) > 3 && trim(3) != "" {
		names = append(names, trim(3))
	}
	return genres, countries, names
}
