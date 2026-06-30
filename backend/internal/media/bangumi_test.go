package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func bgmHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v0/search/subjects":
			_, _ = w.Write([]byte(`{"data":[
				{"id":12345,"name":"Anime EN","name_cn":"动漫中文名","summary":"简介","date":"2024-04-01","type":2,"nsfw":false,"rating":{"score":8.5},"images":{"large":"//lain.bgm.tv/r/400/pic/cover/l/xx/12345.jpg"}},
				{"id":67890,"name":"Real Show","name_cn":"","summary":"","date":"2023-01-01","type":6,"rating":{"score":7.0},"images":{"large":"//lain.bgm.tv/r/400/pic/cover/l/yy/67890.jpg"}}
			]}`))
		case "/v0/subjects/12345":
			_, _ = w.Write([]byte(`{"id":12345,"name":"Anime EN","name_cn":"动漫中文名","summary":"详情简介","date":"2024-04-01","type":2,"rating":{"score":8.5},"images":{"large":"//lain.bgm.tv/r/400/pic/cover/l/xx/12345.jpg"}}`))
		default:
			w.WriteHeader(404)
		}
	})
}

func overrideBangumiBase(b *Bangumi, base string) { b.base = base }

func TestBangumiSearchAndDetail(t *testing.T) {
	srv := httptest.NewServer(bgmHandler(t))
	defer srv.Close()

	b := NewBangumi("")
	overrideBangumiBase(b, srv.URL)

	res, err := b.Search(context.Background(), "kw", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Source != "bangumi" || res[0].SourceID != "bangumi:12345" {
		t.Errorf("result0 = %+v", res[0])
	}
	if res[0].Title != "动漫中文名" || res[0].Type != TypeTV || res[0].Year != 2024 {
		t.Errorf("result0 mapping = %+v", res[0])
	}
	if res[1].Title != "Real Show" { // name_cn 空 → 回退 name
		t.Errorf("result1 title = %q", res[1].Title)
	}
	if res[0].PosterURL != "https://lain.bgm.tv/r/400/pic/cover/l/xx/12345.jpg" {
		t.Errorf("poster_url = %q", res[0].PosterURL)
	}

	d, err := b.Detail(context.Background(), "bangumi:12345")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "动漫中文名" || d.Overview != "详情简介" {
		t.Errorf("detail = %+v", d)
	}
}

func TestBangumiTrending(t *testing.T) {
	srv := httptest.NewServer(bgmHandler(t))
	defer srv.Close()

	b := NewBangumi("tok")
	overrideBangumiBase(b, srv.URL)

	res, err := b.Trending(context.Background(), "anime", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Source != "bangumi" {
		t.Errorf("trending = %+v", res)
	}
}

func TestBangumiYearTolerance(t *testing.T) {
	srv := httptest.NewServer(bgmHandler(t))
	defer srv.Close()
	b := NewBangumi("")
	overrideBangumiBase(b, srv.URL)

	// 查 2020 年：12345(2024) 与 67890(2023) 均超 ±1 容差 → 全过滤
	res, err := b.Search(context.Background(), "kw", 2020)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("year tolerance should filter all, got %d", len(res))
	}
}
