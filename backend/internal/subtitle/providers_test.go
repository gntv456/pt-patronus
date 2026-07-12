package subtitle

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// fakeDoer 按 url 子串返回预设响应体；authH 捕获鉴权头。
type fakeDoer struct {
	bodies map[string]string
	authH  *string
}

func (f *fakeDoer) Do(_ context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	if f.authH != nil {
		if v, ok := headers["Api-Key"]; ok {
			*f.authH = v
		}
	}
	for sub, b := range f.bodies {
		if strings.Contains(rawURL, sub) {
			return &httpclient.Response{StatusCode: 200, Body: []byte(b)}, nil
		}
	}
	return &httpclient.Response{StatusCode: 200, Body: []byte(`{}`)}, nil
}

func TestAssrtSearchAndDownload(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"/v1/sub/search": `{"status":0,"sub":{"subs":[
			{"id":"100","native_name":"电影A 简体中文","videoname":"MovieA","lang":{"langlistdesc":"简体中文"}},
			{"id":"101","native_name":"电影B","lang":{"langlistdesc":"繁體中文"}}
		]}}`,
		"/v1/sub/detail": `{"status":0,"sub":{"subs":[{"filelist":[{"url":"/tools/export/abc.zip","f":"A.srt"}]}]}}`,
		"/tools/export/abc.zip": "ZIPBYTES",
	}}
	p := NewAssrt("tok123", doer)
	subs, err := p.Search(context.Background(), "电影A", SearchOpts{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("want 2 subs, got %d", len(subs))
	}
	if subs[0].SourceID != "100" || subs[0].LangCode != "chs" {
		t.Errorf("subs[0] wrong: %+v", subs[0])
	}
	if subs[1].LangCode != "cht" {
		t.Errorf("subs[1] langcode=%q want cht", subs[1].LangCode)
	}
	// 未配置 → ErrNotConfigured
	if _, err := NewAssrt("", doer).Search(context.Background(), "x", SearchOpts{}); err != ErrNotConfigured {
		t.Errorf("empty token want ErrNotConfigured, got %v", err)
	}
	// 下载链路
	data, name, err := p.Download(context.Background(), subs[0])
	if err != nil {
		t.Fatal(err)
	}
	if name != "A.srt" || string(data) != "ZIPBYTES" {
		t.Errorf("download wrong: name=%q data=%q", name, data)
	}
}

func TestOpenSubtitlesSearch(t *testing.T) {
	var auth string
	doer := &fakeDoer{authH: &auth, bodies: map[string]string{
		"/api/v1/subtitles": `{"data":[
			{"id":"s1","attributes":{"file_name":"Movie.en.srt","language":"en","download_count":42,"files":[{"file_id":"5566","file_name":"Movie.en.srt"}]}}
		]}`,
	}}
	p := NewOpenSubtitles("OSKEY", doer)
	subs, err := p.Search(context.Background(), "Movie", SearchOpts{Language: "eng"})
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].SourceID != "5566" || subs[0].LangCode != "eng" || subs[0].Score != 42 {
		t.Errorf("osd sub wrong: %+v", subs)
	}
	if auth != "OSKEY" {
		t.Errorf("Api-Key header not sent: %q", auth)
	}
	if _, err := NewOpenSubtitles("", doer).Search(context.Background(), "x", SearchOpts{}); err != ErrNotConfigured {
		t.Errorf("empty key want ErrNotConfigured, got %v", err)
	}
}

func TestSubHDSearch(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"/search/": `<html><body>
			<a href="/d/abc123">电影A 简繁双语字幕</a>
			<a href="/d/def456">电影B 英文</a>
			<a href="/d/abc123">重复项应去重</a>
		</body></html>`,
	}}
	p := NewSubHD("", doer)
	subs, err := p.Search(context.Background(), "电影", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("want 2 (去重后), got %d: %+v", len(subs), subs)
	}
	if subs[0].SourceID != "abc123" || subs[0].LangCode != "bi" {
		t.Errorf("subhd[0] wrong: %+v", subs[0])
	}
}

func TestZimukuSearch(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"/search?": `<html><body>
			<a href="/detail/duanju-2024.html">短剧 简体</a>
			<a href="/detail/movie-99.html">某电影</a>
		</body></html>`,
	}}
	p := NewZimuku("", doer)
	subs, err := p.Search(context.Background(), "短剧", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("want 2, got %d", len(subs))
	}
	if subs[0].SourceID != "duanju-2024" {
		t.Errorf("zimuku id wrong: %+v", subs[0])
	}
}
