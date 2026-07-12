package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOMDbSearch 跑通 OMDb ?t 按标题匹配：解析 imdbRating/imdbVotes 字符串为 Rating/VoteCount。
func TestOMDbSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("apikey") == "" {
			t.Errorf("missing apikey param")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Title": "Dune", "Year": "2021", "imdbID": "tt1160419", "Type": "movie",
			"imdbRating": "8.0", "imdbVotes": "500,000", "Response": "True",
		})
	}))
	defer srv.Close()

	o := NewOMDb("testkey")
	o.base = srv.URL // 同包可改私有 base 指向 httptest
	res, err := o.Search(context.Background(), "Dune", 2021)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	mi := res[0]
	if mi.Source != "imdb" || mi.SourceID != "tt1160419" {
		t.Errorf("source/id = %q/%q", mi.Source, mi.SourceID)
	}
	if mi.Rating != 8.0 {
		t.Errorf("Rating = %v, want 8.0", mi.Rating)
	}
	if mi.VoteCount != 500000 {
		t.Errorf("VoteCount = %d, want 500000", mi.VoteCount)
	}
}

// TestOMDbNotConfigured apiKey 为空时返回 ErrNotConfigured（Chain 据此跳过）。
func TestOMDbNotConfigured(t *testing.T) {
	o := NewOMDb("")
	_, err := o.Search(context.Background(), "x", 0)
	if err != ErrNotConfigured {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

// TestOMDbNoMatch Response=False 时返回 nil（无匹配，非错误）。
func TestOMDbNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Response": "False", "Error": "Movie not found!"})
	}))
	defer srv.Close()
	o := NewOMDb("k")
	o.base = srv.URL
	res, err := o.Search(context.Background(), " NoSuchMovie 999 ", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result on no match, got %+v", res)
	}
}

// TestParseVotesFloat 解析 OMDb 投票/评分字符串（含千分位逗号与 N/A）。
func TestParseVotesFloat(t *testing.T) {
	if got := parseVotes("1,234,567"); got != 1234567 {
		t.Errorf("parseVotes = %d", got)
	}
	if got := parseVotes("N/A"); got != 0 {
		t.Errorf("parseVotes N/A = %d", got)
	}
	if got := parseFloat("7.5"); got != 7.5 {
		t.Errorf("parseFloat = %v", got)
	}
	if got := parseFloat("N/A"); got != 0 {
		t.Errorf("parseFloat N/A = %v", got)
	}
}
