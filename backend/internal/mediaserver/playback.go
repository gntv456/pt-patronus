package mediaserver

import (
	"context"
	"fmt"
	"strings"
)

// ---- 播放请求拦截（媒体代理 302 直链） ----
//
// 反代 Emby/JF 时按 path 判断是否「原始流播放」请求：命中则返回 item id 供代理
// 查 MediaPath → 云盘直链 302；未命中返回 ok=false 由代理原样转发。Plex 协议独立
// （transcode/universal 决策 + 直接 file 路径），保守不拦截。

// PlaybackTarget 一次待直链的播放请求。
type PlaybackTarget struct {
	ItemID  string // Emby/JF item id
	StartAt string // 原始 query（透传或鉴权用）
}

// MatchPlaybackEmby 判断反代请求路径是否 Emby/Jellyfin 原始流播放。
// 命中模式（Emby 4.x/JF 10.x 通用）：
//
//	/emby/videos/{id}/original.mkv | /Videos/{id}/original.mkv | /videos/{id}/stream(.*)?
//	/emby/Audio/{id}/original.mp3（音频同理）
func MatchPlaybackEmby(path string) (PlaybackTarget, bool) {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	// 去掉可选前缀 emby/jellyfin
	if len(segs) > 0 && (segs[0] == "emby" || segs[0] == "jellyfin") {
		segs = segs[1:]
	}
	if len(segs) < 3 {
		return PlaybackTarget{}, false
	}
	kind := strings.ToLower(segs[0])
	if kind != "videos" && kind != "audio" {
		return PlaybackTarget{}, false
	}
	id := segs[1]
	file := strings.ToLower(segs[2])
	isOriginal := strings.HasPrefix(file, "original.") ||
		(file == "stream" || strings.HasPrefix(file, "stream?") || strings.HasPrefix(file, "stream."))
	if !isOriginal || id == "" {
		return PlaybackTarget{}, false
	}
	if strings.ContainsAny(id, "./?&=;") { // id 非纯 token（路径异常）不拦截
		return PlaybackTarget{}, false
	}
	return PlaybackTarget{ItemID: id}, true
}

// MediaPath 查询条目的媒体文件路径（代理按路径找云盘挂载/STRM 对应直链）。
// GET /Items/{id}/PlaybackInfo 仅取 MediaSources[0].Path。
func (c *embyJellyfinClient) MediaPath(ctx context.Context, itemID string) (string, error) {
	u := fmt.Sprintf("%s/Items/%s/PlaybackInfo", c.apiRoot, esc(itemID))
	var res struct {
		MediaSources []struct {
			Path string `json:"Path"`
		} `json:"MediaSources"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return "", err
	}
	if len(res.MediaSources) == 0 || res.MediaSources[0].Path == "" {
		return "", fmt.Errorf("item %s 无媒体源", itemID)
	}
	return res.MediaSources[0].Path, nil
}

// EpisodeCount 剧集已入库集数（健康中心缺集判定用）。
func (c *embyJellyfinClient) EpisodeCount(ctx context.Context, seriesID string) (int, error) {
	u := fmt.Sprintf("%s/Shows/%s/Episodes", c.apiRoot, esc(seriesID))
	var res struct {
		Total int `json:"TotalRecordCount"`
		Items []struct {
			ID string `json:"Id"`
		} `json:"Items"`
	}
	if err := jsonGet(ctx, c.hc, u, c.headers(), &res); err != nil {
		return 0, err
	}
	if res.Total > 0 {
		return res.Total, nil
	}
	return len(res.Items), nil
}
