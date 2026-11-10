// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package transfer

import (
	"os"
	"path/filepath"
	"strings"
)

// qualityScore 按文件名关键词给发行版画质打分（0-100，越高越好）。
// 用于洗版比较：仅当新版评分严格大于旧版才替换。词汇与 filter 内置规则
// (BLU/4K/1080P/REMUX/DOLBY/HDR/H265...) 同源；纯字符串匹配，避免 transfer 引入 filter 依赖。
//
// 评分模型：分辨率定基线（2160p=90 > 1080p=70 > 720p=50 > 480p=32 > 其余 30），
// 叠加来源/封装（REMUX +10、BluRay 原盘 +8、WEB-DL +3）、HDR/视界（DV +6、HDR +5）、
// 编码（H265 +3、H264 +1），封顶 100。保证 4K REMUX HDR > 4K WEB > 1080p BluRay > 1080p WEB > 720p。
func qualityScore(path string) int {
	name := strings.ToLower(filepath.Base(path))
	score := 30
	switch {
	case nameHas(name, "2160p", "2160i", "4k", "x2160", "uhd"):
		score = 90
	case strings.Contains(name, "1080"):
		score = 70
	case strings.Contains(name, "720"):
		score = 50
	case strings.Contains(name, "480"):
		score = 32
	}
	// 来源 / 封装加成（原盘 > 重封装 > WEB）
	switch {
	case strings.Contains(name, "remux"):
		score += 10
	case nameHas(name, "blu-ray", "bluray", "bd66", "bd100", "bd50", "bd25", "bdmv", "minibd"):
		score += 8
	case nameHas(name, "web-dl", "webdl", "webrip", "web-dlrip"):
		score += 3
	}
	// HDR / 杜比视界
	switch {
	case nameHas(name, "dolby vision", "dolbyvision", "dovi", "dbov"):
		score += 6
	case nameHas(name, "hdr10+", "hdr10plus", "hdr10", "hdrvivid", "hdr"):
		score += 5
	}
	// 编码
	switch {
	case nameHas(name, "x265", "h265", "h.265", "x.265", "hevc"):
		score += 3
	case nameHas(name, "x264", "h264", "h.264", "x.264", "avc"):
		score += 1
	}
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

// nameHas 报告 name 是否包含任一 part（大小写不敏感子串）。
func nameHas(name string, parts ...string) bool {
	for _, p := range parts {
		if p != "" && strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// firstVideoInDir 返回 dir（非递归）下首个视频文件完整路径；目录不存在或无视频返 ""。
// 用于电影洗版：检测同片库目录下已存在的旧版本（同名或异画质异名）。
func firstVideoInDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || isSample(e.Name()) {
			continue
		}
		if isVideoFile(e.Name()) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// findEpisodeInDir 返回 dir 下与目标 SxxExx 同集的视频文件完整路径；无则 ""。
// 用于剧集洗版：同季目录下定位同集旧版本（异画质）。
func findEpisodeInDir(dir, se string) string {
	if se == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !isVideoFile(e.Name()) {
			continue
		}
		if ParseMeta(e.Name()).SeasonEpisode == se {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// removeMediaFile 删除一个库媒体文件并返回其路径。硬链模式下仅移除库链接，
// 不影响下载器做种源（源与库为同盘 inode 共享，os.Remove 只降一条链接数）。
func removeMediaFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}
