// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestTorrentLeechFetchUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<span class="centerTopBar">
<span onclick="window.location='/profile/Alice/view'">Alice</span>
<div title="Uploaded"><span>10.00 TiB</span></div>
<div title="Downloaded"><span>2.00 TiB</span></div>
<span class="total-TL-points">12345.6</span>
</span>`))
		case "/profile/Alice":
			_, _ = w.Write([]byte(`<script>var userLogUserID = '77';</script>
<div class="profile-details"><div class="label-user-class">Power User</div></div>
<table class="profileViewTable">
<tr><td>Registration date</td><td>Saturday 6th May 2017</td></tr>
<tr><td>Last visit</td><td>Friday 7th November 2025 12:04:58 PM (38 seconds ago)</td></tr>
</table>`))
		case "/profile/Alice/seeding":
			_, _ = w.Write([]byte(`<table id="profile-seedingTable">
<tr><th>Name</th><th>Size</th></tr>
<tr><td>one</td><td>1.50 GiB</td></tr>
<tr><td>two</td><td>2.50 GiB</td></tr>
</table>`))
		case "/user/account/uploadedtorrents":
			if r.Method != http.MethodPost {
				t.Fatalf("uploads method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.Form.Get("userID") != "77" {
				t.Fatalf("userID = %q, want 77", r.Form.Get("userID"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"iTotalRecords":12}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := fetchTorrentLeechUserInfo(context.Background(), hc, &site.SiteMetadata{ID: "torrentleech"}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.Name != "Alice" || u.ID != "77" || u.Level != "Power User" {
		t.Fatalf("identity = name %q id %q level %q", u.Name, u.ID, u.Level)
	}
	if u.Uploaded != 10*(1<<40) || u.Downloaded != 2*(1<<40) {
		t.Fatalf("traffic = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
	if u.Seeding != 2 || u.SeedingSize != 4*(1<<30) {
		t.Fatalf("seeding = %d size %d", u.Seeding, u.SeedingSize)
	}
	if u.Uploads != 12 {
		t.Fatalf("uploads = %d, want 12", u.Uploads)
	}
	if u.JoinDate != "2017-05-06" {
		t.Fatalf("join_date = %q, want 2017-05-06", u.JoinDate)
	}
	if u.LastAccessAt != "2025-11-07 12:04:58" {
		t.Fatalf("last_access_at = %q, want 2025-11-07 12:04:58", u.LastAccessAt)
	}
}
