// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/trakt"
)

// fakeTraktStore 是 SystemConfigOper 的内存替身。
type fakeTraktStore struct{ m map[string]string }

func (f *fakeTraktStore) Get(key string) (string, error) { return f.m[key], nil }
func (f *fakeTraktStore) Set(key, value string) error {
	if f.m == nil {
		f.m = map[string]string{}
	}
	f.m[key] = value
	return nil
}

func TestTraktSettingsRoundTrip(t *testing.T) {
	store := &fakeTraktStore{}
	c := &TraktChain{store: store, client: trakt.NewClient("")}

	in := defaultTraktSettings()
	in.ClientID = "cid"
	in.ClientSecret = "sec"
	in.Enabled = true
	in.SyncMovies = true
	in.SyncShows = false
	if err := c.SaveSettings(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out := c.GetSettings()
	if out.ClientID != "cid" || out.ClientSecret != "sec" || !out.Enabled || out.SyncShows != false || out.SyncMovies != true {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	// 默认值在空 store 上仍生效
	store2 := &fakeTraktStore{}
	c2 := &TraktChain{store: store2, client: trakt.NewClient("")}
	d := c2.GetSettings()
	if !d.SyncMovies || !d.SyncShows || d.RedirectURI == "" {
		t.Fatalf("defaults lost: %+v", d)
	}
}

func TestTraktSettingsMappingDefaults(t *testing.T) {
	d := defaultTraktSettings()
	if d.SyncMovies != true || d.SyncShows != true || d.RedirectURI != "urn:ietf:wg:oauth:2.0:oob" {
		t.Fatalf("bad defaults: %+v", d)
	}
}

// expiredTraktChain 构造：已过期令牌 + 指向测试服务器的 client。
func expiredTraktChain(t *testing.T, srvURL string, store *fakeTraktStore) *TraktChain {
	t.Helper()
	c := &TraktChain{store: store, client: trakt.NewClient(srvURL)}
	s := defaultTraktSettings()
	s.Enabled = true
	s.AccessToken = "AT"
	s.RefreshToken = "RT"
	s.ExpiresAt = time.Now().Add(-time.Hour).Unix() // 已过期，触发刷新
	s.ClientID, s.ClientSecret = "cid", "cs"
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestRefreshTransientFailureKeepsTokens 瞬时刷新失败（5xx）必须保留令牌稍后重试——
// 此前任何失败都清空 access/refresh token → 一次网络抖动永久停摆须重新授权。
func TestRefreshTransientFailureKeepsTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	store := &fakeTraktStore{}
	c := expiredTraktChain(t, srv.URL, store)

	got := c.GetSettings()
	if err := c.refreshTokenIfNeeded(context.Background(), &got, false); err == nil {
		t.Fatal("refresh should fail on 500")
	}
	got2 := c.GetSettings()
	if got2.AccessToken != "AT" || got2.RefreshToken != "RT" {
		t.Fatalf("transient failure must keep tokens, got %+v", got2)
	}
	if got2.LastError == "" {
		t.Fatal("transient failure should record LastError")
	}
}

// TestRefreshAuthDeadClearsTokens 服务端明确拒绝（400 invalid_grant）→ 清令牌要求重新授权。
func TestRefreshAuthDeadClearsTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	store := &fakeTraktStore{}
	c := expiredTraktChain(t, srv.URL, store)

	got := c.GetSettings()
	if err := c.refreshTokenIfNeeded(context.Background(), &got, false); err == nil {
		t.Fatal("refresh should fail on 400")
	}
	got2 := c.GetSettings()
	if got2.AccessToken != "" || got2.RefreshToken != "" {
		t.Fatalf("auth-dead must clear tokens, got %+v", got2)
	}
}

// TestSyncAllTransientRefreshKeepsSyncAlive SyncAll 瞬时刷新失败后令牌保留，
// 故障恢复后下次同步自动成功（不再要求用户重新授权）。
func TestSyncAllTransientRefreshKeepsSyncAlive(t *testing.T) {
	failing := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth/token":
			if failing {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "AT2", "refresh_token": "RT2", "token_type": "Bearer", "expires_in": 7200,
			})
		case strings.HasPrefix(r.URL.Path, "/users/me/watchlist/"):
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	store := &fakeTraktStore{}
	c := expiredTraktChain(t, srv.URL, store)

	if err := c.SyncAll(context.Background()); err == nil {
		t.Fatal("sync should fail while refresh endpoint is down")
	}
	if after := c.GetSettings(); after.AccessToken != "AT" || after.RefreshToken != "RT" {
		t.Fatalf("tokens lost on transient failure: %+v", after)
	}
	failing = false
	if err := c.SyncAll(context.Background()); err != nil {
		t.Fatalf("sync should recover after outage: %v", err)
	}
	after := c.GetSettings()
	if after.AccessToken != "AT2" || after.LastSyncAt == 0 {
		t.Fatalf("sync did not recover/persist: %+v", after)
	}
}
