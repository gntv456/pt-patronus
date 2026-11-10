// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestTMDBVoteCount TMDB 搜索响应含 vote_count，应解析到 MediaInfo.VoteCount。
func TestTMDBVoteCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[` +
			`{"id":1,"media_type":"movie","title":"Dune","release_date":"2021-10-22","vote_average":8.0,"vote_count":15000}` +
			`]}`))
	}))
	defer srv.Close()
	tm := NewTMDB("k", "")
	tm.base = srv.URL
	res, err := tm.Search(context.Background(), "Dune", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0].VoteCount != 15000 || res[0].Rating != 8.0 {
		t.Fatalf("got %+v", res)
	}
}

// TestDoubanVoteCount 豆瓣搜索 rating.count 应解析到 VoteCount。
func TestDoubanVoteCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subjects":{"items":[` +
			`{"target_type":"movie","target":{"id":"1","title":"Dune","rating":{"value":7.5,"count":200000}}}` +
			`]}}`))
	}))
	defer srv.Close()
	d := NewDouban("", "")
	d.base = srv.URL
	res, err := d.Search(context.Background(), "Dune", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0].VoteCount != 200000 || res[0].Rating != 7.5 {
		t.Fatalf("got %+v", res)
	}
}

// TestBangumiVoteCount Bangumi rating.total 应解析到 VoteCount。
func TestBangumiVoteCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[` +
			`{"id":1,"name":"Anime","rating":{"score":8.2,"total":3000},"date":"2021-01-01"}` +
			`]}`))
	}))
	defer srv.Close()
	b := NewBangumi("")
	b.base = srv.URL
	res, err := b.Search(context.Background(), "Anime", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0].VoteCount != 3000 || res[0].Rating != 8.2 {
		t.Fatalf("got %+v", res)
	}
}
