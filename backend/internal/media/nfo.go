// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import (
	"fmt"
	"strings"
)

// NFO 生成 Jellyfin/Emby/Kodi 兼容的 NFO（movie/tv/music）。
func NFO(mi *MediaInfo) string {
	var b strings.Builder
	switch mi.Type {
	case TypeTV:
		b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<tvshow>\n")
	case TypeMusic:
		b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<musicvideo>\n")
	default:
		b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<movie>\n")
	}
	writeTag(&b, "title", mi.Title)
	if mi.OriginalTitle != "" {
		writeTag(&b, "originaltitle", mi.OriginalTitle)
	}
	if mi.Rating > 0 {
		writeTag(&b, "rating", fmt.Sprintf("%.1f", mi.Rating))
	}
	if mi.Year > 0 {
		writeTag(&b, "year", fmt.Sprintf("%d", mi.Year))
	}
	if mi.ReleaseDate != "" {
		writeTag(&b, "premiered", mi.ReleaseDate)
	}
	writeTag(&b, "plot", mi.Overview)
	writeTag(&b, "uniqueid", mi.Source+":"+mi.SourceID)
	for _, g := range mi.Genres {
		writeTag(&b, "genre", g)
	}
	for _, c := range mi.Countries {
		writeTag(&b, "country", c)
	}
	for _, d := range mi.Directors {
		writeTag(&b, "director", d)
	}
	// 海报/背景引用（Emby/Jellyfin 读取 <thumb aspect="...">）。
	if mi.PosterURL != "" {
		fmt.Fprintf(&b, "  <thumb aspect=\"poster\">%s</thumb>\n", xmlEscape(mi.PosterURL))
	}
	if mi.BackdropURL != "" {
		fmt.Fprintf(&b, "  <thumb aspect=\"fanart\">%s</thumb>\n", xmlEscape(mi.BackdropURL))
	}
	// 演员（Emby/Jellyfin/Kodi 人物）。Cast 来自 Detail 富化；Search 结果为空不影响。
	for i, a := range mi.Cast {
		fmt.Fprintf(&b, "  <actor>\n    <name>%s</name>\n", xmlEscape(a.Name))
		if a.Role != "" {
			fmt.Fprintf(&b, "    <role>%s</role>\n", xmlEscape(a.Role))
		}
		fmt.Fprintf(&b, "    <order>%d</order>\n", i)
		if a.Avatar != "" {
			fmt.Fprintf(&b, "    <thumb>%s</thumb>\n", xmlEscape(a.Avatar))
		}
		b.WriteString("  </actor>\n")
	}
	switch mi.Type {
	case TypeTV:
		b.WriteString("</tvshow>")
	case TypeMusic:
		b.WriteString("</musicvideo>")
	default:
		b.WriteString("</movie>")
	}
	return b.String()
}

func writeTag(b *strings.Builder, tag, val string) {
	val = strings.TrimSpace(val)
	if val == "" {
		return
	}
	val = xmlEscape(val)
	fmt.Fprintf(b, "  <%s>%s</%s>\n", tag, val, tag)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

// EpisodeNFO 生成单集 NFO（Kodi/Emby/Jellyfin <episodedetails>）。
// 无剧集级标题/剧情时，title 用剧名+季集兜底；媒体服务器仍可按 uniqueid+季集自行刮削补全。
func EpisodeNFO(mi *MediaInfo, season, episode int) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<episodedetails>\n")
	showTitle := mi.Title
	if showTitle == "" {
		showTitle = "Unknown"
	}
	title := showTitle
	if season > 0 && episode > 0 {
		title = fmt.Sprintf("%s S%02dE%02d", showTitle, season, episode)
	}
	writeTag(&b, "title", title)
	writeTag(&b, "showtitle", showTitle)
	if season > 0 {
		fmt.Fprintf(&b, "  <season>%d</season>\n", season)
	}
	if episode > 0 {
		fmt.Fprintf(&b, "  <episode>%d</episode>\n", episode)
	}
	if mi.Year > 0 {
		fmt.Fprintf(&b, "  <year>%d</year>\n", mi.Year)
	}
	writeTag(&b, "uniqueid", mi.Source+":"+mi.SourceID)
	b.WriteString("</episodedetails>")
	return b.String()
}
