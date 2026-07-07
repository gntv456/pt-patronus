package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func fetchGenericProcessUserInfo(t *testing.T, id string, handler http.Handler) *site.UserInfo {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewGenericHTML(hc, def.Schema).FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("%s FetchUserInfo: %v", id, err)
	}
	return u
}

func fetchNexusProcessUserInfo(t *testing.T, id string, handler http.Handler) *site.UserInfo {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewNexusPHP(hc).FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("%s FetchUserInfo: %v", id, err)
	}
	return u
}

func fetchGazelleProcessUserInfo(t *testing.T, id string, handler http.Handler) *site.UserInfo {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewGazelle(hc).FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("%s FetchUserInfo: %v", id, err)
	}
	return u
}

func fetchLuminanceProcessUserInfo(t *testing.T, id string, handler http.Handler) *site.UserInfo {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewLuminance(hc).FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("%s FetchUserInfo: %v", id, err)
	}
	return u
}

func fetchUnit3DProcessUserInfo(t *testing.T, id string, handler http.Handler) *site.UserInfo {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewUnit3D(hc).FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("%s FetchUserInfo: %v", id, err)
	}
	return u
}

func TestGenericHTMLFetchAudioNewsUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "audionews", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<a class="user_photo" title="Your profile"><img alt="88"></a>`))
		case "/profile.php":
			if r.URL.Query().Get("mode") != "viewprofile" || r.URL.Query().Get("u") != "88" {
				t.Fatalf("profile query = %q, want mode=viewprofile u=88", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<h1 id="username">an-user</h1>
<b id="rank-name">Power User</b>
<span id="user_regdate">2024-05-06</span>
<span id="user_lastvisit">2024-05-07 12:00:00</span>
<div id="bt_user_ratio"><b>4.0</b></div>
<table>
<tr><td id="u_down_total"><span>512 GiB</span></td></tr>
<tr><td id="u_up_total"><span>2.00 TiB</span></td></tr>
</table>
<span class="dls-cnt">Seed: 7 Self: 3</span>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "88" || u.Name != "an-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) {
		t.Fatalf("sizes = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
	if u.Ratio != 4 || u.Seeding != 7 || u.Uploads != 3 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d", u.Ratio, u.Seeding, u.Uploads)
	}
}

func TestGazelleFetchAnthelionUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGazelleProcessUserInfo(t, "anthelion", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=10">ant-user</a>`))
		case "/store.php":
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=10">ant-user</a><h3 class="float_right">You are making 4.5 Orbs per hour</h3>`))
		case "/user.php":
			if r.URL.Query().Get("id") != "10" {
				t.Fatalf("id = %q, want 10", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=10">ant-user</a>
<ul class="stats">
<li>Uploaded: 2.00 TiB</li>
<li>Downloaded: 512 GiB</li>
<li>Ratio: 4.0</li>
<li>Joined: <span title="2024-05-06T12:00:00Z">joined</span></li>
<li>Last seen: <span title="2024-05-07T12:00:00Z">last</span></li>
<li>Adopted: <span>12</span></li>
<li>Seeding Size: <span>2.50 TiB</span></li>
<li>Class: Fanatic</li>
</ul>
<a href="store.php">12,345.5 Orbs</a>`))
		case "/ajax.php":
			if r.URL.Query().Get("action") != "community_stats" {
				t.Fatalf("action = %q, want community_stats", r.URL.Query().Get("action"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"seeding":7}}`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "10" || u.Name != "ant-user" || u.Level != "Fanatic" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Adoptions != 12 {
		t.Fatalf("adoptions = %d, want 12", u.Adoptions)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != int64(2.5*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 7 || u.Bonus != 12345.5 || u.BonusPerHour != 4.5 {
		t.Fatalf("stats = ratio %g seeding %d bonus %g bph %g", u.Ratio, u.Seeding, u.Bonus, u.BonusPerHour)
	}
}

func TestGazelleFetchMooKoUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGazelleProcessUserInfo(t, "mooko", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=66">mooko-user</a>`))
		case "/ajax.php":
			if r.URL.Query().Get("action") != "index" {
				t.Fatalf("action = %q, want index", r.URL.Query().Get("action"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"userstats":{"bonusPointsPerHour":6.75}}}`))
		case "/user.php":
			if r.URL.Query().Get("id") != "66" {
				t.Fatalf("id = %q, want 66", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=66">mooko-user</a>
<div>Stats</div>
<ul class="stats">
<li>Uploaded: 2.00 TiB</li>
<li>Downloaded: 512 GiB</li>
<li>Ratio: 4.0</li>
<li>Joined: <span title="2024-05-06T12:00:00Z">joined</span></li>
</ul>
<div>Personal</div>
<ul class="stats"><li>Class: Elite</li></ul>
<span id="last-access-date-value" data-value="2024-05-08T12:00:00Z"></span>
<div id="downloaded-value"><span data-tooltip="Downloaded, 768 GiB">shown</span></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "66" || u.Name != "mooko-user" || u.Level != "Elite" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-08" {
		t.Fatalf("last_access_at = %q, want 2024-05-08", u.LastAccessAt)
	}
	if u.TrueDownloaded != 768*(1<<30) {
		t.Fatalf("true_downloaded = %d, want %d", u.TrueDownloaded, int64(768*(1<<30)))
	}
	if u.BonusPerHour != 6.75 {
		t.Fatalf("bonus_per_hour = %g, want 6.75", u.BonusPerHour)
	}
}

func TestGazelleFetchBroadcastTheNetUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGazelleProcessUserInfo(t, "broadcasthenet", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=22">btn-user</a>`))
		case "/index.php":
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=22">btn-user</a>`))
		case "/user.php":
			if r.URL.Query().Get("id") != "22" {
				t.Fatalf("id = %q, want 22", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<a class="username" href="user.php?id=22">btn-user</a>
<ul class="nobullet">
<li>Upload: 3.00 TiB</li>
<li>Uploaded: <span>shows</span><span>8</span></li>
<li>Downloaded: 1.00 TiB</li>
<li>User Class: Power User</li>
<li>Bonus Points: <a>250,000.5</a></li>
<li>Per Day: 240</li>
<li>Joined: <span>2024-05-06</span></li>
<li>Last Seen: <span title="2024-05-07T12:00:00Z">last</span></li>
<li>Seeding: 9</li>
<li>Seeding Size: 4.50 TiB</li>
<li>Total Time Seeded: 2 Days</li>
<li>Total Traffic: 4.00 TiB</li>
<li>Snatched: 3</li>
<li>Snatched: 5</li>
</ul>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "22" || u.Name != "btn-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 3*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != int64(4.5*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.SeedingTime != 2*24*3600 || u.TotalTraffic != 4*(1<<40) || u.Snatches != 8 {
		t.Fatalf("extra stats = seeding_time %d total_traffic %d snatches %d", u.SeedingTime, u.TotalTraffic, u.Snatches)
	}
	if u.Seeding != 9 || u.Uploads != 8 || u.Bonus != 250000.5 || u.BonusPerHour != 10 {
		t.Fatalf("stats = seeding %d uploads %d bonus %g bph %g", u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestGazelleFetchHDFUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGazelleProcessUserInfo(t, "hdf", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<h2><a href="user.php?id=77"><span>hdf-user</span></a></h2>`))
		case "/user.php":
			if r.URL.Query().Get("id") != "77" {
				t.Fatalf("id = %q, want 77", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<h2><a href="user.php?id=77"><span>hdf-user</span></a></h2>
<div class="user-name-class"><span class="class_elite">hdf-user</span></div>
<div>Statistiques</div>
<ul class="stats">
<li class="tooltip" title="2.00 TiB">Uploadé :</li>
<li class="tooltip" title="512 GiB">Téléchargé :</li>
<li>Ratio : <span class="tooltip" title="2.5">ratio</span></li>
<li>Points par heure : 4.25</li>
<li>Inscription : <span class="time tooltip" title="2024-05-06T12:00:00Z">joined</span></li>
</ul>
<div>Infos personnelles</div>
<ul class="stats"><li>Rang : Elite</li></ul>
<a id="bonus_points_number_link"><span>12,345.5</span></a>
<span id="comm_upload">8</span>
<span id="comm_seedsize"><span class="tooltip">Taille seedée : 3.00 TiB</span></span>`))
		case "/ajax.php":
			if r.URL.Query().Get("action") != "community_stats" {
				t.Fatalf("action = %q, want community_stats", r.URL.Query().Get("action"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"seeding":6}}`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "77" || u.Name != "hdf-user" || u.Level != "Elite" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 2.5 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchDigitalCoreUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "digitalcore", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/status":
			_, _ = w.Write([]byte(`{"user":{
				"id": 33,
				"username": "dc-user",
				"uploaded": "2.00 TiB",
				"downloaded": "512 GiB",
				"bonuspoang": 12345.5,
				"myseeds": 7,
				"mytorrents": 4,
				"currentGbSeed": 3072
			}}`))
		case "/api/v1/users/33":
			_, _ = w.Write([]byte(`{
				"added": "2024-05-06T12:00:00Z",
				"uploaded": "2.00 TiB",
				"uploaded_real": "3.50 TiB",
				"downloaded": "512 GiB",
				"bonuspoang": 12345.5,
				"class": 20
			}`))
		case "/api/v1/users/33/bonuslog":
			_, _ = w.Write([]byte(`{"line":"[b]240p[/b] paid hourly for seeding"}`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "33" || u.Name != "dc-user" || u.Level != "Viceroy" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.TrueUploaded != int64(3.5*float64(1<<40)) || u.LevelID != 20 {
		t.Fatalf("extras = true_uploaded %d level_id %d", u.TrueUploaded, u.LevelID)
	}
	if u.Seeding != 7 || u.Uploads != 4 || u.Bonus != 12345.5 || u.BonusPerHour != 10 {
		t.Fatalf("stats = seeding %d uploads %d bonus %g bph %g", u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchHDRouteUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "hdroute", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<div class="headerRightInfo">
<span><a href="userdetail.php?id=44">hdr-user</a></span>
<span>上传量 <span class="header-user-data">4.00 TiB</span></span>
<span>下载量 <span class="header-user-data">1.00 TiB</span></span>
<span>分享率 <span class="header-user-data">4.0</span></span>
<span>(VIP/Power)</span>
<span class="header-user-data"><a href="list_seeding.php">6</a><span class="peering-size">2.25 TiB</span></span>
</div>`))
		case "/userdetail.php":
			if r.URL.Query().Get("id") != "44" {
				t.Fatalf("id = %q, want 44", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<div class="userdetail-list-title">注册日期</div><div class="userdetail-list-content">2024-05-06</div>`))
		case "/list_uploaded.php":
			if r.URL.Query().Get("id") != "44" {
				t.Fatalf("id = %q, want 44", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<div id="pager-top"><a>1</a><a><b>9</b></a></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "44" || u.Name != "hdr-user" || u.Level != "VIP/Power" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 4*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != int64(2.25*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 9 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d", u.Ratio, u.Seeding, u.Uploads)
	}
}

func TestGenericHTMLFetchHDSpaceUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "hdspace", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			switch r.URL.Query().Get("page") {
			case "":
				_, _ = w.Write([]byte(`<a href="index.php?page=usercp&uid=99">settings</a>
<table>
<tr><td align="center" style="text-align:center;">Welcome back <span>hds-user</span></td></tr>
<tr><td align="center" style="text-align:center;">Rank: HD Spacer</td></tr>
<tr><td class="green">UP 2.00 TiB</td><td class="red">DL 512 GiB</td><td class="yellow">Ratio 4.0</td></tr>
<tr><td class="green">Bonus: 12345.5</td></tr>
<tr><td><img src="images/actseed.png"><font color="green">6</font></td></tr>
</table>`))
			case "usercp":
				if r.URL.Query().Get("uid") != "99" {
					t.Fatalf("uid = %q, want 99", r.URL.Query().Get("uid"))
				}
				_, _ = w.Write([]byte(`<table>
<tr><td class="header">Joined on</td><td>2024-05-06</td></tr>
<tr><td class="header">Last access</td><td>2024-05-07</td></tr>
</table>`))
			case "modules":
				if r.URL.Query().Get("module") != "seedbonus" {
					t.Fatalf("module = %q, want seedbonus", r.URL.Query().Get("module"))
				}
				_, _ = w.Write([]byte(`<table><tr><td valign="top"><h3>4.25 points per hour</h3></td></tr></table>`))
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "99" || u.Name != "hds-user" || u.Level != "HD Spacer" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) {
		t.Fatalf("sizes = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d bonus %g bph %g", u.Ratio, u.Seeding, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchMilkieUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "milkie", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{
			"displayName": "milkie-user",
			"uploaded": "2.00 TiB",
			"downloaded": "512 GiB",
			"createdAt": "2024-05-06T12:00:00Z"
		}}`))
	}))

	if u.Name != "milkie-user" || u.Level != "Early bird" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = name %q level %q join %q", u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) {
		t.Fatalf("sizes = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
}

func TestGenericHTMLFetchNCoreUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "ncore", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/profile.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<input data-uid="101">
<div id="profil_nev"><span>ncore-user</span></div>
<div id="profil_rang"><nobr>Elite()</nobr></div>
<section>
<div class="placeholder"></div>
<div class="userbox_tartalom_mini">
<div>1</div>
<div><span title="2024-05-06T12:00:00Z">joined</span></div>
<div>3</div>
<div>4</div>
<div>5</div>
<div>6</div>
<div>7</div>
<div><span>2.00 TiB</span></div>
<div>9</div>
<div>12,345.5 pont</div>
</div>
</section>`))
	}))

	if u.ID != "101" || u.Name != "ncore-user" || u.Level != "Elite" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) {
		t.Fatalf("uploaded = %d", u.Uploaded)
	}
	if u.Bonus != 12345.5 {
		t.Fatalf("bonus = %g", u.Bonus)
	}
}

func TestGazelleFetchNebulanceUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGazelleProcessUserInfo(t, "nebulance", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
		case "/ajax.php":
			if r.URL.Query().Get("action") != "index" {
				t.Fatalf("action = %q, want index", r.URL.Query().Get("action"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"id":55,"username":"nbl-user"}}`))
		case "/user.php":
			if r.URL.Query().Get("id") != "55" {
				t.Fatalf("id = %q, want 55", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<a href="bonus.php">12,345.5</a>
<ul>
<li>Joined <span class="time" title="2024-05-06T12:00:00Z">joined</span></li>
<li>Last Seen <span class="time" title="2024-05-07T12:00:00Z">last</span></li>
<li>Class: Viper</li>
<li>Downloaded: 1.00 TiB</li>
<li>Uploaded: 2.00 TiB <span>uploads</span><span>9</span></li>
<li>Cubits per hour 8.5</li>
<li>Seeding <span>7</span></li>
<li>Seeding Size 3.50 TiB</li>
<li><b>Snatched</b><span>11</span></li>
</ul>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "55" || u.Name != "nbl-user" || u.Level != "Viper" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != int64(3.5*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Snatches != 11 {
		t.Fatalf("snatches = %d, want 11", u.Snatches)
	}
	if u.Seeding != 7 || u.Uploads != 9 || u.Bonus != 12345.5 || u.BonusPerHour != 8.5 {
		t.Fatalf("stats = seeding %d uploads %d bonus %g bph %g", u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestUnit3DFetchNordicBytesUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchUnit3DProcessUserInfo(t, "nordicbytes", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<nav class="__navbar-subnav"><a href="/u/nordic-user"><strong>nordic-user</strong></a></nav>
<ul class="__navbar-stats">
<li><a><i class="fa-arrow-up"></i><span>1.50 TiB (4)</span></a></li>
<li><a><i class="fa-arrow-down"></i><span>512 GiB (1)</span></a></li>
<li><a><i class="fa-percent"></i><span>3.0</span></a></li>
</ul>`))
		case "/u/nordic-user":
			_, _ = w.Write([]byte(`<div class="__profile-badges">
<span class="badge"><i class="fa-user"></i>Explorer</span>
<time datetime="2024-05-06T12:00:00Z"></time>
<span class="__bon">9876.5</span>
</div>
<ul class="__profile-stats">
<li data-template="user-seedsize"><strong>2.75 TiB</strong></li>
<li data-template="user-upload"><span>5</span></li>
</ul>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.Name != "nordic-user" || u.Level != "Explorer" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = name %q level %q join %q", u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != int64(1.5*float64(1<<40)) || u.Downloaded != 512*(1<<30) || u.SeedingSize != int64(2.75*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 3 || u.Seeding != 4 || u.Leeching != 1 || u.Uploads != 5 || u.Bonus != 9876.5 {
		t.Fatalf("stats = ratio %g seeding %d leeching %d uploads %d bonus %g", u.Ratio, u.Seeding, u.Leeching, u.Uploads, u.Bonus)
	}
}

func TestUnit3DFetchAura4kUserInfoFromMigratedFields(t *testing.T) {
	u := fetchUnit3DProcessUserInfo(t, "aura4k", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="/users/aura-user/settings">Aura</a>`))
		case "/users/aura-user":
			_, _ = w.Write([]byte(`<ul class="ratio-bar">
<li class="ratio-bar__uploaded"><a><i class="fa-arrow-up"></i> 2.00 TiB</a></li>
<li class="ratio-bar__downloaded"><a><i class="fa-arrow-down"></i> 512 GiB</a></li>
<li class="ratio-bar__ratio"><a><i class="fa-sync-alt"></i> 4.0</a></li>
<li class="ratio-bar__points"><a><i class="fa-coins"></i> 12345.5</a></li>
<li class="ratio-bar__seeding"><a><i class="fa-upload"></i> 7</a></li>
<li class="ratio-bar__leeching"><a><i class="fa-download"></i> 1</a></li>
</ul>
<div class="wow-badges"><span class="wow-badge">Radiant</span></div>
<time class="profile__registration" datetime="2024-05-06T12:00:00Z">joined</time>
<dl><dt>Seeding Size</dt><dd>3.00 TiB</dd></dl>
<main><aside><section>
<div>ignore</div>
<div><div>one</div><div>two</div><div>three</div><div><div>4.25 per hour</div></div></div>
</section></aside></main>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.Name != "aura-user" || u.Level != "Radiant" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = name %q level %q join %q", u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 7 || u.Leeching != 1 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d leeching %d bonus %g bph %g", u.Ratio, u.Seeding, u.Leeching, u.Bonus, u.BonusPerHour)
	}
}

func TestUnit3DFetchSeedpoolUserInfoFromMigratedFields(t *testing.T) {
	u := fetchUnit3DProcessUserInfo(t, "seedpool", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="/users/pool-user/settings">Pool</a>`))
		case "/users/pool-user":
			_, _ = w.Write([]byte(`<ul class="ratio-bar">
<li class="ratio-bar__uploaded"><a><i class="fa-arrow-up"></i> 2.00 TiB</a><a href="/users/pool-user/active?seeding=include">6</a></li>
<li class="ratio-bar__downloaded"><a><i class="fa-arrow-down"></i> 512 GiB</a></li>
<li class="ratio-bar__ratio"><a><i class="fa-sync-alt"></i> 4.0</a></li>
<li class="ratio-bar__points"><a><i class="fa-coins"></i> 99999.9</a></li>
<li class="ratio-bar__leeching"><a><i class="fa-download"></i> 1</a></li>
</ul>
<div class="content"><span class="badge-user" title="PowerPool"></span></div>
<time class="profile__registration" datetime="2024-05-06T12:00:00Z">joined</time>
<dl><dt>Seedpool</dt><dd>3.00 TiB</dd></dl>`))
		case "/users/pool-user/earnings":
			_, _ = w.Write([]byte(`<div class="panelV2"><dl class="key-value"><dd>x</dd><dd>y</dd><dd>12.5</dd></dl></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.Name != "pool-user" || u.Level != "PowerPool" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = name %q level %q join %q", u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Leeching != 1 || u.Bonus != 0 || u.BonusPerHour != 0 {
		t.Fatalf("stats = ratio %g seeding %d leeching %d bonus %g bph %g", u.Ratio, u.Seeding, u.Leeching, u.Bonus, u.BonusPerHour)
	}
}

func TestUnit3DFetchYUSCENEUserInfoFromMigratedFields(t *testing.T) {
	u := fetchUnit3DProcessUserInfo(t, "yuscene", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="/users/yu-user/settings">YU</a>`))
		case "/users/yu-user":
			_, _ = w.Write([]byte(`<ul class="ratio-bar">
<li class="ratio-bar__uploaded"><a><i class="fa-arrow-up"></i> 2.00 TiB</a></li>
<li class="ratio-bar__downloaded"><a><i class="fa-arrow-down"></i> 512 GiB</a></li>
<li class="ratio-bar__ratio"><a><i class="fa-sync-alt"></i> 4.0</a></li>
<li class="ratio-bar__points"><a><i class="fa-coins"></i> 12345.5</a></li>
<li class="ratio-bar__seeding"><a><i class="fa-upload"></i> 7</a></li>
<li class="ratio-bar__leeching"><a><i class="fa-download"></i> 1</a></li>
</ul>
<span class="user-tag profile-hero__username"><a class="user-tag__link" title="Archivist">yu-user</a></span>
<time class="profile-hero__meta-item--registration" datetime="2024-05-06T12:00:00Z"><strong class="profile-hero__meta-value">joined</strong></time>
<dl><dt>Seeding Size</dt><dd>3.00 TiB</dd></dl>`))
		case "/users/yu-user/earnings":
			_, _ = w.Write([]byte(`<div class="panelV2"><dl class="key-value"><dd>x</dd><dd>y</dd><dd>4.25</dd></dl></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.Name != "yu-user" || u.Level != "Archivist" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = name %q level %q join %q", u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 7 || u.Leeching != 1 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d leeching %d bonus %g bph %g", u.Ratio, u.Seeding, u.Leeching, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchSpeedAppUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "speedapp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/profile":
			_, _ = w.Write([]byte(`<div id="kt_quick_user_toggle"><span class="text-dark-50">speed-user</span></div>
<div class="card-body pt-4"><div class="align-items-center"><div class="text-muted">Power User</div></div></div>
<dl>
<dt>Uploaded</dt><dd>2.00 TiB</dd>
<dt>Downloaded</dt><dd>512 GiB</dd>
<dt>Ratio</dt><dd>4.0</dd>
<dt>Signup date</dt><dd>2024-05-06</dd>
<dt>Last access</dt><dd>2024-05-07</dd>
<dt>Bonus points</dt><dd><b>6.5</b><b>3.25 TiB</b></dd>
</dl>`))
		case "/profile/menu-stats":
			_, _ = w.Write([]byte(`<a href="/shop" title="Bonus points">12,345.5</a><a href="/snatch/seeding" title="Currently seeding torrents">8</a>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.Name != "speed-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = name %q level %q join %q", u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != int64(3.25*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 6.5 {
		t.Fatalf("stats = ratio %g seeding %d bonus %g bph %g", u.Ratio, u.Seeding, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchBakaBTUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "bakabt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a class="username" href="/user/42/">bakabt-user</a>`))
		case "/bonus.php":
			_, _ = w.Write([]byte(`<b class="success">7.25</b>`))
		case "/user/42/":
			_, _ = w.Write([]byte(`<a class="username" href="/user/42/">bakabt-user</a><h2><span>(Member - Power User)</span></h2>
<table>
<tr><td>Join date</td><td><span class="datetime" data-timestamp="1714953600">joined</span></td></tr>
<tr><td>Last seen</td><td><span class="datetime" data-timestamp="1715040000">seen</span></td></tr>
<tr><td>Uploaded</td><td>1.25 TiB - credited</td></tr>
<tr><td>Downloaded</td><td>512 GiB - counted</td></tr>
<tr><td>Share ratio</td><td><span>2.5</span></td></tr>
<tr><td>Bonus Points</td><td>1234.5</td></tr>
</table>
<ul><li id="uploaded_tab"><a>3 uploaded</a></li><li id="active_tab"><a>2 active</a></li></ul>
<div id="tab_content"><div id="active"><table class="torrents"><tbody>
<tr><td>one</td><td class="size">1.00 GiB</td></tr>
<tr><td>two</td><td class="size">512 MiB</td></tr>
</tbody></table></div></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "42" || u.Name != "bakabt-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != int64(1280*(1<<30)) || u.Downloaded != 512*(1<<30) || u.SeedingSize != int64(1536*(1<<20)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 2.5 || u.Seeding != 2 || u.Uploads != 3 || u.Bonus != 1234.5 || u.BonusPerHour != 7.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchCinemageddonUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "cinemageddon", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<div class="statusbar"><a href="/userdetails.php?id=77">cg-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "77" {
				t.Fatalf("id = %q, want 77", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table><tr><td class="clx"><table class="frames">
<tr><td class="rowhead">Uploaded</td><td>2.00 TiB</td></tr>
<tr><td class="rowhead">Downloaded</td><td>1.00 TiB</td></tr>
<tr><td class="rowhead">Class</td><td>CG Superfan</td></tr>
<tr><td class="rowhead">Join date</td><td>2024-05-06 (1 year ago)</td></tr>
</table></td></tr></table>
<a href="/credits.php">4,321.5 credits</a>
<div id="ka2"><table>
<tr><th>Name</th><th>Ignore</th><th>Size</th></tr>
<tr><td>A</td><td>x</td><td>1.25 GiB</td></tr>
<tr><td>B</td><td>x</td><td>768 MiB</td></tr>
</table></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "77" || u.Name != "cg-user" || u.Level != "CG Superfan" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != int64(2048*(1<<20)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Seeding != 2 || u.Bonus != 4321.5 {
		t.Fatalf("stats = seeding %d bonus %g", u.Seeding, u.Bonus)
	}
}

func TestGenericHTMLFetchGTRUUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "gtru", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userdetails.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<h1 class="ud-profile-name">gtru-user</h1>
<div class="ud-profile-meta"><img src="/pic/classes/power.gif"><span><i class="fa-calendar-o"></i>2024-05-06 (1 year)</span><span><i class="fa-clock-o"></i>2024-05-07 (today)</span></div>
<div class="ud-stat-row"><span>Uploaded</span><span class="ud-stat-value">1.50 TiB [credited]</span></div>
<div class="ud-stat-row"><span>Downloaded</span><span class="ud-stat-value">512 GiB [counted]</span></div>
<div class="ud-stat-row"><span>Share ratio</span><span class="ud-stat-value">3.0</span></div>
<div class="ud-stat-row"><span>Total Seed Bonus</span><span class="ud-stat-value">9876.5</span></div>
<span id="activeseed">4 active</span>
<div id="uploaded"><table class="dc-torrent-table"><tbody><tr><td>u1</td></tr><tr><td>u2</td></tr></tbody></table></div>
<div id="seeding"><table class="dc-torrent-table"><tbody>
<tr><td>A</td><td>x</td><td>2.00 GiB</td></tr>
<tr><td>B</td><td>x</td><td>512 MiB</td></tr>
</tbody></table></div>`))
	}))

	if u.Name != "gtru-user" || u.Level != "PowerUser" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = name %q level %q join %q", u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != int64(1536*(1<<30)) || u.Downloaded != 512*(1<<30) || u.SeedingSize != int64(2560*(1<<20)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 3 || u.Seeding != 4 || u.Uploads != 2 || u.Bonus != 9876.5 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus)
	}
}

func TestGenericHTMLFetchHDBitsUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "hdbits", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<a href="/userdetails.php?id=8">hdb-user</a>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "8" {
				t.Fatalf("id = %q, want 8", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">Uploaded</td><td>5.00 TiB (credited)</td></tr>
<tr><td class="rowhead">Downloaded</td><td>2.00 TiB (counted)</td></tr>
<tr><td class="rowhead">Share ratio</td><td>2.5</td></tr>
<tr><td class="rowhead">Class</td><td>UHD</td></tr>
<tr><td class="rowhead">Bonus</td><td>12,345.5</td></tr>
<tr><td class="rowhead">JOIN date</td><td>2024-05-06 (joined)</td></tr>
<tr><td class="rowhead">Last seen</td><td>2024-05-07 (last)</td></tr>
<tr><td class="rowhead">Seeding size</td><td>6.50 TiB (total)</td></tr>
<tr><td class="heading">Currently seeding</td><td>11</td></tr>
<tr><td class="heading">Uploaded torrents</td><td>7</td></tr>
<tr><td class="heading">Bonus per hour</td><td>4.25</td></tr>
</table>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "8" || u.Name != "hdb-user" || u.Level != "UHD" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 5*(1<<40) || u.Downloaded != 2*(1<<40) || u.SeedingSize != int64(6.5*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 2.5 || u.Seeding != 11 || u.Uploads != 7 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchHDTorrentsUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "hdtorrents", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="usercp.php?uid=9">profile</a><table><tr><td class="nav" title="Active-Torrents"><a href="#actives"><span>12</span></a></td></tr></table>`))
		case "/usercp.php":
			if r.URL.Query().Get("uid") != "9" {
				t.Fatalf("uid = %q, want 9", r.URL.Query().Get("uid"))
			}
			_, _ = w.Write([]byte(`<table>
<tr id="CurrentDetailsHideShowTR"><td class="header">User</td><td>hdt-user</td></tr>
<tr><td class="header">Uploaded</td><td>3.00 TiB</td></tr>
<tr><td class="header">Downloaded</td><td>1.00 TiB</td></tr>
<tr><td class="header">Ratio</td><td>3.0</td></tr>
<tr><td class="header">Rank</td><td>HD Monster</td></tr>
<tr><td class="header">Seed Bonus Points</td><td>777.5</td></tr>
<tr><td class="header">Joined on</td><td>2024-05-06</td></tr>
<tr><td class="header">Last access</td><td>2024-05-07</td></tr>
<tr><td class="header">Total Size of torrents</td><td>4.25 TiB</td></tr>
</table>`))
		case "/seedbonus.php":
			_, _ = w.Write([]byte(`<div id="BonusPointsHideShowTR"><div class="blockcontent"><center><h1>
<font color="blue">ignore</font><font color="blue">also ignore</font><font color="blue">8.5</font>
</h1></center></div></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "9" || u.Name != "hdt-user" || u.Level != "HD Monster" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 3*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != int64(4.25*float64(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 3 || u.Seeding != 12 || u.Bonus != 777.5 || u.BonusPerHour != 8.5 {
		t.Fatalf("stats = ratio %g seeding %d bonus %g bph %g", u.Ratio, u.Seeding, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchKaraGargaUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "karagarga", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a title="click to see your details page" href="userdetails.php?id=12">kg-user</a>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "12" {
				t.Fatalf("id = %q, want 12", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">Uploaded</td><td>2.50 TiB (5 uploads)</td></tr>
<tr><td class="rowhead">Downloaded</td><td>1.00 TiB</td></tr>
<tr><td class="rowhead">Share ratio</td><td><table><tbody><tr><td><font>2.5</font></td></tr></tbody></table></td></tr>
<tr><td class="rowhead">Class</td><td>Power User</td></tr>
<tr><td class="rowhead">Join date</td><td>2024-05-06 (joined)</td></tr>
</table>`))
		case "/current.php":
			if r.URL.Query().Get("id") != "12" {
				t.Fatalf("current id = %q, want 12", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table id="browse"><tbody>
<tr><td>1</td><td><a href="details.php?id=1">A</a></td><td>3</td><td>4</td><td>5</td><td>6</td><td>7</td><td>8</td><td>9</td><td>10</td><td>1.00 GiB</td></tr>
<tr><td>1</td><td><a href="details.php?id=2">B</a></td><td>3</td><td>4</td><td>5</td><td>6</td><td>7</td><td>8</td><td>9</td><td>10</td><td>512 MiB</td></tr>
</tbody></table>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "12" || u.Name != "kg-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != int64(2560*(1<<30)) || u.Downloaded != 1*(1<<40) || u.SeedingSize != int64(1536*(1<<20)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 2.5 || u.Seeding != 2 || u.Uploads != 5 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d", u.Ratio, u.Seeding, u.Uploads)
	}
}

func TestGenericHTMLFetchPornoLabUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "pornolab", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forum/index.php":
			_, _ = w.Write([]byte(`<a href="/forum/profile.php?mode=viewprofile&u=55">me</a>`))
		case "/forum/profile.php":
			if r.URL.Query().Get("u") != "55" || r.URL.Query().Get("mode") != "viewprofile" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<h1 class="pagetitle"><span>pl-user</span></h1>
<table>
<tr><td id="role">User</td></tr>
<tr><td id="user_regdate"><span>2024-05-06</span></td></tr>
<tr><td id="user_lastvisit"><span>2024-05-07</span></td></tr>
<tr><td id="u_ratio"><b>2.25</b></td></tr>
<tr><td id="u_down_total"><span>1.00 TiB</span></td></tr>
<tr><td id="u_up_total"><span>2.00 TiB</span></td></tr>
</table>
<div class="seed">3 seeding</div><div class="seed">7 uploaded</div>
<table id="t-seed"><tbody>
<tr><td>1</td><td>2</td><td>3</td><td>4</td><td>5</td><td><u>1.00 GiB</u></td></tr>
<tr><td>1</td><td>2</td><td>3</td><td>4</td><td>5</td><td><u>512 MiB</u></td></tr>
</tbody></table>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "55" || u.Name != "pl-user" || u.Level != "User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != int64(1536*(1<<20)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 2.25 || u.Seeding != 3 || u.Uploads != 7 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d", u.Ratio, u.Seeding, u.Uploads)
	}
}

func TestGenericHTMLFetchMyAnonamouseUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "myanonamouse", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jsonLoad.php":
			if r.URL.RawQuery != "snatch_summary&notif" {
				t.Fatalf("query = %q, want snatch_summary&notif", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "uid": 101,
  "username": "mam-user",
  "ratio": "2.75",
  "seedbonus": "9876.5",
  "uploaded_bytes": 2199023255552,
  "downloaded_bytes": 549755813888,
  "classname": "Power User",
  "update": 1715040000,
  "seedUnsat": {"count": 2, "size": 1073741824},
  "seedHnr": {"count": 1, "size": 536870912},
  "sSat": {"count": 3, "size": 2147483648},
  "upAct": {"count": 4, "size": 3221225472},
  "upInact": {"count": 5, "size": 4294967296}
}`))
		case "/u/101":
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">Join date</td><td>2024-05-06 (joined)</td></tr>
<tr><td class="rowhead">Points earning</td><td>Your seeds are worth 12.5 per hour</td></tr>
<tr><td class="rowhead">Real Uploaded</td><td>3.00 TiB</td></tr>
<tr><td class="rowhead">Real Downloaded</td><td>768 GiB</td></tr>
<tr><td class="rowhead">Real share ratio</td><td>4.0</td></tr>
</table>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "101" || u.Name != "mam-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != int64(6.5*float64(1<<30)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.TrueUploaded != 3*(1<<40) || u.TrueDownloaded != 768*(1<<30) || u.TrueRatio != 4 {
		t.Fatalf("true traffic = uploaded %d downloaded %d ratio %g", u.TrueUploaded, u.TrueDownloaded, u.TrueRatio)
	}
	if u.Ratio != 2.75 || u.Seeding != 10 || u.Uploads != 9 || u.Bonus != 9876.5 || u.BonusPerHour != 12.5 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchU2UserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "u2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usercp.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=88">u2-user</a>
<div><font class="color_uploaded">Uploaded:</font> 3.00 TiB</div>
<div><font class="color_downloaded">Downloaded:</font> 1.00 TiB</div>
<div><font class="color_ratio">Ratio:</font> 3.0</div>
<table><tr><td class="rowhead">UCoin</td><td><span title="12345.5">coins</span></td></tr></table>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "88" {
				t.Fatalf("id = %q, want 88", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">等级</td><td><img title="宅贤者"></td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00</td></tr>
</table>`))
		case "/mprecent.php":
			if r.URL.Query().Get("user") != "88" {
				t.Fatalf("user = %q, want 88", r.URL.Query().Get("user"))
			}
			_, _ = w.Write([]byte(`<table class="main"><tr><td class="text">UCoin 240.0 earned today</td></tr></table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<html></html>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "88" {
				t.Fatalf("userid = %q, want 88", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 4.00 GiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>5</b>条记录</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "88" || u.Name != "u2-user" || u.Level != "宅贤者" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 3*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != 4*(1<<30) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 3 || u.Seeding != 6 || u.Uploads != 5 || u.Bonus != 12345.5 || u.BonusPerHour != 10 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchBYRBTUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "byrbt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usercp.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=66">byr-user</a>
<div><font class="color_uploaded">上传量:</font> 2.00 TiB</div>
<div><font class="color_downloaded">下载量:</font> 512 GiB</div>
<div><font class="color_ratio">分享率:</font> 4.0</div>
<div><font class="color_active">当前活动:</font> 6 0</div>
<div><font class="color_bonus">魔力值</font> [使用]: 12345.5</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "66" {
				t.Fatalf("id = %q, want 66", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00</td></tr>
</table>`))
		case "/mybonus.php":
			if r.URL.Query().Get("show") != "seed" {
				t.Fatalf("show = %q, want seed", r.URL.Query().Get("show"))
			}
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 4.25 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "66" {
				t.Fatalf("userid = %q, want 66", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b>条记录</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "66" || u.Name != "byr-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchHaidanUserInfoFromMigratedFields(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "haidan", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usercp.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=67">haidan-user</a>
<div><font class="color_uploaded">上传量:</font> 2.00 TiB</div>
<div><font class="color_downloaded">下载量:</font> 512 GiB</div>
<div><font class="color_ratio">分享率:</font> 4.0</div>
<div><font class="color_active">当前活动:</font> 6 0</div>
<div><font class="color_bonus">魔力值</font> [使用]: 12345.5</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "67" {
				t.Fatalf("id = %q, want 67", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">等级</td><td><img title="(2) Elite User" src="/pic/class/3.gif"></td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 4.25 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "67" {
				t.Fatalf("userid = %q, want 67", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b>条记录</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "67" || u.Name != "haidan-user" || u.Level != "Elite User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchSpringSundayUserInfoFromMigratedFields(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "springsunday", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usercp.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=68">spring-user</a>
<div><font class="color_uploaded">上传量:</font> 2.00 TiB</div>
<div><font class="color_downloaded">下载量:</font> 512 GiB</div>
<div><font class="color_ratio">分享率:</font> 4.0</div>
<div><font class="color_active">当前活动:</font> 0 0</div>
<table><tr><td class="rowhead">积分</td><td>茉莉 12,345.5 [使用]</td></tr></table>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "68" {
				t.Fatalf("id = %q, want 68", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">等级</td><td><img title="大师(Master)"></td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table><tbody><tr class="nowrap"><td>ignore</td><td>4.25 / h</td></tr></tbody></table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "68" {
				t.Fatalf("userid = %q, want 68", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div><b>6</b>条记录 <span>合计</span> <b>3.00 TiB</b></div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b>条记录</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "68" || u.Name != "spring-user" || u.Level != "大师(Master)" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchZRPTUserInfoFromMigratedFields(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "zrpt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usercp.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=69">zr-user</a>
<div><font class="color_uploaded">上传量:</font> 2.00 TiB</div>
<div><font class="color_downloaded">下载量:</font> 512 GiB</div>
<div><font class="color_ratio">分享率:</font> 4.0</div>
<div><font class="color_active">当前活动:</font> 6 0</div>
<ul><li>积分: 12,345.5 [使用]</li></ul>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "69" {
				t.Fatalf("id = %q, want 69", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">等级</td><td><img title="(2) Power User" src="/pic/class/3.gif"></td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 4.25 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "69" {
				t.Fatalf("userid = %q, want 69", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b>条记录</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "69" || u.Name != "zr-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchRetroFlixUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "retroflix", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=70">retro-user</a>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "70" {
				t.Fatalf("id = %q, want 70", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">Username</td><td>retro-user</td></tr>
<tr><td class="rowhead">Uploaded</td><td>2.00 TiB</td></tr>
<tr><td class="rowhead">Downloaded</td><td>512 GiB</td></tr>
<tr><td class="rowhead">Share ratio</td><td>4.0</td></tr>
<tr><td class="rowhead">Class</td><td>Movie Lover</td></tr>
<tr><td class="rowhead">Bonus Points</td><td>12,345.5</td></tr>
<tr><td class="rowhead">Join date</td><td>06-05-2024 12:00:00 (1 year ago)</td></tr>
<tr><td class="rowhead">Currently seeding</td><td>6 torrents / 3.00 TiB</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>You are currently getting 4.25 points per hour.</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "70" {
				t.Fatalf("userid = %q, want 70", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			case "seeding":
				_, _ = w.Write([]byte(`<html></html>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "70" || u.Name != "retro-user" || u.Level != "Movie Lover" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchKeepFRDSUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "keepfrds", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block">欢迎回来
<a href="userdetails.php?id=71">frds-user</a> (Power User)</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "71" {
				t.Fatalf("id = %q, want 71", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<a href="messages.php"><b><span style="color: red">2</span></b></a>
<table>
<tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
<tr><td class="rowhead">魔力值</td><td>魔力值: 12,345.5 [4.25 魔力/h]</td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00 (1 year ago)</td></tr>
<tr><td class="rowhead">当前做种</td><td><a href="/torrents.php?option-torrents=3">6</a> torrents / 3.00 TiB</td></tr>
</table>
<div id="info_block"><a href="myhr.php">H&amp;R 1 / 2 / 3</a></div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "71" {
				t.Fatalf("userid = %q, want 71", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			case "seeding":
				_, _ = w.Write([]byte(`<html></html>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "71" || u.Name != "frds-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchOurBitsUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "ourbits", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block">
<a href="userdetails.php?id=72">ob-user</a>
上传量 2.00 TiB 下载量 512 GiB 分享率 4.0
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "72" {
				t.Fatalf("id = %q, want 72", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
<tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
<tr><td class="rowhead">魔力值</td><td>魔力值: 12,345.5</td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00 (1 year ago)</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 4.25 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "72" {
				t.Fatalf("userid = %q, want 72", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "72" || u.Name != "ob-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchAGSVPTUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "agsvpt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block">
<a href="userdetails.php?id=73">agsv-user</a>
上传量 2.00 TiB 下载量 512 GiB 分享率 4.0 冰晶: 12,345.5
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "73" {
				t.Fatalf("id = %q, want 73", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
<tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
<tr><td class="rowhead">冰晶</td><td>冰晶: 12,345.5</td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00 (1 year ago)</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<h1>每小时获得的合计冰晶</h1><div>4.25</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "73" {
				t.Fatalf("userid = %q, want 73", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "73" || u.Name != "agsv-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchCyanBugUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "cyanbug", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block">
<a href="userdetails.php?id=74">cyan-user</a>
上传量: 2.00 TiB 下载量: 512 GiB 分享率: 4.0 魔力值: 12,345.5
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "74" {
				t.Fatalf("id = %q, want 74", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
<tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
<tr><td class="rowhead">魔力值</td><td>魔力值: 12,345.5</td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00 (1 year ago)</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 4.25 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "74" {
				t.Fatalf("userid = %q, want 74", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "74" || u.Name != "cyan-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchHDTimeUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "hdtime", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block">
<a href="userdetails.php?id=75">time-user</a>
魔力值: 12,345.5
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "75" {
				t.Fatalf("id = %q, want 75", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
<tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
<tr><td class="rowhead">魔力值</td><td>魔力值: 12,345.5</td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00 (1 year ago)</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 4.25 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "75" {
				t.Fatalf("userid = %q, want 75", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "75" || u.Name != "time-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchSoulVoiceUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "soulvoice", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block">
<a href="userdetails.php?id=76">voice-user</a>
上传量: 2.00 TiB 下载量: 512 GiB 分享率: 4.0 魔力值: 12,345.5
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "76" {
				t.Fatalf("id = %q, want 76", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
<tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
<tr><td class="rowhead">魔力值</td><td>魔力值: 12,345.5</td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00 (1 year ago)</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 4.25 个魔力值。</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "76" {
				t.Fatalf("userid = %q, want 76", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "76" || u.Name != "voice-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestNexusPHPFetchUBitsUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchNexusProcessUserInfo(t, "ubits", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block">
<a href="userdetails.php?id=77">ub-user</a>
<font class="color_uploaded">上传量</font>: 2.00 TiB
<font class="color_downloaded">下载量</font>: 512 GiB
<font class="color_ratio">分享率</font>: 4.0
<font class="color_bonus">U币</font>: 12,345.5
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "77" {
				t.Fatalf("id = %q, want 77", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
<tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
<tr><td class="rowhead">U币</td><td>U币: 12,345.5</td></tr>
<tr><td class="rowhead">加入日期</td><td>2024-05-06 12:00:00 (1 year ago)</td></tr>
<tr><td class="rowhead">当前做种</td><td>6 | 3.00 TiB</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table><tr><td rowspan="4">4.25</td></tr></table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "77" {
				t.Fatalf("userid = %q, want 77", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>6 | 3.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div><b>8</b> records</div>`))
			default:
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "77" || u.Name != "ub-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 6 || u.Uploads != 8 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestLuminanceFetchCGPeersUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchLuminanceProcessUserInfo(t, "cgpeers", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<div class="user-dropdown-section"><a href="/user/55">profile</a></div>
<a id="userDropdownTrigger">cg-user
menu</a>`))
		case "/user/55":
			_, _ = w.Write([]byte(`<span class="user_name">cg-user
stuff</span>
<span class="rank">Power User</span>
<ul class="stats">
<li>Joined: <span class="time" title="2024-05-06T12:00:00Z">joined</span></li>
<li>Last Seen: <span title="2024-05-07T12:00:00Z">last</span></li>
<li title="Uploaded: 9 torrents">Uploaded: 2.00 TiB [9 uploaded]</li>
<li>Downloaded: 512 GiB</li>
<li>Ratio: <span>4.0</span></li>
<li>Seeding: 7</li>
<li>Leeching: 1</li>
<li>Seeding Size: 3.00 TiB</li>
<li>Forum Posts: 44</li>
</ul>
<div id="bonusdiv"><h4>Credits: 12345.5</h4></div>
<div id="bonuslog">2024 | +102 credits | 24 hrs<br/></div>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "55" || u.Name != "cg-user" || u.Level != "Power User" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Posts != 44 {
		t.Fatalf("posts = %d, want 44", u.Posts)
	}
	if u.Ratio != 4 || u.Seeding != 7 || u.Leeching != 1 || u.Bonus != 12345.5 || u.BonusPerHour != 4.25 || u.Uploads != 9 {
		t.Fatalf("stats = ratio %g seeding %d leeching %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Leeching, u.Uploads, u.Bonus, u.BonusPerHour)
	}
}

func TestGenericHTMLFetchToTheGloryUserInfoFromMigratedProcess(t *testing.T) {
	u := fetchGenericProcessUserInfo(t, "totheglory", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=66">ttg-user</a>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "66" {
				t.Fatalf("id = %q, want 66", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">上传量</td><td>4.00 TiB</td></tr>
<tr><td class="rowhead">下载量</td><td>1.00 TiB</td></tr>
<tr><td class="rowhead">分享率</td><td>4.0</td></tr>
<tr><td class="rowhead">等级</td><td>GigaByte</td></tr>
<tr><td class="rowhead">头像</td><td><img src="https://img.example/ttg.png"></td></tr>
<tr><td class="rowhead">积分</td><td>55,555.5</td></tr>
<tr><td class="rowhead">注册日期</td><td>2024-05-06</td></tr>
</table>
<h1><img src="pic/flag/donor.gif" alt="Donor"></h1>
<div id="ka1"><table><tr><th>Name</th></tr><tr><td>U1</td></tr><tr><td>U2</td></tr></table></div>
<div id="ka2"><table>
<tr><th>Name</th><th>A</th><th>B</th><th>Size</th></tr>
<tr><td>A</td><td>x</td><td>x</td><td>1.00 GiB</td></tr>
<tr><td>B</td><td>x</td><td>x</td><td>2.00 GiB</td></tr>
</table></div>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table><tr><td class="rowhead">总计</td><td>6.75 points per hour</td></tr></table>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if u.ID != "66" || u.Name != "ttg-user" || u.Level != "GigaByte" || u.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 4*(1<<40) || u.Downloaded != 1*(1<<40) || u.SeedingSize != 3*(1<<30) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Seeding != 2 || u.Uploads != 2 || u.Bonus != 55555.5 || u.BonusPerHour != 6.75 {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour)
	}
	if u.Avatar != "https://img.example/ttg.png" || !u.IsDonor {
		t.Fatalf("profile = avatar %q is_donor %v", u.Avatar, u.IsDonor)
	}
}
