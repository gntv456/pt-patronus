package mediaserver

import "testing"

func TestMatchPlaybackEmby(t *testing.T) {
	cases := []struct {
		path string
		want bool
		id   string
	}{
		{"/emby/videos/12345/original.mkv", true, "12345"},
		{"/Videos/12345/original.mp4?MediaSourceId=x", true, "12345"},
		{"/jellyfin/videos/99/stream", true, "99"},
		{"/videos/88/stream.mp4", true, "88"},
		{"/emby/audio/77/original.mp3", true, "77"},
		{"/emby/videos/12345/master.m3u8", false, ""},     // 转码流不拦截
		{"/emby/Users/1/Items", false, ""},                // 元数据
		{"/emby/videos/../admin/original.mkv", false, ""}, // 路径异常不拦截
		{"/emby/videos//original.mkv", false, ""},         // 空 id
		{"/emby/System/Info", false, ""},
	}
	for _, c := range cases {
		tgt, ok := MatchPlaybackEmby(c.path)
		if ok != c.want {
			t.Errorf("MatchPlaybackEmby(%q)=%v want %v", c.path, ok, c.want)
			continue
		}
		if ok && tgt.ItemID != c.id {
			t.Errorf("path %q id=%q want %q", c.path, tgt.ItemID, c.id)
		}
	}
}
