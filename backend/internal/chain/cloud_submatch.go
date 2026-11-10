// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// cloud_submatch.go —— 云盘 STRM 记录 ↔ 订阅 的文件名匹配纯函数群（自 cloud.go 拆出）。
// 匹配策略：双方归一化（去扩展名/分隔符/分辨率编码年份噪声）后，
// 订阅标题的每个 token 都须作为独立词出现在文件名（避免裸子串误匹配）。

package chain

import (
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// matchAnySubscription 按文件名归一化匹配任一活跃订阅（标题为文件名子串即命中）。
func matchAnySubscription(subs []models.Subscription, filename string) *models.Subscription {
	nf := normalizeName(filename)
	if nf == "" {
		return nil
	}
	ftSet := make(map[string]bool)
	for _, t := range strings.Fields(nf) {
		ftSet[t] = true
	}
	for i := range subs {
		t := normalizeName(subs[i].Title)
		if t == "" {
			continue
		}
		titleTokens := strings.Fields(t)
		if len(titleTokens) == 0 {
			continue
		}
		// 订阅标题的每个 token 都须作为独立词出现在文件名（避免裸子串误匹配：
		// "It" 命中 "Comm**it**ments"、"Her" 命中 "Mot**her**"）。
		all := true
		for _, tt := range titleTokens {
			if !ftSet[tt] {
				all = false
				break
			}
		}
		if all {
			return &subs[i]
		}
	}
	return nil
}

// normalizeName 归一化文件名/标题用于匹配：去扩展名、小写、分隔符→空格、剥离分辨率/编码/年份噪声。
func normalizeName(s string) string {
	s = strings.ToLower(s)
	// 去扩展名：仅当尾段像扩展名（≤4 且纯字母数字，如 mkv/mp4/srt），避免误吃标题中的点分段。
	if i := strings.LastIndex(s, "."); i > 0 {
		tail := s[i+1:]
		if len(tail) <= 4 && isAlnum(tail) {
			s = s[:i]
		}
	}
	// 分隔符 → 空格
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ", "[", " ", "]", " ", "(", " ", ")", " ").Replace(s)
	// 剥离常见噪声 token
	noise := []string{
		"1080p", "2160p", "4k", "720p", "480p",
		"x264", "h264", "x265", "h265", "hevc", "avc",
		"bluray", "blu-ray", "webrip", "web-dl", "webdl", "hdrip", "dvdrip",
		"10bit", "sdr", "hdr", "dts", "aac", "ac3", "flac", "mp3",
		"remux", "chs", "cht", "gb", "big5",
	}
	words := strings.Fields(s)
	out := make([]string, 0, len(words))
	for _, w := range words {
		clean := strings.Trim(w, ".,;:!?\"'")
		if clean == "" {
			continue
		}
		if isNoise(clean, noise) {
			continue
		}
		// 纯年份 token（1900-2099）剥离
		if isYear(clean) {
			continue
		}
		out = append(out, clean)
	}
	return strings.Join(out, " ")
}

func isNoise(w string, list []string) bool {
	for _, n := range list {
		if w == n {
			return true
		}
	}
	return false
}

func isYear(w string) bool {
	if len(w) != 4 {
		return false
	}
	y, err := strconv.Atoi(w)
	return err == nil && y >= 1900 && y <= 2099
}

// isAlnum 全小写字母数字（扩展名尾段判定）。
func isAlnum(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
