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
