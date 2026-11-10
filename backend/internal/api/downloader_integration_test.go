// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

type qbStub struct {
	mu       sync.Mutex
	forms    map[string]url.Values
	requests map[string]int
}

func newQBStub(t *testing.T) (*qbStub, *httptest.Server) {
	t.Helper()
	stub := &qbStub{forms: map[string]url.Values{}, requests: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.requests[r.URL.Path]++
		stub.mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.1.0"))
		case "/api/v2/sync/maindata":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{"abc":{"name":"Movie","size":1024,"progress":0.5,"ratio":1.25,"state":"downloading","dlspeed":128,"upspeed":64,"save_path":"/downloads","category":"movie","tags":"hd"}},"server_state":{"dl_info_speed":128,"up_info_speed":64}}`))
		case "/api/v2/torrents/add", "/api/v2/torrents/stop", "/api/v2/torrents/start", "/api/v2/torrents/delete":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm %s: %v", r.URL.Path, err)
			}
			stub.mu.Lock()
			stub.forms[r.URL.Path] = r.Form
			stub.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return stub, server
}

func (s *qbStub) form(path string) url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forms[path]
}

func TestDownloaderAPIJourneyAgainstQBStub(t *testing.T) {
	stub, qbServer := newQBStub(t)
	deps := newTestDeps(t)
	server := httptest.NewServer(Build(deps))
	defer server.Close()

	token := loginAsAdmin(t, server)

	code, body := do(t, server, http.MethodPost, "/api/v1/downloaders", map[string]any{
		"name": "qb-stub", "type": "qb", "url": qbServer.URL, "username": "u", "password": "p", "is_default": true,
	}, token)
	if code != http.StatusOK {
		t.Fatalf("create downloader=%d %v", code, body)
	}
	id := int(body["id"].(float64))
	base := fmt.Sprintf("/api/v1/downloaders/%d", id)

	if code, body = do(t, server, http.MethodPost, base+"/test", nil, token); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("test connection=%d %v", code, body)
	}
	if code, body = do(t, server, http.MethodGet, base+"/torrents", nil, token); code != http.StatusOK {
		t.Fatalf("torrents=%d %v", code, body)
	}
	torrents, ok := body["torrents"].([]any)
	if !ok || len(torrents) != 1 {
		t.Fatalf("torrents=%v", body["torrents"])
	}
	if code, body = do(t, server, http.MethodGet, base+"/stats", nil, token); code != http.StatusOK || body["dl_rate"] != float64(128) || body["up_rate"] != float64(64) {
		t.Fatalf("stats=%d %v", code, body)
	}

	if code, body = do(t, server, http.MethodPost, base+"/torrents", map[string]any{
		"url": "magnet:?xt=urn:btih:abc", "category": "movie", "tags": []string{"hd"}, "save_path": "/downloads", "paused": true,
	}, token); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("add=%d %v", code, body)
	}
	addForm := stub.form("/api/v2/torrents/add")
	if addForm.Get("urls") != "magnet:?xt=urn:btih:abc" || addForm.Get("category") != "movie" || addForm.Get("stopped") != "true" {
		t.Fatalf("add form=%v", addForm)
	}

	for _, tc := range []struct {
		path     string
		stubPath string
		body     map[string]any
	}{
		{"/pause", "/api/v2/torrents/stop", map[string]any{"hashes": []string{"abc"}}},
		{"/resume", "/api/v2/torrents/start", map[string]any{"hashes": []string{"abc"}}},
		{"/remove", "/api/v2/torrents/delete", map[string]any{"hashes": []string{"abc"}, "delete_files": true}},
	} {
		if code, body = do(t, server, http.MethodPost, base+tc.path, tc.body, token); code != http.StatusOK || body["ok"] != true {
			t.Fatalf("%s=%d %v", tc.path, code, body)
		}
		if stub.form(tc.stubPath).Get("hashes") != "abc" {
			t.Fatalf("%s form=%v", tc.stubPath, stub.form(tc.stubPath))
		}
	}
	if stub.form("/api/v2/torrents/delete").Get("deleteFiles") != "true" {
		t.Fatalf("delete form=%v", stub.form("/api/v2/torrents/delete"))
	}

	if code, _ = do(t, server, http.MethodPost, base+"/torrents/tags", map[string]any{"hashes": []string{"abc"}, "mode": "typo"}, token); code != http.StatusBadRequest {
		t.Fatalf("invalid tag mode=%d, want 400", code)
	}
}

func TestDownloaderAPIConcurrentReadsAreThrottledSafely(t *testing.T) {
	_, qbServer := newQBStub(t)
	deps := newTestDeps(t)
	server := httptest.NewServer(Build(deps))
	defer server.Close()

	token := loginAsAdmin(t, server)
	_, body := do(t, server, http.MethodPost, "/api/v1/downloaders", map[string]any{"name": "qb", "type": "qb", "url": qbServer.URL}, token)
	id := int(body["id"].(float64))

	var wg sync.WaitGroup
	statuses := make(chan int, 24)
	for i := 0; i < 12; i++ {
		for _, suffix := range []string{"/torrents", "/stats"} {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				code, _ := do(t, server, http.MethodGet, fmt.Sprintf("/api/v1/downloaders/%d%s", id, path), nil, token)
				statuses <- code
			}(suffix)
		}
	}
	wg.Wait()
	close(statuses)
	var okCount, throttledCount int
	for status := range statuses {
		switch status {
		case http.StatusOK:
			okCount++
		case http.StatusTooManyRequests:
			throttledCount++
		default:
			t.Errorf("unexpected concurrent status %d", status)
		}
	}
	if okCount == 0 || throttledCount == 0 {
		t.Fatalf("ok=%d throttled=%d, want both successful and throttled requests", okCount, throttledCount)
	}
}
