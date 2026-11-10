// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

type transmissionStub struct {
	mu         sync.Mutex
	sessionID  string
	handshakes int
	methods    []string
	arguments  map[string][]map[string]any
	offline    bool
}

func newTransmissionStub(t *testing.T) (*transmissionStub, *httptest.Server) {
	t.Helper()
	stub := &transmissionStub{sessionID: "SID-1", arguments: map[string][]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		stub.mu.Lock()
		expectedSID := stub.sessionID
		offline := stub.offline
		stub.mu.Unlock()
		if offline {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("X-Transmission-Session-Id") != expectedSID {
			stub.mu.Lock()
			stub.handshakes++
			stub.mu.Unlock()
			w.Header().Set("X-Transmission-Session-Id", expectedSID)
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method    string         `json:"method"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode RPC: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		stub.mu.Lock()
		stub.methods = append(stub.methods, req.Method)
		stub.arguments[req.Method] = append(stub.arguments[req.Method], req.Arguments)
		stub.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "session-get":
			_, _ = w.Write([]byte(`{"result":"success","arguments":{"downloadSpeed":321,"uploadSpeed":123}}`))
		case "torrent-get":
			_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrents":[{"id":42,"name":"Series","hashString":"def","totalSize":2048,"downloadedEver":1024,"uploadedEver":4096,"percentDone":0.5,"uploadRatio":2,"status":4,"rateDownload":321,"rateUpload":123,"downloadDir":"/media","labels":["tv"],"addedDate":1700000000}]}}`))
		case "torrent-add":
			_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrent-added":{"id":42}}}`))
		default:
			_, _ = w.Write([]byte(`{"result":"success","arguments":{}}`))
		}
	}))
	t.Cleanup(server.Close)
	return stub, server
}

func (s *transmissionStub) rotateSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionID = sessionID
}

func (s *transmissionStub) setOffline(offline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offline = offline
}

func (s *transmissionStub) lastArgs(method string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.arguments[method]
	if len(items) == 0 {
		return nil
	}
	return items[len(items)-1]
}

func (s *transmissionStub) handshakeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handshakes
}

func TestDownloaderAPIJourneyAgainstTransmissionStub(t *testing.T) {
	stub, transmissionServer := newTransmissionStub(t)
	deps := newTestDeps(t)
	server := httptest.NewServer(Build(deps))
	defer server.Close()

	token := loginAsAdmin(t, server)
	code, body := do(t, server, http.MethodPost, "/api/v1/downloaders", map[string]any{
		"name": "tr-stub", "type": "tr", "url": transmissionServer.URL, "username": "user", "password": "pass",
	}, token)
	if code != http.StatusOK {
		t.Fatalf("create=%d %v", code, body)
	}
	id := int(body["id"].(float64))
	base := "/api/v1/downloaders/" + bodyNumber(id)

	if code, body = do(t, server, http.MethodPost, base+"/test", nil, token); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("test=%d %v", code, body)
	}
	if stub.handshakeCount() != 1 {
		t.Fatalf("initial handshakes=%d, want 1", stub.handshakeCount())
	}
	if code, body = do(t, server, http.MethodGet, base+"/torrents", nil, token); code != http.StatusOK {
		t.Fatalf("torrents=%d %v", code, body)
	}
	torrents, ok := body["torrents"].([]any)
	if !ok || len(torrents) != 1 {
		t.Fatalf("torrents=%v", body["torrents"])
	}
	if code, body = do(t, server, http.MethodGet, base+"/stats", nil, token); code != http.StatusOK || body["dl_rate"] != float64(321) || body["up_rate"] != float64(123) {
		t.Fatalf("stats=%d %v", code, body)
	}

	if code, body = do(t, server, http.MethodPost, base+"/torrents", map[string]any{
		"url": "magnet:?xt=urn:btih:def", "tags": []string{"tv"}, "save_path": "/media", "paused": true,
		"upload_limit": 1025, "download_limit": 2049,
	}, token); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("add=%d %v", code, body)
	}
	addArgs := stub.lastArgs("torrent-add")
	if addArgs["filename"] != "magnet:?xt=urn:btih:def" || addArgs["download-dir"] != "/media" || addArgs["paused"] != true {
		t.Fatalf("torrent-add args=%v", addArgs)
	}
	limitArgs := stub.lastArgs("torrent-set")
	if limitArgs["uploadLimit"] != float64(2) || limitArgs["downloadLimit"] != float64(3) {
		t.Fatalf("torrent-set limits=%v", limitArgs)
	}

	if code, body = do(t, server, http.MethodPost, base+"/pause", map[string]any{"hashes": []string{"def"}}, token); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("pause=%d %v", code, body)
	}
	stub.rotateSession("SID-2")
	if code, body = do(t, server, http.MethodPost, base+"/resume", map[string]any{"hashes": []string{"def"}}, token); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("resume after session rotation=%d %v", code, body)
	}
	if stub.handshakeCount() != 2 {
		t.Fatalf("rotated handshakes=%d, want 2", stub.handshakeCount())
	}
	if code, body = do(t, server, http.MethodPost, base+"/remove", map[string]any{"hashes": []string{"def"}, "delete_files": true}, token); code != http.StatusOK || body["ok"] != true {
		t.Fatalf("remove=%d %v", code, body)
	}
	if stub.lastArgs("torrent-remove")["delete-local-data"] != true {
		t.Fatalf("torrent-remove args=%v", stub.lastArgs("torrent-remove"))
	}
}

func TestTransmissionAuthenticationFailureIsReported(t *testing.T) {
	_, transmissionServer := newTransmissionStub(t)
	deps := newTestDeps(t)
	server := httptest.NewServer(Build(deps))
	defer server.Close()

	token := loginAsAdmin(t, server)
	_, body := do(t, server, http.MethodPost, "/api/v1/downloaders", map[string]any{
		"name": "tr-bad-auth", "type": "tr", "url": transmissionServer.URL, "username": "user", "password": "wrong",
	}, token)
	id := int(body["id"].(float64))
	code, body := do(t, server, http.MethodPost, "/api/v1/downloaders/"+bodyNumber(id)+"/test", nil, token)
	if code != http.StatusOK || body["ok"] != false || body["error"] != "invalid credentials" {
		t.Fatalf("auth failure=%d %v", code, body)
	}
}

func TestTransmissionRecoversAfterTemporaryOutage(t *testing.T) {
	stub, transmissionServer := newTransmissionStub(t)
	stub.setOffline(true)
	deps := newTestDeps(t)
	server := httptest.NewServer(Build(deps))
	defer server.Close()

	token := loginAsAdmin(t, server)
	_, body := do(t, server, http.MethodPost, "/api/v1/downloaders", map[string]any{
		"name": "tr-recovery", "type": "tr", "url": transmissionServer.URL, "username": "user", "password": "pass",
	}, token)
	id := int(body["id"].(float64))
	path := "/api/v1/downloaders/" + bodyNumber(id) + "/test"

	code, body := do(t, server, http.MethodPost, path, nil, token)
	if code != http.StatusOK || body["ok"] != false || body["error"] != "rpc status 503" {
		t.Fatalf("offline result=%d %v", code, body)
	}
	stub.setOffline(false)
	code, body = do(t, server, http.MethodPost, path, nil, token)
	if code != http.StatusOK || body["ok"] != true {
		t.Fatalf("recovery result=%d %v", code, body)
	}
}

func bodyNumber(value int) string {
	return strconv.Itoa(value)
}
