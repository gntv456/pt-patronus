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

func TestPussyTorrentsFetchUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<div id="memberBar"><div class="span8"><a href="/profile/Alice">Alice</a></div></div>
<div id="profile">
  <button><i class="icon-arrow-up"></i>1.25 TiB</button>
  <button><i class="icon-arrow-down"></i>512 GiB</button>
</div>`))
		case "/profile/Alice":
			_, _ = w.Write([]byte(`<table id="profileTable">
  <tr><td>Class</td><td>Power User</td></tr>
  <tr><td>Join Date</td><td>2024-02-03</td></tr>
</table>
<a href="#snatchlist" data-ajax="/user/account/snatchlist" data-userid="42">Snatchlist</a>`))
		case "/user/account/snatchlist":
			if r.Method != http.MethodPost {
				t.Fatalf("snatchlist method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse snatchlist form: %v", err)
			}
			if r.Form.Get("userID") != "42" {
				t.Fatalf("snatchlist userID = %q, want 42", r.Form.Get("userID"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"aaData":[
				["A","1.50 GiB","","","","","",1,"yes"],
				["B","3.00 GiB","","","","","",1,"no"],
				["C","4.00 GiB","","","","","",0,"yes"],
				["D","512 MiB","","","","","",1,"YES"]
			]}`))
		case "/user/account/uploadedtorrents":
			if r.Method != http.MethodPost {
				t.Fatalf("uploads method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse uploads form: %v", err)
			}
			if r.Form.Get("userID") != "42" {
				t.Fatalf("uploads userID = %q, want 42", r.Form.Get("userID"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"iTotalRecords":7}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("pussytorrents")
	if !ok {
		t.Fatal("pussytorrents definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")
	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.Name != "Alice" || u.ID != "Alice" || u.Level != "Power User" || u.JoinDate != "2024-02-03" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.NumericUserID != 42 {
		t.Fatalf("numeric_user_id = %d, want 42", u.NumericUserID)
	}
	if want := int64(1280 * (1 << 30)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if u.Seeding != 2 {
		t.Fatalf("seeding = %d, want 2", u.Seeding)
	}
	if want := int64(2 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
	if u.Uploads != 7 {
		t.Fatalf("uploads = %d, want 7", u.Uploads)
	}
}
