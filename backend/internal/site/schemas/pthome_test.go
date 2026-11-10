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

func TestNexusPHPFetchPTHomeUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="info_block">
  <a href="userdetails.php?id=42">alice</a>
  <span>上传量: 1.50 TiB 下载量: 512 GiB 分享率: 3.0 魔力值: 12,345.6 当前做种: 7</span>
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "42" {
				t.Fatalf("detail id = %q, want 42", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2024-05-06 07:08:09</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量: 1.50 TiB 下载量: 512 GiB 分享率: 3.0</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 77.5 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "42" {
				t.Fatalf("ajax userid = %q, want 42", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>7 | 3.00 GiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>4</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("pthome")
	if !ok {
		t.Fatal("pthome definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	u, err := n.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "alice" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if want := int64(1536 * (1 << 30)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if u.Ratio != 3 || u.Bonus != 12345.6 || u.Seeding != 7 || u.Uploads != 4 || u.BonusPerHour != 77.5 {
		t.Fatalf("stats = ratio %g bonus %g seeding %d uploads %d bph %g", u.Ratio, u.Bonus, u.Seeding, u.Uploads, u.BonusPerHour)
	}
	if want := int64(3 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
