// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebDAVList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/data/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat>
  </d:response>
  <d:response>
    <d:href>/data/movie.mkv</d:href>
    <d:propstat><d:prop><d:displayname>movie.mkv</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/><d:getlastmodified>Mon, 02 Jan 2023 15:04:05 GMT</d:getlastmodified></d:prop></d:propstat>
  </d:response>
</d:multistatus>`))
			return
		}
		if r.Method == "MKCOL" {
			w.WriteHeader(201)
			return
		}
		if r.Method == "PUT" {
			w.WriteHeader(201)
			return
		}
		if r.Method == "GET" {
			_, _ = w.Write([]byte("file-content"))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	w := NewWebDAV(srv.URL, "", "", "")
	entries, err := w.List(context.Background(), "/data")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "movie.mkv" || entries[0].Size != 1024 {
		t.Errorf("entries = %+v", entries)
	}

	if err := w.Mkdir(context.Background(), "/data/new"); err != nil {
		t.Errorf("mkdir: %v", err)
	}
	if err := w.Upload(context.Background(), "/data/new/a.txt", strings.NewReader("hello")); err != nil {
		t.Errorf("upload: %v", err)
	}
	rc, err := w.Download(context.Background(), "/data/movie.mkv")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "file-content" {
		t.Errorf("download = %q", b)
	}
}
