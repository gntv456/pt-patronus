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

func TestStarSpaceFetchUserInfo(t *testing.T) {
	srv, hc := starSpaceMockServer(t)
	defer srv.Close()

	u, err := fetchStarSpaceUserInfo(context.Background(), hc, &site.SiteMetadata{ID: "starspace"}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	assertStarSpaceUserInfo(t, u)
}

func TestGenericHTMLRoutesStarSpaceDefinitionToDedicatedUserInfo(t *testing.T) {
	srv, hc := starSpaceMockServer(t)
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("starspace")
	if !ok {
		t.Fatal("starspace definition missing")
	}
	g := NewGenericHTML(hc, "")
	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	assertStarSpaceUserInfo(t, u)
}

func starSpaceMockServer(t *testing.T) (*httptest.Server, *httpclient.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/p_index/index.php":
			_, _ = w.Write([]byte(`<span id="user_info"><a href="/p_user/user_detail.php?uid=8">star</a></span>`))
		case "/p_user/user_detail.php":
			if r.URL.Query().Get("uid") != "8" {
				t.Fatalf("detail uid = %q, want 8", r.URL.Query().Get("uid"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="nowrap">User Level</td><td><img alt="class2"></td></tr>
<tr><td class="nowrap">Join Date</td><td>2022-03-04 10:11:12</td></tr>
<tr><td class="nowrap">Last Access</td><td>2026-07-07 06:00</td></tr>
<tr><td class="nowrap">Bonus</td><td>4567.8</td></tr>
<tr><td class="nowrap">Seeding Bonus</td><td>888.5</td></tr>
<tr><td class="nowrap">Transfer</td><td><table><tr id="tr_item_min">
<td>9.00 TB</td><td>-</td><td>3.00 TB</td><td>3.0</td><td>6.00 TB</td><td>2.00 TB</td><td>3.0</td>
</tr></table></td></tr>
</table>`))
		case "/p_torrent/torrent_user.php":
			if r.URL.Query().Get("uid") != "8" {
				t.Fatalf("torrent uid = %q, want 8", r.URL.Query().Get("uid"))
			}
			switch r.URL.Query().Get("pop") {
			case "8":
				_, _ = w.Write([]byte(`<table>
<tr class="tm_tr"><td>one</td><td id="tm_td_size">1.00 GB</td></tr>
<tr class="tm_tr"><td>two</td><td id="tm_td_size">2.00 GB</td></tr>
</table>`))
			case "7":
				_, _ = w.Write([]byte(`<div id="table_pager"><span class="pager_current">5</span></div>`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		case "/p_bonus/bonus_hour.php":
			_, _ = w.Write([]byte(`<table><tr id="tr_list"><td>total</td><td>12.5</td></tr></table>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	return srv, hc
}

func assertStarSpaceUserInfo(t *testing.T, u *site.UserInfo) {
	t.Helper()
	if u.ID != "8" || u.Name != "star" || u.Level != "Power User" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if u.Uploaded != 9*(1<<40) || u.Downloaded != 3*(1<<40) || u.Ratio != 3 {
		t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", u.Uploaded, u.Downloaded, u.Ratio)
	}
	if u.TrueUploaded != 6*(1<<40) || u.TrueDownloaded != 2*(1<<40) || u.TrueRatio != 3 {
		t.Fatalf("true traffic = uploaded %d downloaded %d ratio %g", u.TrueUploaded, u.TrueDownloaded, u.TrueRatio)
	}
	if u.Seeding != 2 || u.SeedingSize != 3*(1<<30) {
		t.Fatalf("seeding = %d size %d", u.Seeding, u.SeedingSize)
	}
	if u.SeedingBonus != 888.5 {
		t.Fatalf("seeding_bonus = %g", u.SeedingBonus)
	}
	if u.Uploads != 5 || u.BonusPerHour != 12.5 {
		t.Fatalf("uploads/bph = %d/%g", u.Uploads, u.BonusPerHour)
	}
	if u.ClaimCount != 0 || u.MaxClaim != 0 {
		t.Fatalf("claim defaults = %d/%d, want 0/0", u.ClaimCount, u.MaxClaim)
	}
	if u.JoinDate != "2022-03-04" {
		t.Fatalf("join_date = %q", u.JoinDate)
	}
	if u.LastAccessAt != "2026-07-07 06:00" {
		t.Fatalf("last_access_at = %q", u.LastAccessAt)
	}
}
