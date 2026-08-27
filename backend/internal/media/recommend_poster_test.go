package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 豆瓣 frodo 榜单（2026 起）封面字段是 pic{large,normal}；cover 对象已不存在于 top250/weekly。
// 用 topMovie 验证 pic 解析 + doubanPosterURL 直通，用 weekly 复用同一解析路径。
func TestDoubanTrendingTopMovieParsesPicPoster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subject_collection/movie_top250/items" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":2,"subject_collection_items":[
			{"type":"movie","id":"1292052","title":"肖申克的救赎","year":"1994","card_subtitle":"1994 / 美国 / 剧情","rating":{"value":9.7},"pic":{"large":"https://img3.doubanio.com/view/photo/m_ratio_poster/public/p2934829882.jpg","normal":"https://img3.doubanio.com/view/photo/s_ratio_poster/public/p2934829882.jpg"}},
			{"type":"movie","id":"1","title":"无封面条目","cover":{"url":""}}
		]}`))
	}))
	defer srv.Close()

	d := NewDouban("", "")
	d.base = srv.URL

	res, err := d.Trending(context.Background(), "topMovie", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("count = %d, want 2", len(res))
	}
	// doubanPosterURL 把 photo 文件名统一换算成 img9 m_ratio_poster 竖版地址
	want := "https://img9.doubanio.com/view/photo/m_ratio_poster/public/p2934829882.jpg"
	if res[0].PosterURL != want {
		t.Errorf("poster = %q, want %q", res[0].PosterURL, want)
	}
	if res[0].Year != 1994 || res[0].Rating != 9.7 {
		t.Errorf("year/rating = %d/%g", res[0].Year, res[0].Rating)
	}
}

// trendingTopTv 走网页版 /j/search_subjects（frodo 无剧集 Top250 榜单）：翻页聚合 → 过滤
// rate≥8.5 → 按 rate 降序取前 20。同时校验 webItem.Cover 经 doubanPosterURL 规整。
func TestDoubanTrendingTopTvAggregatesWebSearch(t *testing.T) {
	pageStartSeen := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/j/search_subjects" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("type") != "tv" || q.Get("tag") == "" {
			t.Errorf("query = %v", q)
		}
		pageStartSeen[q.Get("page_start")] = true
		start := q.Get("page_start")
		w.Header().Set("Content-Type", "application/json")
		switch start {
		case "0":
			items := []map[string]any{}
			for i := 0; i < 50; i++ {
				rate := fmt.Sprintf("%g", 6.0+float64(i%30)/10) // 6.0~8.9，>=8.5 的占少数
				items = append(items, map[string]any{
					"id": fmt.Sprintf("a%d", i), "title": fmt.Sprintf("剧A%d", i),
					"rate": rate, "cover": fmt.Sprintf("https://img1.doubanio.com/view/photo/s_ratio_poster/public/p%d.jpg", i),
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"subjects": items})
		case "50":
			_ = json.NewEncoder(w).Encode(map[string]any{"subjects": []map[string]any{
				{"id": "b1", "title": "剧B满分", "rate": "9.5", "cover": "https://img9.doubanio.com/view/photo/m_ratio_poster/public/p999.jpg"},
				{"id": "", "title": "缺ID应被跳过", "rate": "9.0"},
				{"id": "b3", "title": "无分应被跳过", "rate": ""},
			}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"subjects": []map[string]any{}})
		}
	}))
	defer srv.Close()

	d := NewDouban("", "")
	d.webBase = srv.URL // 同包可访问私有字段；getWeb 走网页版 host

	res, err := d.Trending(context.Background(), "topTv", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || len(res) > 20 {
		t.Fatalf("count = %d, want 1..20", len(res))
	}
	// 满分条目必须排第一
	if res[0].Title != "剧B满分" || res[0].Rating != 9.5 {
		t.Errorf("first = %+v", res[0])
	}
	if res[0].SourceID != "tv:b1" {
		t.Errorf("source_id = %q, want tv:b1", res[0].SourceID)
	}
	if res[0].Type != TypeTV {
		t.Errorf("type = %q", res[0].Type)
	}
	// 翻页至少到第 2 页（第一页 50 条已满）
	if !pageStartSeen["0"] || !pageStartSeen["50"] {
		t.Errorf("pages seen = %v, want 0 and 50", pageStartSeen)
	}
	// 第二页：空档期（start 超界）返回空切片而非错误
	res2, err := d.Trending(context.Background(), "topTv", 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2) != 0 {
		t.Errorf("page99 count = %d, want 0", len(res2))
	}
}

// trendingTopTv 入围条目会并发调 frodo Detail 富化——卡片才有 年份/类型/产地/简介 等
// 字段（此前只有 title/rate/cover，比电影 Top250 少一整行）。Detail 失败的条目保留
// 裸数据仍在榜。Base 同时覆盖 /j/search_subjects（web）与 /tv/{id}（frodo），用同一个
// httptest.Server + 双 base 覆盖（d.base / d.webBase）。
func TestDoubanTrendingTopTvEnrichesFromDetail(t *testing.T) {
	mux := http.NewServeMux()
	detailHits := map[string]int{}
	mux.HandleFunc("/j/search_subjects", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subjects":[
			{"id":"t1","title":"富化成功剧","rate":"9.2","cover":"https://img9.doubanio.com/view/photo/m_ratio_poster/public/p1.jpg"},
			{"id":"t2","title":"富化失败剧","rate":"8.8","cover":"https://img1.doubanio.com/view/photo/s_ratio_poster/public/p2.jpg"}
		]}`))
	})
	mux.HandleFunc("/tv/t1", func(w http.ResponseWriter, r *http.Request) {
		detailHits["t1"]++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"富化成功剧","year":2005,"summary":"经典神作简介","rating":{"value":9.5,"count":100},
			"genres":["剧情","悬疑"],"countries":["中国大陆"],"directors":[{"name":"某导演"}],
			"pubdate":["2005-01-01"],"pic":{"large":"https://img9.doubanio.com/view/photo/m_ratio_poster/public/p9.jpg"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewDouban("", "")
	d.base, d.webBase = srv.URL, srv.URL

	res, err := d.Trending(context.Background(), "topTv", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("count = %d, want 2", len(res))
	}
	// 按 rate 排序：t1(9.2) 在前
	first := res[0]
	if first.Title != "富化成功剧" {
		t.Fatalf("first = %+v", first)
	}
	if detailHits["t1"] != 1 {
		t.Errorf("detail called %d times for t1, want 1", detailHits["t1"])
	}
	if first.Year != 2005 || first.Overview == "" || first.ReleaseDate == "" ||
		len(first.Genres) != 2 || len(first.Countries) != 1 || len(first.Directors) != 1 {
		t.Errorf("enriched fields missing: year=%d overview=%q release=%q genres=%v countries=%v directors=%v",
			first.Year, first.Overview, first.ReleaseDate, first.Genres, first.Countries, first.Directors)
	}
	if first.Rating < 9.2 { // Detail 的 9.5 可用；榜单兜底 9.2 至少保留一个
		t.Errorf("rating lost: %v", first.Rating)
	}
	// 失败条目仍以裸数据上榜（海报经 doubanPosterURL 规整、评分/标题保留榜单值）
	second := res[1]
	if second.Title != "富化失败剧" || second.Year != 0 || second.Rating != 8.8 ||
		second.PosterURL != "https://img9.doubanio.com/view/photo/m_ratio_poster/public/p2.jpg" {
		t.Errorf("fallback item wrong: %+v", second)
	}
}

// 订阅日历需带作品海报（缺失时退 next_episode_to_air.still_path），前端日历行展示缩略图。
func TestTMDBCalendarForSubscriptionsIncludesPoster(t *testing.T) {
	dayA := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	dayB := time.Now().UTC().AddDate(0, 0, 5).Format("2006-01-02")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/tv/100":
			_, _ = w.Write([]byte(`{"name":"有海报剧","poster_path":"/poster.jpg","next_episode_to_air":{"season_number":2,"episode_number":5,"air_date":"` + dayA + `","overview":"下一集"}}`))
		case "/3/tv/200":
			_, _ = w.Write([]byte(`{"name":"无海报剧","poster_path":"","next_episode_to_air":{"season_number":1,"episode_number":2,"air_date":"` + dayB + `","overview":"","still_path":"/still.jpg"}}`))
		case "/3/tv/300":
			_, _ = w.Write([]byte(`{"name":"不开播"}`)) // 无 next → 跳过
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tm := NewTMDB("k", "")
	overrideBase(tm, srv.URL+"/3")

	cal, err := tm.CalendarForSubscriptions(context.Background(), []string{"100", "200", "300"}, 30)
	if err != nil {
		t.Fatal(err)
	}
	day1 := cal[dayA]
	if len(day1) != 1 || day1[0].PosterURL != tmdbImgBase+"/poster.jpg" {
		t.Errorf("day1 = %+v", day1)
	}
	if len(day1) == 1 && (day1[0].Season != 2 || day1[0].Episode != 5) {
		t.Errorf("s/e = %d/%d", day1[0].Season, day1[0].Episode)
	}
	day2 := cal[dayB]
	if len(day2) != 1 || day2[0].PosterURL != tmdbImgBase+"/still.jpg" {
		t.Errorf("day2 (still fallback) = %+v", day2)
	}
	if len(cal) != 2 {
		t.Errorf("calendar days = %d, want 2 (subject without next_episode skipped)", len(cal))
	}
}
