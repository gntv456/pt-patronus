package transfer

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Meta 是从文件名/种子名解析出的元信息（对标 MP MetaBase，精简版）。
// 用于：(1) 无 DownloadMediaLink 时构造搜索关键字；(2) 命名时填季集/分辨率/组。
type Meta struct {
	Title         string // 清洗后的标题（搜索关键字）
	Year          int
	Season        int    // 0=未识别
	Episode       int    // 0=未识别（季包或电影）
	Seasons       string // 展示用 "S01"
	Episodes      string // 展示用 "S01E02" / "E01E02"
	SeasonEpisode string // 命名用 "S01E02" / "S01"
	Resolution    string // 1080P / 2160P / 720P / 4K ...
	ReleaseGroup  string // 发布组（末段 -XXX）
	IsTV          bool
}

var (
	reSeasonEpisode = regexp.MustCompile(`(?i)S(\d{1,2})\s*E(\d{1,3})`)
	// 季包「S01」：仅当未命中 S01E02 时才尝试（见 ParseMeta 顺序），故无需 lookahead 排除。
	reSeasonPack  = regexp.MustCompile(`(?i)\bS(\d{1,2})\b`)
	reEpisodeOnly = regexp.MustCompile(`(?i)\bE(\d{1,3})\b`)
	reYear        = regexp.MustCompile(`[\(\[]?((?:19|20)\d{2})[\)\]]?`)
	// 分辨率：4K/2160P/1080P/720P/480P/8K/UHD（统一大写，2160↔4K 归一在 normalize）。
	reResolution = regexp.MustCompile(`(?i)\b(4K|2160P|1080P|1080I|720P|480P|8K|UHD|H265|H264|HEVC|X264|X265|AVC)\b`)
	reResNorm    = regexp.MustCompile(`(?i)^(2160P|4K|UHD)$`)
	// 质量噪声词：从标题里剥离，避免污染搜索关键字。
	reNoise   = regexp.MustCompile(`(?i)\b(bluray|blu-ray|bdrip|brrip|webrip|web-dl|webdl|web|hdtv|dvdrip|remux|hdr10?|hdr10plus|dts|atmos|truehd|ddp|ddp5|aac|ac3|flac|dual|10bit|hybrid|avc|hevc|x264|x265|h264|h265)\b`)
	reSep     = regexp.MustCompile(`[._]`)
	reMultiEp = regexp.MustCompile(`(?i)E(\d{1,3})[-E]?(\d{1,3})`)
	// 动漫常见「标题 - 12」绝对集数：数字后必须紧跟括号/方括号或行尾，
	// 避免把「Death Race - 2 2008」这类标题里的数字误判成集数。
	reEpisodeAbsolute = regexp.MustCompile(
		`(?i)\s-\s(\d{1,4})(?:\s*(?:END|FIN|v\d+))?\s*(?:[\(\[]|$)`)
	// 标题开头的 [fansub] 标签：既是发布组来源，也不属于片名。
	reLeadTag = regexp.MustCompile(`^\s*\[([^\[\]]+)\]`)
	// 去噪后残留的空括号（[1080P] 剥掉后留下 "[]"）。
	reEmptyBrackets = regexp.MustCompile(`[\(\[]\s*[\)\]]`)
)

// ParseMeta 从文件名（含或不含扩展名）解析元信息。
func ParseMeta(filename string) Meta {
	m := Meta{}
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	stem = reSep.ReplaceAllString(stem, " ")

	// 季集
	if loc := reSeasonEpisode.FindStringSubmatchIndex(stem); loc != nil {
		m.Season = atoiSafe(stem[loc[2]:loc[3]])
		m.Episode = atoiSafe(stem[loc[4]:loc[5]])
		m.IsTV = true
	} else if g := reSeasonPack.FindStringSubmatch(stem); g != nil {
		m.Season = atoiSafe(g[1])
		m.IsTV = true
	} else if g := reEpisodeOnly.FindStringSubmatch(stem); g != nil {
		m.Episode = atoiSafe(g[1])
		m.IsTV = true
	} else if g := reEpisodeAbsolute.FindStringSubmatch(stem); g != nil {
		m.Episode = atoiSafe(g[1])
		m.IsTV = true
	}
	// 多集合并展示 E01E02
	if g := reMultiEp.FindStringSubmatch(stem); g != nil {
		m.Episodes = "E" + g[1] + "E" + g[2]
	}

	// 年份
	if g := reYear.FindStringSubmatch(stem); g != nil {
		m.Year = atoiSafe(g[1])
	}

	// 分辨率
	if g := reResolution.FindStringSubmatch(stem); g != nil {
		m.Resolution = normalizeResolution(g[1])
	}

	// 发布组：取 stem 末段 - 之后（排除空），最多末两段以兼顾 "x264-GROUP"
	if parts := strings.Split(stem, "-"); len(parts) > 1 {
		last := strings.TrimSpace(parts[len(parts)-1])
		if isPlausibleGroup(last) {
			m.ReleaseGroup = last
		}
	}
	// 动漫发布把组名放在开头 [SubsPlease]：没有后缀组时用它；分辨率/编码标签不算组。
	if m.ReleaseGroup == "" {
		if g := reLeadTag.FindStringSubmatch(stem); g != nil {
			tag := strings.TrimSpace(g[1])
			if isPlausibleGroup(tag) &&
				!reResolution.MatchString(tag) && !reNoise.MatchString(tag) {
				m.ReleaseGroup = tag
			}
		}
	}

	// 标题清洗：取季集/年份之前的部分，去分辨率/质量噪声。
	titleCut := stem
	// 开头 [fansub] 标签不属于片名（组名已在上面取用）。
	titleCut = reLeadTag.ReplaceAllString(titleCut, " ")
	for _, re := range []*regexp.Regexp{
		reSeasonEpisode, reSeasonPack, reEpisodeOnly, reEpisodeAbsolute,
	} {
		if loc := re.FindStringIndex(titleCut); loc != nil {
			titleCut = titleCut[:loc[0]]
		}
	}
	if loc := reYear.FindStringIndex(titleCut); loc != nil {
		titleCut = titleCut[:loc[0]]
	}
	titleCut = reResolution.ReplaceAllString(titleCut, " ")
	titleCut = reNoise.ReplaceAllString(titleCut, " ")
	// 去噪后残留的空括号（[1080P] → "[]"）与末尾分隔符一并清掉。
	titleCut = reEmptyBrackets.ReplaceAllString(titleCut, " ")
	titleCut = strings.ReplaceAll(titleCut, "-", " ")
	m.Title = collapseSpaces(strings.TrimSpace(titleCut))
	if m.Title == "" {
		// 清洗过头（罕见）→ 回退用去掉扩展名的原名首段
		m.Title = collapseSpaces(strings.TrimSpace(strings.SplitN(stem, " ", 2)[0]))
	}

	m.SeasonEpisode = formatSeasonEpisode(m.Season, m.Episode)
	if m.IsTV {
		m.Seasons = formatSeason(m.Season)
		if m.Episodes == "" && m.Episode > 0 {
			m.Episodes = formatSeasonEpisode(m.Season, m.Episode)
		}
	}
	return m
}

// normalizeResolution 归一：2160P/4K/UHD → "2160P"；其余大写原样。
func normalizeResolution(r string) string {
	up := strings.ToUpper(strings.TrimSpace(r))
	if reResNorm.MatchString(up) {
		return "2160P"
	}
	if up == "H265" || up == "HEVC" || up == "X265" || up == "H264" || up == "X264" || up == "AVC" {
		return "" // 编码不算分辨率
	}
	return up
}

// isPlausibleGroup 判断末段是否像发布组（无空格、长度合理、非纯数字）。
func isPlausibleGroup(s string) bool {
	if s == "" || len(s) > 24 {
		return false
	}
	if strings.ContainsAny(s, " ") {
		return false
	}
	if isAllDigit(s) {
		return false
	}
	return true
}

func formatSeason(season int) string {
	if season <= 0 {
		return ""
	}
	return pad2(season)
}

func formatSeasonEpisode(season, episode int) string {
	if season <= 0 && episode <= 0 {
		return ""
	}
	if season <= 0 {
		return "E" + pad2(episode)
	}
	if episode <= 0 {
		return "S" + pad2(season) // 季包
	}
	return "S" + pad2(season) + "E" + pad2(episode)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + atoi(n)
	}
	return atoi(n)
}
