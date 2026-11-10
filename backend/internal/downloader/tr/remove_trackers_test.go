// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package tr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRemoveTrackersMapsURLsToIDs(t *testing.T) {
	var methods []string
	var removeArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "SID")
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method    string         `json:"method"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(body, &req)
		methods = append(methods, req.Method)
		if req.Method == "torrent-get" {
			_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrents":[{"trackers":[{"id":7,"announce":"https://a/announce"},{"id":9,"announce":"https://b/announce"}]}]}}`))
			return
		}
		removeArgs = req.Arguments
		_, _ = w.Write([]byte(`{"result":"success","arguments":{}}`))
	}))
	defer srv.Close()

	client := New(srv.URL, "u", "p")
	if err := client.RemoveTrackers(context.Background(), "hash", []string{"https://b/announce"}); err != nil {
		t.Fatalf("RemoveTrackers: %v", err)
	}
	if !reflect.DeepEqual(methods, []string{"torrent-get", "torrent-set"}) {
		t.Fatalf("methods=%v", methods)
	}
	if got := removeArgs["trackerRemove"]; !reflect.DeepEqual(got, []any{float64(9)}) {
		t.Fatalf("trackerRemove=%v, want [9]", got)
	}
}

func TestRemoveTrackersUnknownURLIsNoop(t *testing.T) {
	var setCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "SID")
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Method == "torrent-set" {
			setCalls++
		}
		_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrents":[{"trackers":[]}]}}`))
	}))
	defer srv.Close()

	client := New(srv.URL, "u", "p")
	if err := client.RemoveTrackers(context.Background(), "hash", []string{"https://missing/announce"}); err != nil {
		t.Fatalf("RemoveTrackers: %v", err)
	}
	if setCalls != 0 {
		t.Fatalf("torrent-set calls=%d, want 0", setCalls)
	}
}
