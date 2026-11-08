package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// startXGServer 起一个 mock 寸光集站点，按 path 返回探索列表 / 详情 JSON。
func startXGServer(t *testing.T, listJSON, detailJSON string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/explore", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(listJSON))
	})
	mux.HandleFunc("/api/explore/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(detailJSON))
	})
	return httptest.NewServer(mux)
}

func TestCunguangjiTrendingAndMapping(t *testing.T) {
	list := `{"items":[
		{"id":12,"title":"闪婚老伴是富豪","altTitle":"闪婚老伴","cover":"/api/img?url=http%3A%2F%2Fi1.letvimg.com%2Fa.jpg","description":"退休教授闪婚","releaseYear":2025,"rating":8.2,"tags":"逆袭,甜宠","region":"大陆","totalEpisodes":80,"status":"已完结"},
		{"id":13,"title":"无封面短剧","cover":"","releaseYear":2024,"rating":0,"tags":"","status":"连载中"}
	],"total":2}`
	srv := startXGServer(t, list, "{}")
	defer srv.Close()

	x := NewCunguangji(srv.URL)

	// Trending
	got, err := x.Trending(context.Background(), "duanju", 1)
	if err != nil {
		t.Fatalf("Trending: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	first := got[0]
	if first.Source != "cunguangji" || first.SourceID != "12" {
		t.Errorf("source/id mapping: %+v", first)
	}
	if first.Title != "闪婚老伴是富豪" || first.OriginalTitle != "闪婚老伴" {
		t.Errorf("title mapping: %+v", first)
	}
	if first.Type != TypeTV {
		t.Errorf("type should be tv, got %s", first.Type)
	}
	if first.Year != 2025 || first.Rating != 8.2 {
		t.Errorf("year/rating: %d %.1f", first.Year, first.Rating)
	}
	// 反代封面应解包为上游直链（resolvePoster 提速；非豆瓣上游不再走站点 /api/img）
	if want := "http://i1.letvimg.com/a.jpg"; first.PosterURL != want {
		t.Errorf("poster: got %q want %q", first.PosterURL, want)
	}
	if len(first.Genres) != 2 || first.Genres[0] != "逆袭" {
		t.Errorf("genres split: %+v", first.Genres)
	}
	// 状态应并入 overview
	if !strings.Contains(first.Overview, "已完结") {
		t.Errorf("status should be merged into overview: %q", first.Overview)
	}
	// 无封面项 PosterURL 为空
	if got[1].PosterURL != "" {
		t.Errorf("empty cover should stay empty, got %q", got[1].PosterURL)
	}

	// Trending 请求应带 sort=top
	// （通过单独 server 校验 query）
}

func TestCunguangjiSearchAndYearFilter(t *testing.T) {
	list := `{"items":[
		{"id":1,"title":"2023短剧","releaseYear":2023},
		{"id":2,"title":"2025短剧","releaseYear":2025}
	],"total":2}`
	srv := startXGServer(t, list, "{}")
	defer srv.Close()
	x := NewCunguangji(srv.URL)

	// 不带年份：两条都返回
	got, err := x.Search(context.Background(), "短剧", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	// 带年份 2025±1：只命中 2025
	got, _ = x.Search(context.Background(), "短剧", 2025)
	if len(got) != 1 || got[0].Year != 2025 {
		t.Fatalf("year filter 2025: got %+v", got)
	}
}

func TestCunguangjiDetailWithCasts(t *testing.T) {
	list := `{"items":[]}`
	detail := `{"id":42,"title":"龙王赘婿","altTitle":"","cover":"https://dj.hxpt.org/api/img?url=y","description":"龙王隐姓埋名","releaseYear":2025,"rating":9.0,"tags":"爽文","status":"连载中","casts":[{"name":"张三","role":"男主","avatar":"https://dj.hxpt.org/a.jpg"},{"name":"李四","role":"女主"}]}`
	srv := startXGServer(t, list, detail)
	defer srv.Close()
	x := NewCunguangji(srv.URL)

	mi, err := x.Detail(context.Background(), "42")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if mi.Title != "龙王赘婿" || mi.SourceID != "42" {
		t.Errorf("detail mapping: %+v", mi)
	}
	if len(mi.Cast) != 2 || mi.Cast[0].Name != "张三" || mi.Cast[0].Role != "男主" {
		t.Errorf("casts mapping: %+v", mi.Cast)
	}
}

// 校验 Trending/Search 请求确实带期望 query 参数（sort / q / page）。
func TestCunguangjiRequestParams(t *testing.T) {
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/explore") {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		got["path"] = r.URL.Path
		got["sort"] = q.Get("sort")
		got["q"] = q.Get("q")
		got["page"] = q.Get("page")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"total":0}`))
	}))
	defer srv.Close()
	x := NewCunguangji(srv.URL)

	_, _ = x.Trending(context.Background(), "duanju", 3)
	if got["sort"] != "top" || got["page"] != "3" {
		t.Errorf("Trending params: sort=%q page=%q", got["sort"], got["page"])
	}

	_, _ = x.Search(context.Background(), "赘婿", 0)
	if got["q"] != "赘婿" {
		t.Errorf("Search q param: %q", got["q"])
	}

	// 空 base → 默认 dj.hxpt.org，且尾斜杠被裁
	if x2 := NewCunguangji(""); x2.base != "https://dj.hxpt.org" {
		t.Errorf("default base: %q", x2.base)
	}
	if x2 := NewCunguangji("https://a.b/"); x2.base != "https://a.b" {
		t.Errorf("trailing slash trim: %q", x2.base)
	}
}

// 确保 Categories/Types 声明正确（CategoryCapable + TypedSource）。
func TestCunguangjiCapabilities(t *testing.T) {
	x := NewCunguangji("")
	if len(x.Categories()) != 1 || x.Categories()[0] != "duanju" {
		t.Errorf("Categories: %+v", x.Categories())
	}
	if len(x.Types()) != 1 || x.Types()[0] != TypeTV {
		t.Errorf("Types: %+v", x.Types())
	}
	// 便捷：JSON 编码不报错（结构体可序列化）
	if _, err := json.Marshal(xgItem{ID: 1, Title: "x"}); err != nil {
		t.Errorf("marshal xgItem: %v", err)
	}
}

// resolvePoster：解包站点 /api/img 反代为上游直链（提速）；doubanio 保留反代；静态图原样。
func TestCunguangjiResolvePoster(t *testing.T) {
	x := NewCunguangji("")
	cases := []struct{ in, want string }{
		// 站内相对反代 → 解包上游直链
		{"/api/img?url=https%3A%2F%2Fp6-novel.byteimg.com%2Fnovel-pic%2Fa.jpg", "https://p6-novel.byteimg.com/novel-pic/a.jpg"},
		// doubanio 保留反代（防盗链）
		{"/api/img?url=https%3A%2F%2Fimg2.doubanio.com%2Fview%2Fphoto%2Fp.jpg", "https://dj.hxpt.org/api/img?url=https%3A%2F%2Fimg2.doubanio.com%2Fview%2Fphoto%2Fp.jpg"},
		// 站内静态图（无 url 参数）→ 拼 base
		{"/_nuxt/img/cover.png", "https://dj.hxpt.org/_nuxt/img/cover.png"},
		// 绝对 URL 原样
		{"https://example.com/a.jpg", "https://example.com/a.jpg"},
		// 空
		{"", ""},
	}
	for _, c := range cases {
		if got := x.resolvePoster(c.in); got != c.want {
			t.Errorf("resolvePoster(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
