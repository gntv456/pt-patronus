// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package subauto

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// seasonEpisodeRe 匹配文件名里的 S01E02 片段（用于剧集字幕搜索关键字）。
var seasonEpisodeRe = regexp.MustCompile(`(?i)S\d{1,2}\s?E\d{1,3}`)

// videoExts 视频扩展名白名单（整理后 res.Files 理论上都是视频，此守卫兜底防 .nfo/海报）。
var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".wmv": true, ".flv": true,
	".mov": true, ".m4v": true, ".ts": true, ".m2ts": true, ".iso": true, ".rmvb": true,
}

func isVideoFile(p string) bool { return videoExts[strings.ToLower(filepath.Ext(p))] }

// langTags 字幕文件名中可能的语言中缀，用于幂等匹配 basename。
var langTags = []string{".chs", ".cht", ".eng", ".bi", ".und", ".zh", ".cn", ".tw", ".zh-cn", ".zh-tw"}

// existingSubtitles 列出视频同目录、与视频同 basename（含语言中缀）的已有字幕文件名。
func existingSubtitles(dir, base string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	baseLower := strings.ToLower(base)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !subtitle.IsSubtitleExt(name) {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		for _, tag := range langTags {
			stem = strings.TrimSuffix(stem, tag)
		}
		if stem == baseLower {
			out = append(out, name)
		}
	}
	return out
}

// subtitleExt 取字幕扩展名：优先下载文件名 ext，次用 Subtitle.Ext，兜底 .srt。
func subtitleExt(name, ext string) string {
	if e := strings.ToLower(filepath.Ext(name)); e != "" && subtitle.IsSubtitleExt(e) {
		return e
	}
	if ext != "" {
		e := strings.ToLower(ext)
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		if subtitle.IsSubtitleExt(e) {
			return e
		}
	}
	return ".srt"
}

// subtitlePath 生成字幕落盘路径：有 langcode 时加语言中缀（{base}.chs.srt，对齐 Plex/Emby 多语言约定）。
func subtitlePath(dir, base, lang, ext string) string {
	if lang != "" && lang != "und" {
		return filepath.Join(dir, base+"."+lang+ext)
	}
	return filepath.Join(dir, base+ext)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func firstFile(files []string) string {
	if len(files) > 0 {
		return files[0]
	}
	return ""
}
