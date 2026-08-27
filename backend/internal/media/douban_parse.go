package media

import (
	"regexp"
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
