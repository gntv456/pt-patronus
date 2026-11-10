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

func TestZhixingFetchUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<strong class="vwmy"><a href="/user/bob">bob</a></strong>`))
		case "/user/bob":
			_, _ = w.Write([]byte(`<div>
<p>Uploaded: 4.00 TB</p>
<p>Downloaded: 1.00 TB</p>
<p>Ratio: 4.0</p>
<p>User Group: Power</p>
<p>Total Bonus: 9876</p>
<p>Seeding Bonus: 4321</p>
<p>Join: 2021-02-03 04:05:06</p>
<p>Last IPv6 Access: 2024-06-07 08:09:10</p>
</div>`))
		case "/user/bob/seeding/p1":
			_, _ = w.Write([]byte(zhixingTorrentTable()))
		case "/user/bob/seeding/p2":
			_, _ = w.Write([]byte(`<table class="torrenttable"><tbody></tbody></table>`))
		case "/user/bob/uploaded/p1":
			_, _ = w.Write([]byte(zhixingTorrentTable()))
		case "/user/bob/uploaded/p2":
			_, _ = w.Write([]byte(`<table class="torrenttable"><tbody></tbody></table>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := fetchZhixingUserInfo(context.Background(), hc, zhixingTestDef(), srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "bob" || u.Name != "bob" || u.Level != "Power" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if u.Uploaded != 4*(1<<40) || u.Downloaded != 1*(1<<40) || u.Ratio != 4 {
		t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", u.Uploaded, u.Downloaded, u.Ratio)
	}
	if u.Seeding != 2 || u.SeedingSize != 3*(1<<30) {
		t.Fatalf("seeding = %d size %d", u.Seeding, u.SeedingSize)
	}
	if u.Uploads != 2 {
		t.Fatalf("uploads = %d, want 2", u.Uploads)
	}
	if u.JoinDate != "2021-02-03" {
		t.Fatalf("join_date = %q", u.JoinDate)
	}
	if u.SeedingBonus != 4321 || u.LastAccessAt != "2024-06-07 08:09:10" {
		t.Fatalf("extras = seeding_bonus %g last_access_at %q", u.SeedingBonus, u.LastAccessAt)
	}
}

func zhixingTestDef() *site.SiteMetadata {
	return &site.SiteMetadata{
		ID:     "zhixing",
		Schema: "cgbtsource",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "table.torrenttable > tbody > tr"},
			Fields: map[string]site.ElementQuery{
				"title": {Selector: "a[name='title']"},
				"size":  {Selector: "td:nth-child(2)", Filters: []site.Filter{{Name: "parse_size"}}},
			},
		},
	}
}

func zhixingTorrentTable() string {
	return `<table class="torrenttable"><tbody>
<tr><td><a name="title" href="/torrents/1">one</a></td><td>1.00 GB</td></tr>
<tr><td><a name="title" href="/torrents/2">two</a></td><td>2.00 GB</td></tr>
</tbody></table>`
}
