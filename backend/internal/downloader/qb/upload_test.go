// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package qb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// TestQBAddTorrentFile 验证 .torrent 文件走 multipart 上传，且首次 403 会话过期时
// 能重登并重建 body 重试成功（doBody 的 bodyFactory 重建路径，见方案 G.5）。
func TestQBAddTorrentFile(t *testing.T) {
	var (
		addCalls     int
		loginCount   int
		gotMultipart bool
		gotFilename  string
		gotCategory  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			loginCount++
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{},"server_state":{},"tags":[]}`))
		case "/api/v2/torrents/add":
			addCalls++
			if ct := r.Header.Get("Content-Type"); strings.HasPrefix(ct, "multipart/form-data") {
				gotMultipart = true
				_ = r.ParseMultipartForm(8 << 20)
				if r.MultipartForm != nil {
					if fh := r.MultipartForm.File["torrents"]; len(fh) > 0 {
						gotFilename = fh[0].Filename
					}
					gotCategory = r.MultipartForm.Value["category"][0]
				}
			}
			if addCalls == 1 {
				w.WriteHeader(403) // 首次会话过期 → 触发重登重试
				return
			}
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("test conn: %v", err)
	}

	if err := c.AddTorrent(context.Background(), downloader.AddOptions{
		FileContent: []byte("d8:announceFakeTorrentBytes"),
		FileName:    "movie.torrent",
		Category:    "movies",
	}); err != nil {
		t.Fatalf("add file: %v", err)
	}

	if !gotMultipart {
		t.Error("expected multipart/form-data upload")
	}
	if gotFilename != "movie.torrent" {
		t.Errorf("filename = %q, want movie.torrent", gotFilename)
	}
	if gotCategory != "movies" {
		t.Errorf("category = %q, want movies", gotCategory)
	}
	if addCalls != 2 {
		t.Errorf("expected 2 add calls (403→relogin→retry), got %d", addCalls)
	}
	if loginCount < 2 {
		t.Errorf("expected relogin on 403, loginCount=%d", loginCount)
	}
}

// TestQBCategories 验证 ListCategories/ListTags 零请求来自同步状态。
func TestQBCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{
				"rid":1,"full_update":true,"torrents":{},
				"categories":{"movies":{"name":"movies"},"tv":{"name":"tv"}},
				"tags":["seed","hd"],
				"server_state":{}
			}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "u", "p")
	_ = c.TestConnection(context.Background())

	cats, err := c.ListCategories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 2 {
		t.Errorf("categories = %v", cats)
	}
	tags, err := c.ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "seed" {
		t.Errorf("tags = %v", tags)
	}
}
