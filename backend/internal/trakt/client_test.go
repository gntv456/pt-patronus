package trakt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientDeviceCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/device/code" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("trakt-api-version") != "2" {
			t.Fatalf("missing trakt-api-version header")
		}
		_ = json.NewEncoder(w).Encode(DeviceCode{DeviceCode: "DC", UserCode: "UC", VerificationURL: "https://trakt.tv/activate", ExpiresIn: 600, Interval: 5})
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	dc, err := c.DeviceCode(context.Background(), Credentials{ClientID: "cid"})
	if err != nil {
		t.Fatalf("DeviceCode: %v", err)
	}
	if dc.DeviceCode != "DC" || dc.UserCode != "UC" {
		t.Fatalf("bad device code: %+v", dc)
	}
}

func TestClientPollDeviceTokenPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	_, err := c.PollDeviceToken(context.Background(), "DC", Credentials{ClientID: "cid", ClientSecret: "cs"})
	var dte *DeviceTokenError
	if err == nil || !strings.Contains(err.Error(), "authorization_pending") {
		t.Fatalf("want DeviceTokenError, got %v", err)
	}
	if !asDeviceErr(err, &dte) {
		t.Fatalf("not a *DeviceTokenError: %T", err)
	}
}

func TestClientPollDeviceTokenSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "AT", "refresh_token": "RT", "token_type": "Bearer", "expires_in": 7200,
		})
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	tok, err := c.PollDeviceToken(context.Background(), "DC", Credentials{ClientID: "cid", ClientSecret: "cs"})
	if err != nil {
		t.Fatalf("PollDeviceToken: %v", err)
	}
	if tok.AccessToken != "AT" || tok.RefreshToken != "RT" || tok.CreatedAt == 0 {
		t.Fatalf("bad token: %+v", tok)
	}
}

func TestClientWatchlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me/watchlist/movies" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer AT" {
			t.Fatalf("missing bearer: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("trakt-api-key") != "cid" {
			t.Fatalf("missing trakt-api-key")
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"type": "movie", "movie": map[string]any{
				"title": "Inception", "year": 2010,
				"ids": map[string]any{"trakt": 1, "tmdb": 27205, "imdb": "tt1375666"},
			}},
			{"type": "movie", "movie": map[string]any{
				"title": "Interstellar", "year": 2014,
				"ids": map[string]any{"trakt": 2, "tmdb": 157336, "imdb": "tt0816692"},
			}},
		})
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	items, err := c.Watchlist(context.Background(), "AT", "cid", "movies")
	if err != nil {
		t.Fatalf("Watchlist: %v", err)
	}
	if len(items) != 2 || items[0].Title != "Inception" || items[0].TMDBID != 27205 || items[1].IMDbID != "tt0816692" {
		t.Fatalf("bad watchlist parse: %+v", items)
	}
}

// asDeviceErr 是 errors.As 的本地替身（测试足够）。
func asDeviceErr(err error, target **DeviceTokenError) bool {
	if dte, ok := err.(*DeviceTokenError); ok {
		*target = dte
		return true
	}
	return false
}

// TestRefreshTokenErrorClassification 刷新失败分类：400/401=AuthDead（令牌已死须重授权）；
// 429/5xx=瞬时（保留令牌重试）。此前一律为泛化错误，调用方无法区分而清空令牌。
func TestRefreshTokenErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		status int
		dead   bool
	}{
		{400, true}, {401, true}, {429, false}, {500, false}, {503, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		}))
		c := NewClient(srv.URL)
		_, err := c.RefreshToken(context.Background(), "RT", "", Credentials{ClientID: "cid", ClientSecret: "cs"})
		var re *RefreshError
		if !errors.As(err, &re) {
			t.Fatalf("status %d: want *RefreshError, got %T %v", tc.status, err, err)
		}
		if re.AuthDead != tc.dead {
			t.Fatalf("status %d: AuthDead=%v want %v", tc.status, re.AuthDead, tc.dead)
		}
		srv.Close()
	}
}
