package subauto

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// recNotifier 记录收到的通知（Notifier 的内存替身）。
type recNotifier struct {
	mu   sync.Mutex
	seen []notice.Input
}

func (n *recNotifier) Notice(_ context.Context, in notice.Input, _ bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.seen = append(n.seen, in)
	return nil
}

func (n *recNotifier) bodies() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, 0, len(n.seen))
	for _, s := range n.seen {
		out = append(out, s.Body)
	}
	return out
}

// #307：自动化「没配上」以前只有一行 Debug，通知里只有计数；
// 且「各源都没这条字幕」被算成失败——两种处境在收件箱里长得一样。
func TestAutomationNoticeCarriesReason(t *testing.T) {
	// 1) 搜索报错 ⇒ 通知正文必须带原因
	dir := t.TempDir()
	video := writeVideo(t, dir, "Dead.2020.mkv")
	fs := &fakeSearcher{searchErr: errors.New("subhd: HTTP 404（请检查 cookie/登录 token 是否过期）")}
	rn := &recNotifier{}
	s := New(nil, fs, &memKV{m: map[string]string{}}, rn)
	if err := Save(s.kv, Config{Enabled: true, Languages: []string{"chs"}}); err != nil {
		t.Fatal(err)
	}
	s.process(MediaInfo{Title: "Dead", Year: 2020, Type: "movie", Files: []string{video}})
	got := rn.bodies()
	if len(got) != 1 {
		t.Fatalf("通知数 = %d，期望 1（%v）", len(got), got)
	}
	for _, want := range []string{"失败 1", "原因：", "404"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("正文丢了 %q: %q", want, got[0])
		}
	}
	if !strings.Contains(got[0], "未找到 0") {
		t.Errorf("正文应有「未找到」这一档: %q", got[0])
	}

	// 2) 正控：各源都答话但确实没字幕 ⇒ 算「未找到」，不发通知、不冒充失败
	dir2 := t.TempDir()
	video2 := writeVideo(t, dir2, "Rare.2021.mkv")
	fs2 := &fakeSearcher{subs: nil, data: nil}
	rn2 := &recNotifier{}
	s2 := New(nil, fs2, &memKV{m: map[string]string{}}, rn2)
	if err := Save(s2.kv, Config{Enabled: true, Languages: []string{"chs"}}); err != nil {
		t.Fatal(err)
	}
	s2.process(MediaInfo{Title: "Rare", Year: 2021, Type: "movie", Files: []string{video2}})
	if len(rn2.bodies()) != 0 {
		t.Fatalf("没搜到字幕不该发通知，got %v", rn2.bodies())
	}
	if _, reason := s2.fetchForFile(context.Background(), Config{Languages: []string{"chs"}}, video2, MediaInfo{Title: "Rare"}); reason != "" {
		t.Errorf("未找到不该带失败原因: %q", reason)
	}

	// 3) 正控：正常下载成功 ⇒ 正文是成功口径、没有「原因：」
	dir3 := t.TempDir()
	video3 := writeVideo(t, dir3, "Good.2022.mkv")
	fs3 := &fakeSearcher{
		subs: []subtitle.Subtitle{{Source: "subhd", SourceID: "9", LangCode: "chs", Ext: ".srt"}},
		data: []byte("1\n00:00:01 --> 00:00:02\nok\n"),
		name: "s.srt",
	}
	rn3 := &recNotifier{}
	s3 := New(nil, fs3, &memKV{m: map[string]string{}}, rn3)
	if err := Save(s3.kv, Config{Enabled: true, Languages: []string{"chs"}}); err != nil {
		t.Fatal(err)
	}
	s3.process(MediaInfo{Title: "Good", Year: 2022, Type: "movie", Files: []string{video3}})
	b := rn3.bodies()
	if len(b) != 1 || !strings.Contains(b[0], "下载 1") || strings.Contains(b[0], "原因：") {
		t.Errorf("成功口径不对: %v", b)
	}
}

// 上游「成功」但返回空内容：旧代码在这里记一条 err=<nil> 的下载失败，
// 通知里也只有计数 ⇒ 现在必须能在正文里看到「下载内容为空」。
func TestAutomationEmptyDownloadHasReason(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "Empty.2020.mkv")
	fs := &fakeSearcher{subs: []subtitle.Subtitle{{Source: "zimuku", SourceID: "1", LangCode: "chs", Ext: ".srt"}}}
	rn := &recNotifier{}
	s := New(nil, fs, &memKV{m: map[string]string{}}, rn)
	if err := Save(s.kv, Config{Enabled: true, Languages: []string{"chs"}}); err != nil {
		t.Fatal(err)
	}
	s.process(MediaInfo{Title: "Empty", Year: 2020, Type: "movie", Files: []string{video}})
	b := rn.bodies()
	if len(b) != 1 || !strings.Contains(b[0], "下载内容为空") {
		t.Fatalf("正文应说明是空内容而不是 err=<nil>，got %v", b)
	}
	if strings.Contains(b[0], "err=<nil>") {
		t.Errorf("原因句不该是空的: %v", b)
	}
}
