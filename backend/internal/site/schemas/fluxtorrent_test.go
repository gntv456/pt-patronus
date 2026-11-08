package schemas

import (
	"regexp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// fluxTorrentServer 起一个按 FluxTorrent 兼容层口径应答的 mock 站。
// 返回服务地址与鉴权头记录（校验 Token 前缀 + 信封解包路径）。
func fluxTorrentServer(t *testing.T, items string, ptpp string) (*httptest.Server, *[]string) {
	t.Helper()
	var auths []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/compat/nexusphp/torrents.json", func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if r.URL.Query().Get("keyword") == "" && r.URL.Query().Encode() == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"page":1,"page_size":50,"has_more":false,"items":[` + items + `]}}`))
	})
	mux.HandleFunc("/api/v1/plugins/ptppUserInfo", func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":` + ptpp + `}`))
	})
	srv := httptest.NewServer(mux)
	return srv, &auths
}

func fluxTestClient(t *testing.T) *httpclient.Client {
	t.Helper()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatalf("httpclient: %v", err)
	}
	return hc
}

const fluxCredPkg = `{"token":"fxo_abc123","passkey":"0123456789abcdef0123456789abcdef"}`

func TestFluxTorrentSearch(t *testing.T) {
	items := `{"id":7,"name":"Demo 2026 1080p","small_descr":"中字","seeders":3,"leechers":1,"completed":9,
		"size":5368709120,"added":1759276800,"category_name":"电影","promotion":"free","free":true,"info_hash":"aa"},
		{"id":8,"name":"Half Show S01","small_descr":"","seeders":0,"leechers":0,"completed":2,
		"size":1073741824,"added":1759363200,"category_name":"剧集","promotion":"half","free":false,"info_hash":"bb"},
		{"id":9,"name":"X2 Free Pack","small_descr":"x2free","seeders":5,"leechers":2,"completed":4,
		"size":2147483648,"added":1759449600,"category_name":"音乐","promotion":"x2free","free":true,"info_hash":"cc"}`
	srv, auths := fluxTorrentServer(t, items, `{}`)
	defer srv.Close()

	f := NewFluxTorrent(fluxTestClient(t))
	ts, err := f.Search(context.Background(), &site.SiteMetadata{ID: "fluxtorrent", Schema: "fluxtorrent"},
		srv.URL, fluxCredPkg, "demo", "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 3 {
		t.Fatalf("torrents = %d, want 3", len(ts))
	}
	first := ts[0]
	if first.ID != "7" || first.Title != "Demo 2026 1080p" || first.SubTitle != "中字" {
		t.Fatalf("identity = %q %q %q", first.ID, first.Title, first.SubTitle)
	}
	if first.Seeders != 3 || first.Leechers != 1 || first.Completed != 9 {
		t.Fatalf("stats = %d/%d/%d", first.Seeders, first.Leechers, first.Completed)
	}
	if first.Size != "5.00 GiB" {
		t.Fatalf("size = %q, want 5.00 GiB", first.Size)
	}
	if !first.Free || first.DownloadVolumeFactor != 0 {
		t.Fatalf("free item: Free=%v dvf=%v", first.Free, first.DownloadVolumeFactor)
	}
	if !strings.Contains(first.DownloadURL, "download.php?id=7&passkey=0123456789abcdef0123456789abcdef") {
		t.Fatalf("download url = %q", first.DownloadURL)
	}
	if !strings.Contains(first.DetailURL, "/torrent/7") {
		t.Fatalf("detail url = %q", first.DetailURL)
	}
	half := ts[1]
	if half.DownloadVolumeFactor != 0.5 || half.UploadVolumeFactor != 0 {
		t.Fatalf("half factors = %v/%v", half.DownloadVolumeFactor, half.UploadVolumeFactor)
	}
	x2free := ts[2]
	if x2free.DownloadVolumeFactor != 0 || x2free.UploadVolumeFactor != 2 {
		t.Fatalf("x2free factors = %v/%v", x2free.DownloadVolumeFactor, x2free.UploadVolumeFactor)
	}
	if len(*auths) == 0 || (*auths)[0] != "Token fxo_abc123" {
		t.Fatalf("auth header = %v, want 'Token fxo_abc123' (not Bearer)", *auths)
	}
}

func TestFluxTorrentSearchWithoutPasskey(t *testing.T) {
	srv, _ := fluxTorrentServer(t, `{"id":7,"name":"Demo","size":1024,"seeders":1,"leechers":0,"completed":0,"added":1759276800,"promotion":"","free":false}`, `{}`)
	defer srv.Close()
	f := NewFluxTorrent(fluxTestClient(t))
	ts, err := f.Search(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"},
		srv.URL, `{"token":"fxo_x"}`, "", "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 1 || !strings.Contains(ts[0].DownloadURL, "download.php?id=7") {
		t.Fatalf("download url = %q", ts[0].DownloadURL)
	}
	if strings.Contains(ts[0].DownloadURL, "passkey") {
		t.Fatalf("unexpected passkey in url: %q", ts[0].DownloadURL)
	}
}

func TestFluxTorrentSearchAuthAndErrors(t *testing.T) {
	// 无 token → 明确报错
	f := NewFluxTorrent(fluxTestClient(t))
	if _, err := f.Search(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"},
		"http://example.invalid", "", "kw", "", 1); err == nil || !strings.Contains(err.Error(), "token required") {
		t.Fatalf("no-token err = %v", err)
	}

	// 401 → ErrNotLoggedIn
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := f.Search(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"},
		srv.URL, fluxCredPkg, "kw", "", 1); err != site.ErrNotLoggedIn {
		t.Fatalf("401 err = %v", err)
	}
	if _, err := f.FetchUserInfo(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"},
		srv.URL, fluxCredPkg); err != site.ErrNotLoggedIn {
		t.Fatalf("userinfo 401 err = %v", err)
	}

	// 信封业务错误 code!=0 → 带服务端 message
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1015,"message":"rate limited","data":null}`))
	}))
	defer srv2.Close()
	if _, err := f.Search(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"},
		srv2.URL, fluxCredPkg, "kw", "", 1); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("biz err = %v", err)
	}
}

func TestFluxTorrentUserInfoAndPasskey(t *testing.T) {
	ptpp := `{"id":42,"name":"root","bonus":1234.5,"uploaded":1099511627776,"downloaded":536870912000,
		"seeding":6,"leeching":1,"seedingSize":21474836480,"invites":2,"levelName":"管理员",
		"joinTime":1704067200,"messageCount":3,"passkey":"fedcba0987654321fedcba0987654321","isLogged":true}`
	srv, _ := fluxTorrentServer(t, `[]`, ptpp)
	defer srv.Close()

	f := NewFluxTorrent(fluxTestClient(t))
	u, err := f.FetchUserInfo(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"},
		srv.URL, fluxCredPkg)
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "root" || u.Level != "管理员" {
		t.Fatalf("identity = %q %q %q", u.ID, u.Name, u.Level)
	}
	if u.Uploaded != 1099511627776 || u.Downloaded != 536870912000 {
		t.Fatalf("traffic = %d/%d", u.Uploaded, u.Downloaded)
	}
	if u.Seeding != 6 || u.Leeching != 1 || u.SeedingSize != 21474836480 {
		t.Fatalf("seeding = %d/%d/%d", u.Seeding, u.Leeching, u.SeedingSize)
	}
	if u.Bonus != 1234.5 || u.JoinDate != "2024-01-01" {
		t.Fatalf("bonus/join = %v/%q", u.Bonus, u.JoinDate)
	}
	wantRatio := float64(1099511627776) / 536870912000
	if u.Ratio < wantRatio-0.001 || u.Ratio > wantRatio+0.001 {
		t.Fatalf("ratio = %v, want ~%v", u.Ratio, wantRatio)
	}
	// passkey 随本次返回值交付（无实例级中转，见 TestFluxTorrentPasskeyConcurrentIsolation）
	if u.DiscoveredPasskey != "fedcba0987654321fedcba0987654321" {
		t.Fatalf("discovered passkey = %q", u.DiscoveredPasskey)
	}
	// 不参与序列化：passkey 不应随 UserInfo 出现在任何 JSON 出口（API 响应/快照）
	if b, err := json.Marshal(u); err != nil || strings.Contains(string(b), "fedcba") {
		t.Fatalf("UserInfo JSON must not carry passkey: %s err=%v", string(b), err)
	}
}

// TestFluxTorrentPasskeyConcurrentIsolation 钉住并发串站回归：app_setup 全 app 只注册一个
// FluxTorrent 适配器实例，TestConnectionsStream 会并发对多站调 UserInfo。passkey 必须只随
// 本次调用返回值传递——一旦经实例字段中转，A 站会取到 B 站的 passkey（写错身份做种），
// 而 B 站自己拿到空值（下载链永远缺凭证）。
func TestFluxTorrentPasskeyConcurrentIsolation(t *testing.T) {
	mk := func(passkey string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"id":1,"name":"u","passkey":"` + passkey + `"}}`))
		}))
	}
	srvA := mk("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	defer srvA.Close()
	srvB := mk("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	defer srvB.Close()

	f := NewFluxTorrent(fluxTestClient(t)) // 同一个实例服务两站
	def := &site.SiteMetadata{Schema: "fluxtorrent"}

	var wg sync.WaitGroup
	var mu sync.Mutex
	got := map[string]string{}
	fetch := func(base, cred string) {
		defer wg.Done()
		u, err := f.FetchUserInfo(context.Background(), def, base, cred)
		if err != nil {
			t.Errorf("FetchUserInfo(%s): %v", base, err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		got[base] = u.DiscoveredPasskey
	}
	wg.Add(2)
	go fetch(srvA.URL, `{"token":"fxo_aaa"}`)
	go fetch(srvB.URL, `{"token":"fxo_bbb"}`)
	wg.Wait()

	if got[srvA.URL] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("A 站 passkey = %q（串站或丢失）", got[srvA.URL])
	}
	if got[srvB.URL] != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("B 站 passkey = %q（串站或丢失）", got[srvB.URL])
	}
}

func TestParseFluxTorrentCredential(t *testing.T) {
	if c := parseFluxTorrentCredential(fluxCredPkg); c.Token != "fxo_abc123" ||
		c.Passkey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("pkg cred = %+v", c)
	}
	if c := parseFluxTorrentCredential("  fxo_bare  "); c.Token != "fxo_bare" || c.Passkey != "" {
		t.Fatalf("bare cred = %+v", c)
	}
	if c := parseFluxTorrentCredential(""); c.Token != "" || c.Passkey != "" {
		t.Fatalf("empty cred = %+v", c)
	}
	// 常见别名键（api_key/apikey）与 rsskey 回退
	if c := parseFluxTorrentCredential(`{"api_key":"k1","rsskey":"r1"}`); c.Token != "k1" || c.Passkey != "r1" {
		t.Fatalf("alias cred = %+v", c)
	}
}

func TestFluxTorrentEnvelopeShape(t *testing.T) {
	// data 直接是数组（未来兼容层若去掉包一层 items）：宽松解析不 panic、返回空列表
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":[]}`))
	}))
	defer srv.Close()
	f := NewFluxTorrent(fluxTestClient(t))
	var env fluxEnvelope
	_ = json.Unmarshal([]byte(`{"code":0,"message":"ok","data":{"items":[]}}`), &env)
	if env.Code != 0 || len(env.Data) == 0 {
		t.Fatalf("envelope = %+v", env)
	}
	ts, err := f.Search(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"},
		srv.URL, fluxCredPkg, "kw", "", 1)
	if err != nil {
		t.Fatalf("Search empty-data: %v", err)
	}
	if len(ts) != 0 {
		t.Fatalf("torrents = %d, want 0", len(ts))
	}
}

// fluxCapServer 起一个 /open/me/* 三端点 mock 站（HR/做种/站内信）。
func fluxCapServer(t *testing.T, hr, seeding, messages string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/open/me/hr", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token fxo_abc123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"items":[` + hr + `],"count":1}}`))
	})
	mux.HandleFunc("/api/v1/open/me/seeding", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token fxo_abc123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"items":[` + seeding + `],"count":1}}`))
	})
	mux.HandleFunc("/api/v1/open/me/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token fxo_abc123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"items":[` + messages + `],"count":1}}`))
	})
	return httptest.NewServer(mux)
}

func TestFluxTorrentFetchHR(t *testing.T) {
	srv := fluxCapServer(t,
		`{"torrent_id":11,"name":"HR Torrent 2026","seeded_seconds":36000,"required_seconds":172800,
		 "shortfall_seconds":136800,"detected_at":1759276800,"resolved_at":null,"open":true}`,
		`{}`, `{}`)
	defer srv.Close()
	f := NewFluxTorrent(fluxTestClient(t))
	hrs, err := f.FetchHR(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"}, srv.URL, fluxCredPkg)
	if err != nil {
		t.Fatalf("FetchHR: %v", err)
	}
	if len(hrs) != 1 {
		t.Fatalf("hr items = %d, want 1", len(hrs))
	}
	h := hrs[0]
	if h.TorrentID != "11" || h.Title != "HR Torrent 2026" {
		t.Fatalf("identity = %q %q", h.TorrentID, h.Title)
	}
	if h.Status != "未达标" {
		t.Fatalf("status = %q", h.Status)
	}
	if !strings.Contains(h.SeedTime, "10时") || !strings.Contains(h.SeedTime, "2天") {
		t.Fatalf("seed time = %q", h.SeedTime)
	}
	if !strings.Contains(h.DeadlineLeft, "1天14时") {
		t.Fatalf("deadline left = %q", h.DeadlineLeft)
	}
	if !strings.Contains(h.DetailURL, "/torrent/11") {
		t.Fatalf("detail url = %q", h.DetailURL)
	}
}

func TestFluxTorrentFetchSeedingList(t *testing.T) {
	srv := fluxCapServer(t, `{}`,
		`{"torrent_id":1,"name":"Seed A","size":5368709120,"category_name":"电影"},
		 {"torrent_id":2,"name":"Seed B","size":1073741824}`,
		`{}`)
	defer srv.Close()
	f := NewFluxTorrent(fluxTestClient(t))
	items, err := f.FetchSeedingList(context.Background(), &site.SiteMetadata{Schema: "fluxtorrent"}, srv.URL, fluxCredPkg)
	if err != nil {
		t.Fatalf("FetchSeedingList: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("seeding items = %d, want 2", len(items))
	}
	if items[0].Title != "Seed A" || items[0].Size != 5368709120 {
		t.Fatalf("first = %+v", items[0])
	}
	if items[1].Size != 1073741824 {
		t.Fatalf("second size = %d", items[1].Size)
	}
}

var msgDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$`)

func TestFluxTorrentFetchMessages(t *testing.T) {
	srv := fluxCapServer(t, `{}`, `{}`,
		`{"id":9,"from":"系统","subject":"欢迎","body":"欢迎入驻","created_at":1704067200,"read_at":null,"unread":true}`)
	defer srv.Close()
	f := NewFluxTorrent(fluxTestClient(t))
	def := &site.SiteMetadata{Schema: "fluxtorrent"}
	msgs, err := f.FetchMessages(context.Background(), def, srv.URL, fluxCredPkg, site.MailboxInbox)
	if err != nil {
		t.Fatalf("FetchMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	m := msgs[0]
	if m.ID != 9 || m.Sender != "系统" || m.Title != "欢迎" || m.Content != "欢迎入驻" {
		t.Fatalf("message = %+v", m)
	}
	// 日期由 time.Unix 本地时区格式化（对齐站内展示口径）：断言形状而非绝对时刻，
	// 否则 UTC CI（00:00）与 +8 本地（08:00）各挂一边。
	if m.IsRead || !msgDateRe.MatchString(m.Date) {
		t.Fatalf("read/date = %v %q", m.IsRead, m.Date)
	}
	// 非收件箱信箱返回空（FluxTorrent 只有收件箱概念）
	other, err := f.FetchMessages(context.Background(), def, srv.URL, fluxCredPkg, site.MailboxSent)
	if err != nil || len(other) != 0 {
		t.Fatalf("sent mailbox = %v %d, want empty", err, len(other))
	}
	// 写操作 graceful：返回 Success=false 的结果而非报错（范式同 MTeam 只读）
	if rr, err := f.ReplyMessage(context.Background(), def, srv.URL, fluxCredPkg, "", ""); err != nil || rr.Success {
		t.Fatalf("reply = %v %v, want graceful not-success", rr, err)
	}
	if err := f.DeleteMessage(context.Background(), def, srv.URL, fluxCredPkg, ""); err == nil {
		t.Fatalf("delete should report unsupported")
	}
}

func TestFluxTorrentCapabilityAuth(t *testing.T) {
	// 401 → ErrNotLoggedIn（能力端点共用管道）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	f := NewFluxTorrent(fluxTestClient(t))
	def := &site.SiteMetadata{Schema: "fluxtorrent"}
	if _, err := f.FetchHR(context.Background(), def, srv.URL, fluxCredPkg); err != site.ErrNotLoggedIn {
		t.Fatalf("hr 401 = %v", err)
	}
	if _, err := f.FetchSeedingList(context.Background(), def, srv.URL, fluxCredPkg); err != site.ErrNotLoggedIn {
		t.Fatalf("seeding 401 = %v", err)
	}
	if _, err := f.FetchMessages(context.Background(), def, srv.URL, fluxCredPkg, site.MailboxInbox); err != site.ErrNotLoggedIn {
		t.Fatalf("messages 401 = %v", err)
	}
}

func TestFluxTorrentDefaultsEnableCapabilities(t *testing.T) {
	def := site.EnsureDefaults(&site.SiteMetadata{ID: "fluxtorrent", Schema: "fluxtorrent"})
	if !def.HR.Enabled {
		t.Fatalf("hr should default enabled for fluxtorrent")
	}
	if !def.Messages.Enabled {
		t.Fatalf("messages should default enabled for fluxtorrent")
	}
	// 幂等（TestEnsureDefaultsIdempotent 全局钉死，这里按 schema 复核）
	again := site.EnsureDefaults(def)
	if !again.HR.Enabled || !again.Messages.Enabled {
		t.Fatalf("defaults not idempotent")
	}
}
