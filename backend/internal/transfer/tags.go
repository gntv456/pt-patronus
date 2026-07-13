package transfer

import "strings"

// 媒体身份标签编解码：subscribe/rss/workflow 推送种子时把媒体身份编码进 qBittorrent/Transmission
// 标签（AddTorrent 不返回 hash，无法在推送时落 DownloadMediaLink；标签是最可靠的传输载体）。
// 整理时读标签 → 秒级识别（Path A），无需再按文件名搜索。未带标签则回退文件名搜索（Path B）。
//
// 格式：ptpsrc|<source>|<sourceID>|<type>（sourceID 内部不含 |，故用 | 分隔）。
const mediaTagPrefix = "ptpsrc|"

// EncodeMediaTag 把媒体身份编码为标签。
func EncodeMediaTag(source, sourceID, typ string) string {
	return mediaTagPrefix + source + "|" + sourceID + "|" + typ
}

// DecodeMediaTag 从单个标签解码媒体身份。非 ptpsrc 标签返回 ok=false。
func DecodeMediaTag(tag string) (source, sourceID, typ string, ok bool) {
	if !strings.HasPrefix(tag, mediaTagPrefix) {
		return "", "", "", false
	}
	rest := strings.TrimPrefix(tag, mediaTagPrefix)
	parts := strings.SplitN(rest, "|", 3)
	if len(parts) < 2 {
		return "", "", "", false
	}
	if len(parts) >= 3 {
		return parts[0], parts[1], parts[2], true
	}
	return parts[0], parts[1], "", true
}

// FindMediaHint 在标签切片里找首个 ptpsrc 标签并解码为 MediaLink。
func FindMediaHint(tags []string) *MediaLink {
	for _, t := range tags {
		if s, id, typ, ok := DecodeMediaTag(t); ok {
			return &MediaLink{Source: s, SourceID: id, Type: typ}
		}
	}
	return nil
}
