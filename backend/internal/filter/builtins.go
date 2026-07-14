package filter

import "github.com/hxpt/ptpatronus/internal/site"

// BuiltinRuleSet 是 22 条内置规则（对齐 MP-2 app/modules/filter/builtin_rules.py BUILTIN_RULE_SET）。
//
// regex 字符串均不含 (?i) 前缀——引擎在编译时统一加 IGNORECASE（对齐 MP-2 _compile_ignorecase）。
// RE2 不支持 lookahead/lookbehind/空交替，以下两处相对 MP-2 原文做了等价改写（fixture 测试钉死）：
//   - BLU.exclude: \b(?<!WEB-|HDTV)RIP\b → \bRIP\b（exclude 为 OR，WEB/HDTV 的 RIP 已被各自项覆盖；
//     裸 RIP 命中范围更广，方向与「蓝光原盘规则排除一切 RIP」一致）。
//   - CNSUB.include 两段：
//     ([\s,.-\[])(chs|cht)(|[\s,.-\]])            → ([\s,.-\[])(chs|cht)([\s,.-\]]|$)   （去掉空交替）
//     (?<![a-z0-9])(gb|big5)(?![a-z0-9])          → (?:^|[^a-z0-9])(gb|big5)(?:[^a-z0-9]|$)
var BuiltinRuleSet = map[string]Rule{
	// 蓝光原盘
	"BLU": {
		ID: "BLU",
		Include: []string{
			`(\bBlu-?Ray\b.*\b(?:VC-?1|AVC|MPEG-?2)\b|\b(?:UHD|4K|2160p)\b(?:.*Blu-?Ray)?.*\b(?:HEVC|H\.?265)\b|\bBlu-?Ray\b.*\b(?:UHD|4K|2160p)\b.*\b(?:HEVC|H\.?265)\b|\b(?:COMPLETE|FULL)\b.*\b(?:(?:UHD|4K|2160p)\b.*)?Blu-?Ray\b|\b(BD25|BD50|BD66|BD100|BDMV|MiniBD)\b)`,
		},
		Exclude: []string{
			`(\b[XH]\.?264\b|\b[XH]\.?265\b|\bWEB-?DL\b|\bWEB-?RIP\b|\bHDTV(?:RIP)?\b|\bREMUX\b|\bBDRip\b|\bBRRip\b|\bHDRip\b|\bENCODE\b|\bRIP\b)`,
		},
	},
	// 4K
	"4K": {ID: "4K", Include: []string{`4k|2160p|x2160`}},
	// 1080P
	"1080P": {ID: "1080P", Include: []string{`1080[pi]|x1080`}},
	// 720P
	"720P": {ID: "720P", Include: []string{`720[pi]|x720`}},
	// 中字（含 TMDB 国别短路：原声中文直接命中）
	"CNSUB": {
		ID: "CNSUB",
		Include: []string{
			`[中国國繁简](/|\s|\\|\|)?[繁简英粤]|[英简繁](/|\s|\\|\|)?[中繁简]` +
				`|繁體|简体|[中国國][字配]|国语|國語|中文|中字|简日|繁日|简繁|繁体` +
				`|([\s,.-\[])(chs|cht)([\s,.-\]]|$)` +
				`|(?:^|[^a-z0-9])(gb|big5)(?:[^a-z0-9]|$)`,
		},
		TMDB: map[string]string{"original_language": "zh,cn"},
	},
	// 官种（只在 labels/Tags 上匹配）；关键词与 site.IsOfficial 同源（site.OfficialKeywords）。
	"GZ": {ID: "GZ", Include: site.OfficialKeywords, Match: []string{"labels"}},
	// 特效字幕
	"SPECSUB": {ID: "SPECSUB", Include: []string{`特效`}},
	// BluRay
	"BLURAY": {ID: "BLURAY", Include: []string{`Blu-?Ray`}},
	// UHD
	"UHD": {ID: "UHD", Include: []string{`UHD|UltraHD`}},
	// H265
	"H265": {ID: "H265", Include: []string{`[Hx].?265|HEVC`}},
	// H264
	"H264": {ID: "H264", Include: []string{`[Hx].?264|AVC`}},
	// 杜比视界
	"DOLBY": {ID: "DOLBY", Include: []string{`Dolby[\s.]+Vision|DOVI|[\s.]+DV[\s.]+|杜比视界`}},
	// 杜比全景声
	"ATMOS": {ID: "ATMOS", Include: []string{`Dolby[\s.+]+Atmos|Atmos|杜比全景[声聲]`}},
	// HDR
	"HDR": {ID: "HDR", Include: []string{`[\s.]+HDR[\s.]+|HDR10|HDR10\+|HDRVivid`}},
	// SDR
	"SDR": {ID: "SDR", Include: []string{`[\s.]+SDR[\s.]+`}},
	// 重编码
	"REMUX": {ID: "REMUX", Include: []string{`REMUX`}},
	// WEB-DL
	"WEBDL": {ID: "WEBDL", Include: []string{`WEB-?DL|WEB-?RIP`}},
	// 免费（下载因子=0；PTPatronus site.Torrent 无该字段时用 Free 近似 true→0）
	"FREE": {ID: "FREE", DownloadVolumeFactor: ptrFloat(0)},
	// 国语配音（TMDB 原声 zh 短路）
	"CNVOI": {
		ID:      "CNVOI",
		Include: []string{`[国國][语語]配音|[国國]配|[国國][语語]`},
		TMDB:    map[string]string{"original_language": "zh"},
	},
	// 粤语配音
	"HKVOI": {ID: "HKVOI", Include: []string{`粤语配音|粤语`}},
	// 60FPS
	"60FPS": {ID: "60FPS", Include: []string{`60fps|60帧`}},
	// 3D
	"3D": {ID: "3D", Include: []string{`3D`}},
}

// ptrFloat 返回 v 的指针（用于 DownloadVolumeFactor 字段）。
func ptrFloat(v float64) *float64 { return &v }

// builtinOrder 供前端稳定展示内置规则顺序（map 遍历无序）。
var builtinOrder = []string{
	"BLU", "4K", "1080P", "720P", "CNSUB", "GZ", "SPECSUB", "BLURAY", "UHD",
	"H265", "H264", "DOLBY", "ATMOS", "HDR", "SDR", "REMUX", "WEBDL", "FREE",
	"CNVOI", "HKVOI", "60FPS", "3D",
}

// ListBuiltins 按 builtinOrder 返回内置规则副本（调用方可安全修改）。
func ListBuiltins() []Rule {
	out := make([]Rule, 0, len(builtinOrder))
	for _, id := range builtinOrder {
		r, ok := BuiltinRuleSet[id]
		if !ok {
			continue
		}
		out = append(out, cloneRule(r))
	}
	return out
}

// cloneRule 深拷贝 Rule（切片/字典独立，避免调用方污染内置常量）。
func cloneRule(r Rule) Rule {
	r.Include = append([]string(nil), r.Include...)
	r.Exclude = append([]string(nil), r.Exclude...)
	r.Match = append([]string(nil), r.Match...)
	if r.TMDB != nil {
		m := make(map[string]string, len(r.TMDB))
		for k, v := range r.TMDB {
			m[k] = v
		}
		r.TMDB = m
	}
	if r.DownloadVolumeFactor != nil {
		v := *r.DownloadVolumeFactor
		r.DownloadVolumeFactor = &v
	}
	return r
}
