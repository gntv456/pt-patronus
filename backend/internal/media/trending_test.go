package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 豆瓣 frodo 热映榜（/subject_collection/movie_showing/items，item 字段扁平）。
func TestDoubanTrending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subject_collection/movie_showing/items" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"subject_collection_items":[
			{"type":"movie","id":"1","title":"热映电影","cover":{"url":"https://img9.doubanio.com/view/photo/m_ratio_poster/public/p1.jpg"},"rating":{"value":8.2},"year":"2024"}
		]}`))
	}))
	defer srv.Close()

	d := NewDouban("", "")
	d.base = srv.URL // 同包可访问私有 base

	res, err := d.Trending(context.Background(), "movie", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Title != "热映电影" || res[0].Source != "douban" || res[0].Type != TypeMovie {
		t.Errorf("douban trending = %+v", res)
	}
	if res[0].Rating != 8.2 {
		t.Errorf("rating = %g", res[0].Rating)
	}
}

// TMDB 每周趋势（/trending/movie/week）。端点结果无 media_type，按 category 定类型。
func TestTMDBTrending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/trending/movie/week" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":42,"title":"Trend Movie","poster_path":"/x.jpg","release_date":"2024-06-01","vote_average":7.5}
		]}`))
	}))
	defer srv.Close()

	tm := NewTMDB("k", "")
	overrideBase(tm, srv.URL+"/3")

	res, err := tm.Trending(context.Background(), "movie", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Title != "Trend Movie" || res[0].Type != TypeMovie || res[0].Year != 2024 {
		t.Errorf("tmdb trending = %+v", res)
	}
	if res[0].PosterURL != tmdbImgBase+"/x.jpg" {
		t.Errorf("poster = %q", res[0].PosterURL)
	}
}

// TMDB 未配置时 Trending 返回 ErrNotConfigured（RecommendChain 会跳过该源）。
func TestTMDBTrendingNotConfigured(t *testing.T) {
	tm := NewTMDB("", "")
	if _, err := tm.Trending(context.Background(), "movie", 1); err != ErrNotConfigured {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}
