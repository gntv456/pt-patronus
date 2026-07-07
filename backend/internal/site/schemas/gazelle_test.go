package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestGazelleSearchGroupFlatten：canned torrent_table 含表头 + group 头 + 2 group_torrent 子 + 1 独立。
// 验证：group 扁平化（子行继承 group 标题）、表头列探测（size/seeders/completed 按 nth-child）、download 已 action=download。
func TestGazelleSearchGroupFlatten(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><table class="torrent_table">
		<tr class="colhead">
		  <td>Cat</td><td>Name</td>
		  <td><a href="torrents.php?order_by=size">Size</a></td>
		  <td><a href="torrents.php?order_by=seeders">S</a></td>
		  <td><a href="torrents.php?order_by=leechers">L</a></td>
		  <td><a href="torrents.php?order_by=snatched">C</a></td>
		</tr>
		<tr class="group">
		  <td><a href="filter_cat[1]=1"><img alt="Music"></a></td>
		  <td><a href="torrents.php?id=500">Group Title</a><div class="tags">tag1</div></td>
		  <td></td><td></td><td></td><td></td>
		</tr>
		<tr class="group_torrent groupid_500">
		  <td></td>
		  <td><a href="torrents.php?action=download&amp;id=10">DL</a></td>
		  <td>1.0 GB</td><td>5</td><td>2</td><td>40</td>
		</tr>
		<tr class="group_torrent groupid_500">
		  <td></td>
		  <td><a href="torrents.php?action=download&amp;id=11">DL</a></td>
		  <td>2.0 GB</td><td>3</td><td>1</td><td>20</td>
		</tr>
		<tr class="torrent">
		  <td><a href="filter_cat[2]=1"><img alt="App"></a></td>
		  <td><a href="torrents.php?id=600">Standalone</a><a href="torrents.php?action=download&amp;id=20">DL</a></td>
		  <td>500 MB</td><td>1</td><td>0</td><td>5</td>
		</tr>
		</table></body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelle(hc)
	def := &site.SiteMetadata{ID: "gz", Schema: "gazelle"} // 选择器由 Search 内 EnsureDefaults 注入

	ts, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 3 {
		t.Fatalf("expected 3 torrents (2 group children + 1 standalone), got %d", len(ts))
	}
	// group 子行：继承 group 标题，download 已 action=download&id=
	if ts[0].Title != "Group Title" || ts[0].ID != "10" {
		t.Errorf("ts[0]: Title=%q ID=%q, want Group Title/10", ts[0].Title, ts[0].ID)
	}
	if ts[0].Seeders != 5 || ts[0].Completed != 40 || ts[0].Size != "1.0 GB" {
		t.Errorf("ts[0] columns: Seeders=%d Completed=%d Size=%q", ts[0].Seeders, ts[0].Completed, ts[0].Size)
	}
	if ts[1].Title != "Group Title" || ts[1].ID != "11" || ts[1].Seeders != 3 {
		t.Errorf("ts[1]: %+v", ts[1])
	}
	// 独立种子：自有标题/分类
	if ts[2].Title != "Standalone" || ts[2].ID != "20" || ts[2].Category != "App" {
		t.Errorf("ts[2]: Title=%q ID=%q Cat=%q", ts[2].Title, ts[2].ID, ts[2].Category)
	}
}

// TestGazelleUserInfoTwoStep：/ 发现 uid → /user.php?id=N 解析 ul.stats。
func TestGazelleUserInfoTwoStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a class="username" href="user.php?id=7">u</a></body></html>`))
		case "/user.php":
			_, _ = w.Write([]byte(`<html><body>
			<a class="username" href="user.php?id=7">alice</a>
			<div>Stats</div><ul class="stats"><li>Uploaded: 2.0 TB</li><li>Ratio: 3.5</li></ul>
			<div>Community</div><ul class="stats"><li>Uploaded: 12</li></ul>
			<div>Personal</div><ul class="stats"><li>Class: VIP</li></ul>
			</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelle(hc)
	def := &site.SiteMetadata{ID: "gz", Schema: "gazelle"}

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "7" {
		t.Errorf("ID = %q, want 7", ui.ID)
	}
	if ui.Name != "alice" {
		t.Errorf("Name = %q, want alice", ui.Name)
	}
	if ui.Level != "VIP" {
		t.Errorf("Level = %q, want VIP", ui.Level)
	}
	if ui.Ratio != 3.5 {
		t.Errorf("Ratio = %v, want 3.5", ui.Ratio)
	}
	if ui.Uploaded <= 0 {
		t.Errorf("Uploaded = %d, want >0", ui.Uploaded)
	}
	if ui.Uploads != 12 {
		t.Errorf("Uploads = %d, want 12", ui.Uploads)
	}
}

func TestGazelleStandardDefinitionsUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a class="username" href="user.php?id=7">alice</a></body></html>`))
		case "/user.php":
			if r.URL.Query().Get("id") != "7" {
				t.Fatalf("id = %q, want 7", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<html><body>
<a class="username" href="user.php?id=7">alice</a>
<div>Stats</div><ul class="stats">
  <li>Uploaded: 2.00 TiB</li>
  <li>Downloaded: 512.00 GiB</li>
  <li>Ratio: 4.0</li>
  <li>Joined: <span title="2024-01-02 03:04:05">joined</span></li>
  <li>Bonus Points: 12345.6</li>
</ul>
<div>Community</div><ul class="stats">
  <li>Seeding: 7</li>
  <li>Total seed size: <span title="3.00 TiB">3.00 TiB</span></li>
  <li>Uploaded: 42</li>
</ul>
<div>Personal</div><ul class="stats"><li>Class: Elite</li></ul>
</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelle(hc)
	for _, id := range []string{"awesomehd", "gfxpeers", "mooko", "sanpro"} {
		t.Run(id, func(t *testing.T) {
			def, ok := reg.Definition(id)
			if !ok {
				t.Fatalf("%s definition missing", id)
			}
			u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
			if err != nil {
				t.Fatalf("FetchUserInfo: %v", err)
			}
			if u.ID != "7" || u.Name != "alice" || u.Level != "Elite" || u.JoinDate != "2024-01-02" {
				t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
			}
			if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.Ratio != 4 {
				t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", u.Uploaded, u.Downloaded, u.Ratio)
			}
			if u.Seeding != 7 || u.SeedingSize != 3*(1<<40) || u.Uploads != 42 || u.Bonus != 12345.6 {
				t.Fatalf("stats = seeding %d size %d uploads %d bonus %g", u.Seeding, u.SeedingSize, u.Uploads, u.Bonus)
			}
		})
	}
}

func TestGazellePassThePopcornDefinitionUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/", "/index.php":
			_, _ = w.Write([]byte(`<html><body><a href="user.php?id=77">ptp-user</a></body></html>`))
		case "/user.php":
			if r.URL.Query().Get("id") != "77" {
				t.Fatalf("id = %q, want 77", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<html><body>
<a href="user.php?id=77">ptp-user</a>
<div class="panel__heading">Stats</div><div class="panel__body"><ul class="list">
  <li>Uploaded: 2.00 TiB</li>
  <li>Downloaded: 512.00 GiB</li>
  <li>Ratio: 4.0</li>
  <li>Joined: <span title="2024-01-02 03:04:05">joined</span></li>
  <li>Points: 12,345.6</li>
  <li>Points per hour: 12.5</li>
</ul></div>
<div class="panel__heading">Community</div><div class="panel__body"><ul class="list">
  <li>Seeding: 7</li>
  <li>Seeding size: 3.00 TiB</li>
  <li>Uploaded: 42</li>
</ul></div>
<div class="panel__heading">Personal</div><div class="panel__body"><ul class="list">
  <li>Class: Elite</li>
</ul></div>
</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("passthepopcorn")
	if !ok {
		t.Fatal("passthepopcorn definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewGazelle(hc).FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "77" || u.Name != "ptp-user" || u.Level != "Elite" || u.JoinDate != "2024-01-02" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.Ratio != 4 {
		t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", u.Uploaded, u.Downloaded, u.Ratio)
	}
	if u.Seeding != 7 || u.SeedingSize != 3*(1<<40) || u.Uploads != 42 || u.Bonus != 12345.6 || u.BonusPerHour != 12.5 {
		t.Fatalf("stats = seeding %d size %d uploads %d bonus %g bph %g", u.Seeding, u.SeedingSize, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestGazelleUserInfoPagesWithAjaxHeaders(t *testing.T) {
	var sawBonus, sawSeeding, sawUploaded, sawPersonal bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a class="username" href="user.php?id=7">alice</a></body></html>`))
		case "/user.php":
			if r.URL.Query().Get("action") == "edit" {
				sawPersonal = true
				if r.URL.Query().Get("userid") != "7" {
					t.Fatalf("edit userid = %q, want 7", r.URL.Query().Get("userid"))
				}
				_, _ = w.Write([]byte(`<html><body><input name="avatar" value="https://img.example/jps.png"></body></html>`))
				return
			}
			_, _ = w.Write([]byte(`<html><body>
			<a class="username" href="user.php?id=7">alice</a>
			<div>Stats</div><ul class="stats">
			  <li>Uploaded: <span>10.00 TiB</span></li>
			  <li>Downloaded: <span>2.00 TiB</span></li>
			  <li>Ratio: <span>5.0</span></li>
			  <li>Joined: <span title="May 06 2024, 12:00">joined</span></li>
			  <li>Bonus Points: 12345.6</li>
			</ul>
			<div>Community</div><ul class="stats"><li>Seeding: 99</li><li>Uploaded: 3</li></ul>
			<div>Personal</div><ul class="stats"><li>Class: Power User</li></ul>
			</body></html>`))
		case "/bonus.php":
			sawBonus = true
			_, _ = w.Write([]byte(`<html><body><div class="box">You currently earn <strong>12.5 / 300</strong></div></body></html>`))
		case "/ajax.php":
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				t.Fatalf("missing ajax header on %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("userid") != "7" {
				t.Fatalf("userid = %q, want 7", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				sawSeeding = true
				_, _ = w.Write([]byte(`<html><body><table>
				<tr class="torrent"><td>One</td><td class="nobr">1.50 TiB</td></tr>
				<tr class="torrent"><td>Two</td><td class="nobr">512.00 GiB</td></tr>
				</table></body></html>`))
			case "uploaded":
				sawUploaded = true
				_, _ = w.Write([]byte(`<html><body><table>
				<tr class="torrent"><td>A</td></tr>
				<tr class="torrent"><td>B</td></tr>
				<tr class="torrent"><td>C</td></tr>
				</table></body></html>`))
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelle(hc)
	def := &site.SiteMetadata{
		ID:     "jpopsuki",
		Schema: "gazelle",
		UserInfo: site.UserInfoDef{
			Fields: map[string]site.ElementQuery{
				"uploaded":       {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Uploaded:")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:Uploaded|送信量|アップロード数)[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}}},
				"downloaded":     {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Downloaded:")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:Downloaded|受信量)[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}}},
				"ratio":          {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Ratio:")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:Ratio|負担率)[^0-9∞Inf]*([\d.]+|∞|Inf|Infinity)`}}},
				"bonus":          {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Bonus Points")`, `.box:contains("You currently earn")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:Bonus Points|ボーナスポイント|ボーナス|You currently earn)[^0-9]*([\d.,]+)`}, {Name: "parse_number"}}},
				"bonus_per_hour": {Selectors: []string{`.box:contains("You currently earn")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:Bonus Points per Hour/Day|You currently earn)[^0-9]*([\d.]+)`}, {Name: "parse_number"}}},
				"join_date":      {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Joined:") > span`}, Attr: "title", Filters: []site.Filter{{Name: "parse_time"}}},
				"level":          {Selectors: []string{`div:contains("Personal") + ul.stats > li:contains("Class:")`}, Filters: []site.Filter{{Name: "regexp", Args: `^(?:Class:|階級:)\s*(.+)$`}}},
				"seeding":        {Selectors: []string{`div:contains("Community") + ul.stats > li:contains("Seeding:")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:Seeding|シード中)[^0-9]*(\d+)`}, {Name: "parse_number"}}},
				"uploads":        {Selectors: []string{`div:contains("Community") + ul.stats > li:contains("Uploaded:")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:Uploaded|アップロード数)[^0-9]*(\d+)`}, {Name: "parse_number"}}},
			},
			Pages: []site.UserInfoPageDef{
				{Path: "/bonus.php", UseFields: []string{"bonus", "bonus_per_hour"}},
				{
					Path:    "/ajax.php",
					Params:  map[string]string{"section": "torrents", "userid": "{id}", "page": "1", "type": "seeding"},
					Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
					Fields: map[string]site.ElementQuery{
						"seeding":      {Selector: "tr.torrent", Attr: "count"},
						"seeding_size": {Selector: ":self", Attr: "text_all", Filters: []site.Filter{{Name: "sum_size", Args: `([\d.,]+\s*[KMGTPE]i?B)`}}},
					},
				},
				{
					Path:    "/ajax.php",
					Params:  map[string]string{"section": "torrents", "userid": "{id}", "page": "1", "type": "uploaded"},
					Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
					Fields:  map[string]site.ElementQuery{"uploads": {Selector: "tr.torrent", Attr: "count"}},
				},
				{
					Path:   "/user.php",
					Params: map[string]string{"action": "edit", "userid": "{id}"},
					Fields: map[string]site.ElementQuery{
						"personal_avatar": {Selectors: []string{`input[name="avatar"]`, "input#avatar"}, Attr: "value", Filters: []site.Filter{{Name: "clean_avatar"}}},
					},
				},
			},
		},
	}

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if !sawBonus || !sawSeeding || !sawUploaded || !sawPersonal {
		t.Fatalf("pages requested: bonus=%v seeding=%v uploaded=%v personal=%v", sawBonus, sawSeeding, sawUploaded, sawPersonal)
	}
	if ui.ID != "7" || ui.Name != "alice" || ui.Level != "Power User" || ui.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", ui.ID, ui.Name, ui.Level, ui.JoinDate)
	}
	if ui.Uploaded != 10*(1<<40) || ui.Downloaded != 2*(1<<40) || ui.Ratio != 5 {
		t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", ui.Uploaded, ui.Downloaded, ui.Ratio)
	}
	if ui.Seeding != 2 || ui.SeedingSize != 2*(1<<40) || ui.Uploads != 3 {
		t.Fatalf("seed stats = seeding %d size %d uploads %d", ui.Seeding, ui.SeedingSize, ui.Uploads)
	}
	if ui.Bonus != 12.5 || ui.BonusPerHour != 12.5 {
		t.Fatalf("bonus = %g bonus_per_hour = %g", ui.Bonus, ui.BonusPerHour)
	}
	if ui.PersonalAvatar != "https://img.example/jps.png" {
		t.Fatalf("personal_avatar = %q", ui.PersonalAvatar)
	}
}

// TestGazelleLoggedOut：cookie 失效 → 302 跳 login.php → ErrNotLoggedIn。
func TestGazelleLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login.php", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	g := NewGazelle(hc)
	def := &site.SiteMetadata{ID: "gz", Schema: "gazelle"}

	_, err := g.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}
