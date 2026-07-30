package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTMDBDetailReturnsCompleteCastAndRecommendations(t *testing.T) {
	cast := make([]map[string]any, 24)
	for i := range cast {
		cast[i] = map[string]any{
			"name": fmt.Sprintf("Actor %d", i+1), "character": "Role", "profile_path": "/actor.jpg",
		}
	}
	recommendations := make([]map[string]any, 16)
	for i := range recommendations {
		recommendations[i] = map[string]any{
			"id": i + 1, "title": fmt.Sprintf("Recommendation %d", i+1), "poster_path": "/poster.jpg",
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 123, "title": "The Movie", "credits": map[string]any{"cast": cast},
			"recommendations": map[string]any{"results": recommendations},
		})
	}))
	defer srv.Close()

	tmdb := NewTMDB("test-key", "")
	overrideBase(tmdb, srv.URL)
	detail, err := tmdb.Detail(context.Background(), "movie:123")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Cast) != len(cast) {
		t.Fatalf("cast count = %d, want %d", len(detail.Cast), len(cast))
	}
	if len(detail.Recommendations) != len(recommendations) {
		t.Fatalf("recommendation count = %d, want %d", len(detail.Recommendations), len(recommendations))
	}
}

func TestTMDBSearchAndDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/multi":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": 123.0, "media_type": "movie", "title": "The Movie", "poster_path": "/abc.jpg", "release_date": "2023-05-01", "vote_average": 7.8},
					{"id": 456.0, "media_type": "tv", "name": "The Show", "first_air_date": "2022-01-01"},
					{"id": 789.0, "media_type": "person"}, // 应被过滤
				},
			})
		case "/3/movie/123":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 123.0, "title": "The Movie", "poster_path": "/abc.jpg",
				"overview": "A great film", "release_date": "2023-05-01", "vote_average": 7.8,
				"genres": []map[string]any{{"name": "Action"}, {"name": "Sci-Fi"}},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	tm := NewTMDB("test-key", "")
	// 重写 baseURL 到测试服务器
	overrideBase(tm, srv.URL+"/3")

	res, err := tm.Search(context.Background(), "movie", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results (person filtered), got %d", len(res))
	}
	if res[0].Title != "The Movie" || res[0].PosterURL != tmdbImgBase+"/abc.jpg" || res[0].Year != 2023 {
		t.Errorf("result0 = %+v", res[0])
	}
	if res[1].Type != TypeTV {
		t.Errorf("result1 type = %v", res[1].Type)
	}

	d, err := tm.Detail(context.Background(), "movie:123")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "The Movie" || d.Year != 2023 || len(d.Genres) != 2 {
		t.Errorf("detail = %+v", d)
	}
}

func TestTMDBNotConfigured(t *testing.T) {
	tm := NewTMDB("", "")
	if _, err := tm.Search(context.Background(), "x", 0); err != ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestNFO(t *testing.T) {
	xml := NFO(&MediaInfo{Source: "tmdb", SourceID: "1", Title: "A & B <x>", Year: 2023, Type: TypeMovie, Rating: 7.5, Genres: []string{"Action"}})
	if !contains(xml, "<movie>") || !contains(xml, "A &amp; B &lt;x&gt;") || !contains(xml, "<genre>Action</genre>") {
		t.Errorf("nfo = %s", xml)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// overrideBase 把 TMDB 客户端指向测试服务器（同包可访问私有字段）。
func overrideBase(t *TMDB, base string) { t.base = base }
