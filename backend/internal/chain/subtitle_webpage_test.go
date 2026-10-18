package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// TestSubtitleChainDownloadRejectsWebPage 接线验证：护栏不是只在纯函数里绿。
// 手动下载、健康中心「补齐缺失字幕」、追更自动补字幕三条写入路径都汇到
// SubtitleChain.Download → subtitle.Extract，所以字幕站返回 200 + 登录墙网页时，
// 这里必须返回错误且不交出可写入的字节——否则会往媒体库落一个装网页的 .srt，
// 播放器没字幕、健康中心还从此判定「字幕已存在」。
func TestSubtitleChainDownloadRejectsWebPage(t *testing.T) {
	page := &fakeSubProvider{
		name:   "subhd",
		dlData: []byte("<!DOCTYPE html>\n<html><head><title>请登录</title></head><body>登录后可下载</body></html>"),
		dlName: "12345",
	}
	p := newSubtitleTestProvider(t, "", page)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)

	data, name, err := ch.Download(context.Background(), subtitle.Subtitle{Source: "subhd", SourceID: "12345"})
	if err == nil {
		t.Fatalf("网页内容必须报错，got name=%q len=%d", name, len(data))
	}
	if !strings.Contains(err.Error(), "网页") {
		t.Errorf("错误应说明是网页，got %v", err)
	}
	if data != nil || name != "" {
		t.Errorf("拒绝时不得交出可写入内容，got name=%q len=%d", name, len(data))
	}

	// 正控：真字幕走同一条链照常返回（证明不是把下载整体堵死）。
	real := &fakeSubProvider{
		name:   "zimuku",
		dlData: []byte("1\n00:00:01,000 --> 00:00:02,000\n字幕正文\n"),
		dlName: "movie.srt",
	}
	p2 := newSubtitleTestProvider(t, "", real)
	ch2 := NewSubtitleChain(NewBase(module.New(), event.New()), p2)
	data2, name2, err2 := ch2.Download(context.Background(), subtitle.Subtitle{Source: "zimuku", SourceID: "9"})
	if err2 != nil {
		t.Fatalf("正常字幕不应报错: %v", err2)
	}
	if !strings.Contains(string(data2), "字幕正文") || name2 != "movie.srt" {
		t.Fatalf("应返回字幕内容，got name=%q data=%q", name2, data2)
	}
}
