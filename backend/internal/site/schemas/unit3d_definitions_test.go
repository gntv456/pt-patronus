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

func TestUnit3DDefinitionsSearchAndUserInfo(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)

	for _, id := range []string{
		"animelovers",
		"blutopia",
		"capybarabr",
		"cinematik",
		"dxdhd",
		"hd4free",
		"hdpost",
		"jptv4us",
		"jptvclub",
		"kimoji",
		"lst",
		"nordicquality",
		"oldtoonsworld",
		"reelflix",
		"shareisland",
		"telly",
		"torrentseeds",
	} {
		t.Run(id, func(t *testing.T) {
			def, ok := reg.Definition(id)
			if !ok {
				t.Fatalf("%s definition missing", id)
			}
			srv := unit3DDefinitionMockServer(t, id)
			defer srv.Close()

			ts, err := u.Search(context.Background(), def, srv.URL, "cookie", "movie", "", 0)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(ts) != 1 {
				t.Fatalf("Search returned %d torrents, want 1", len(ts))
			}
			if ts[0].Title == "" || ts[0].DetailURL == "" {
				t.Fatalf("torrent title/detail = %q/%q", ts[0].Title, ts[0].DetailURL)
			}

			ui, err := u.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
			if err != nil {
				t.Fatalf("FetchUserInfo: %v", err)
			}
			if ui.Name != "alice" || ui.Level != "Archivist" || ui.JoinDate != "2024-01-02" {
				t.Fatalf("identity = name %q level %q join %q", ui.Name, ui.Level, ui.JoinDate)
			}
			if ui.Uploaded != 2*(1<<40) || ui.Downloaded != 512*(1<<30) || ui.Ratio != 4 {
				t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", ui.Uploaded, ui.Downloaded, ui.Ratio)
			}
			if ui.Seeding != 7 || ui.Leeching != 2 {
				t.Fatalf("active = seeding %d leeching %d", ui.Seeding, ui.Leeching)
			}
			if ui.SeedingSize != 3*(1<<40) {
				t.Fatalf("seeding_size = %d, want %d", ui.SeedingSize, int64(3*(1<<40)))
			}
			if id == "lst" {
				if ui.ID != "99" || ui.Uploads != 42 || ui.BonusPerHour != 88.5 {
					t.Fatalf("lst extras = id %q uploads %d bph %g", ui.ID, ui.Uploads, ui.BonusPerHour)
				}
			} else if ui.BonusPerHour != 12.5 {
				t.Fatalf("bonus_per_hour = %g, want 12.5", ui.BonusPerHour)
			}
		})
	}
}

func unit3DDefinitionMockServer(t *testing.T, id string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a href="/users/alice/settings">Alice</a></body></html>`))
		case "/torrents", "/torrents/":
			if id == "lst" {
				_, _ = w.Write([]byte(`<html><body>
<div class="torrent-results__list">
  <div class="torrent-search-row">
    <span class="torrent-search-row__category" title="Movie"></span>
    <div class="torrent-search-row__name"><a href="/torrents/123">LST Movie</a></div>
    <span class="torrent-search-row__stat--size" title="2147483648">2.00 GiB</span>
    <span class="torrent-search-row__stat--seeders">8</span>
    <span class="torrent-search-row__stat--leechers">1</span>
    <span class="torrent-search-row__stat--completed">22</span>
  </div>
</div>
</body></html>`))
				return
			}
			_, _ = w.Write([]byte(`<html><body>
<div class="torrent-search--list__results"><table><tbody>
  <tr>
    <td><a class="view-torrent" href="/torrents/123">Unit3D Movie</a></td>
    <td class="torrent-search--list__size">2.00 GiB</td>
    <td class="torrent-search--list__seeders">8</td>
    <td class="torrent-search--list__leechers">1</td>
    <td class="torrent-search--list__completed">22</td>
    <td><a href="/torrents/download_check/123">download</a></td>
  </tr>
</tbody></table></div>
</body></html>`))
		case "/users/alice":
			switch id {
			case "capybarabr":
				_, _ = w.Write([]byte(unit3DProfileHTML(`<dl><dt>Tamanho em seeding</dt><dd>3.00 TiB</dd></dl>`)))
			case "lst":
				_, _ = w.Write([]byte(unit3DProfileHTML(`
<span class="profile-hero__account-item"><i class="fa-hashtag"></i><span class="profile-hero__account-value">99</span></span>
<span class="profile-hero__meta-item">Registration date: 2024-01-02 03:04:05</span>
<div class="profile-mini-stat__label">Seeding Size</div><div>3.00 TiB</div>
<div class="profile-mini-stat__label">Total uploads</div><div>42</div>
<span class="sidebar-stat__label">Points per hour</span><span>88.5</span>`)))
			default:
				_, _ = w.Write([]byte(unit3DProfileHTML(`<dl><dt>Seeding Size</dt><dd>3.00 TiB</dd></dl>`)))
			}
		case "/users/alice/earnings":
			_, _ = w.Write([]byte(`<html><body><div class="panelV2"><dl class="key-value">
  <dd>seedbonus</dd><dd>total</dd><dd>12.5</dd>
</dl></div></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func unit3DProfileHTML(extra string) string {
	return `<html><body>
<ul class="ratio-bar">
  <li class="ratio-bar__uploaded"><a><i class="fa-arrow-up"></i> 2.00 TiB</a></li>
  <li class="ratio-bar__downloaded"><a><i class="fa-arrow-down"></i> 512.00 GiB</a></li>
  <li class="ratio-bar__ratio"><a><i class="fa-sync-alt"></i> 4.0</a></li>
  <li class="ratio-bar__points"><a><i class="fa-coins"></i> 12345.6</a></li>
  <li class="ratio-bar__seeding"><a><i class="fa-upload"></i> 7</a></li>
  <li class="ratio-bar__leeching"><a><i class="fa-download"></i> 2</a></li>
</ul>
<div class="content"><span class="badge-user" title="Archivist"></span></div>
<time class="profile__registration" datetime="2024-01-02 03:04:05">registered</time>
` + extra + `
</body></html>`
}
