// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package transfer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMergeWordsLocalWins(t *testing.T) {
	local := []IdentifyWord{
		{Kind: "replace", From: "【A组】", To: ""},
		{Kind: "trim", From: "prefix-"},
	}
	incoming := []IdentifyWord{
		{Kind: "replace", From: "【A组】", To: "KEEP_LOCAL"}, // 同 key → 本地优先
		{Kind: "replace", From: "【B组】", To: ""},           // 新增
		{Kind: "replace", From: "【B组】", To: "dup"},        // 仓库内重复 → 合并侧也去重
	}
	merged, added := MergeWords(local, incoming)
	if added != 1 {
		t.Fatalf("added=%d want 1", added)
	}
	if len(merged) != 3 || merged[0].To != "" {
		t.Fatalf("merged=%+v", merged)
	}
}

func TestMergeWordsEmpty(t *testing.T) {
	merged, added := MergeWords(nil, nil)
	// MergeWords 返回空切片（非 nil）——make(len(existing)) 语义；条数与新增为 0 即正确。
	if len(merged) != 0 || added != 0 {
		t.Fatalf("merge empty: %+v %d", merged, added)
	}
}

func TestFetchSharedRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"words":[
			{"kind":"replace","from":"【C组】","to":"","note":"云词条"},
			{"kind":"bogus","from":"x","to":"y"},
			{"kind":"replace","from":"","to":"z"}
		]}`))
	}))
	defer srv.Close()
	words, err := FetchSharedRepo(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// 非法 kind / 空 from 被过滤
	if len(words) != 1 || words[0].From != "【C组】" {
		t.Fatalf("words=%+v", words)
	}
}

func TestFetchSharedRepoBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	if _, err := FetchSharedRepo(context.Background(), srv.URL); err == nil {
		t.Fatal("bad json must error")
	}
}

func TestFetchSharedRepoEmptyURL(t *testing.T) {
	if _, err := FetchSharedRepo(context.Background(), ""); err == nil {
		t.Fatal("empty url must error")
	}
}
