package schemas

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// mteamSearchHandler 返回一条 FREE 种子（seeders 为字符串、size 为数字，验证容错解析）。
func mteamSearchHandler(t *testing.T, gotKey *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/torrent/search" {
			t.Errorf("search path = %s", r.URL.Path)
		}
		*gotKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"success","data":{"total":1,"data":[
			{"id":"12345","name":"Test Movie 2024","smallDescr":"1080p Blu-ray","size":16106127360,
			 "status":{"seeders":"5","leechers":"2","discount":"FREE","toppingLevel":0}}
		]}}`))
	})
}

func TestMTeamSearch(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(mteamSearchHandler(t, &gotKey))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	def := &site.SiteMetadata{Schema: "mteam", ExtraParam: map[string]string{"mode": "normal"}}

	ts, err := m.Search(context.Background(), def, srv.URL, "my-api-key", "test", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "my-api-key" {
		t.Errorf("x-api-key header = %q, want my-api-key", gotKey)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(ts))
	}
	tor := ts[0]
	if tor.Title != "Test Movie 2024" || tor.ID != "12345" {
		t.Errorf("unexpected torrent: %+v", tor)
	}
	if tor.Seeders != 5 || tor.Leechers != 2 {
		t.Errorf("seeders/leechers = %d/%d (tolerant string parse)", tor.Seeders, tor.Leechers)
	}
	if !tor.Free {
		t.Errorf("expected Free=true (discount=FREE)")
	}
	if tor.Size != "15.00 GiB" {
		t.Errorf("size = %q, want 15.00 GiB", tor.Size)
	}
}

// TestMTeamGenDlToken：multipart {id} + x-api-key POST → data 字符串即签名下载 URL。
func TestMTeamGenDlToken(t *testing.T) {
	var gotKey, gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/torrent/genDlToken" {
			t.Errorf("path = %s, want /api/torrent/genDlToken", r.URL.Path)
		}
		gotKey = r.Header.Get("x-api-key")
		_ = r.ParseMultipartForm(1 << 20)
		gotID = r.FormValue("id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"success","data":"https://api.m-team.cc/api/torrent/download?id=12345&sig=ABC"}`))
	}))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	url, err := m.GenDlToken(context.Background(), srv.URL, "KEY", "12345")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "KEY" {
		t.Errorf("x-api-key = %q, want KEY", gotKey)
	}
	if gotID != "12345" {
		t.Errorf("multipart id = %q, want 12345", gotID)
	}
	const want = "https://api.m-team.cc/api/torrent/download?id=12345&sig=ABC"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
}

// TestMTeamGenDlTokenError：code!=0 报错；code=0 但 data 空也报错。
func TestMTeamGenDlTokenError(t *testing.T) {
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"401","message":"api key invalid","data":""}`))
	}))
	defer srv.Close()
	if _, err := m.GenDlToken(context.Background(), srv.URL, "BAD", "1"); err == nil {
		t.Fatal("want error for code!=0")
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"ok","data":""}`))
	}))
	defer srv2.Close()
	if _, err := m.GenDlToken(context.Background(), srv2.URL, "KEY", "1"); err == nil {
		t.Fatal("want error for empty data")
	}
}

func TestMTeamFetchUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/member/profile":
			_, _ = w.Write([]byte(`{"code":"0","data":{"username":"tester","role":"3","createdDate":"2023-01-02 03:04:05",
				"memberCount":{"uploaded":"10737418240","downloaded":"1073741824","bonus":"1234.5","shareRate":"1.5"}}}`))
		case "/api/tracker/mybonus":
			_, _ = w.Write([]byte(`{"code":"0","data":{"formulaParams":{"finalBs":"12.5"}}}`))
		case "/api/tracker/myPeerStatistics":
			_, _ = w.Write([]byte(`{"code":"0","data":{"seederSize":"16106127360","seederCount":"7","leecherCount":"2","uploadCount":"3"}}`))
		default:
			t.Errorf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	ui, err := m.FetchUserInfo(context.Background(), &site.SiteMetadata{}, srv.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	if ui.Name != "tester" || ui.Level != "Elite User" {
		t.Errorf("profile: %+v", ui)
	}
	if ui.JoinDate != "2023-01-02 03:04:05" {
		t.Errorf("joinDate = %q", ui.JoinDate)
	}
	if ui.Uploaded != 10737418240 || ui.Ratio != 1.5 || ui.Bonus != 1234.5 {
		t.Errorf("numeric fields: uploaded=%d ratio=%g bonus=%g", ui.Uploaded, ui.Ratio, ui.Bonus)
	}
	if ui.BonusPerHour != 12.5 {
		t.Errorf("bonusPerHour = %g, want 12.5", ui.BonusPerHour)
	}
	if ui.Seeding != 7 || ui.Leeching != 2 || ui.SeedingSize != 16106127360 || ui.Uploads != 3 {
		t.Errorf("peer stats: seeding=%d leeching=%d size=%d uploads=%d", ui.Seeding, ui.Leeching, ui.SeedingSize, ui.Uploads)
	}
}

// role 缺失时 Level 不得为 "<nil>"（fmt.Sprint(nil) 的坑），应为空串。
func TestMTeamFetchUserInfoMissingRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","data":{"username":"tester","memberCount":{}}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	ui, err := m.FetchUserInfo(context.Background(), &site.SiteMetadata{}, srv.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	if ui.Level != "" {
		t.Errorf("missing role: Level = %q, want empty (not \"<nil>\")", ui.Level)
	}
}

func TestMTeamAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	_, err := m.FetchUserInfo(context.Background(), &site.SiteMetadata{}, srv.URL, "bad-key")
	if !errors.Is(err, site.ErrNotLoggedIn) {
		t.Errorf("err = %v, want ErrNotLoggedIn", err)
	}
}

// code!="0" 的业务错误应返回带消息的错误（而非 ErrNotLoggedIn）。
func TestMTeamBusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"1001","message":"参数错误","data":null}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	_, err := m.Search(context.Background(), &site.SiteMetadata{}, srv.URL, "k", "x", "", 1)
	if err == nil || errors.Is(err, site.ErrNotLoggedIn) {
		t.Errorf("err = %v, want non-auth business error", err)
	}
}

// TestMTeamSearchCategoryMode 验证 category → mode 映射：movie→movie、tv→tvshow、
// 空→extra_param.mode 兜底（默认 normal）。
func TestMTeamSearchCategoryMode(t *testing.T) {
	cases := []struct {
		category string
		wantMode string
	}{
		{"movie", "movie"},
		{"tv", "tvshow"},
		{"", "normal"},      // 综合：兜底 extra_param.mode=normal
		{"music", "normal"}, // 无独立 mode → 综合
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.category, func(t *testing.T) {
			var gotMode string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if m, ok := body["mode"].(string); ok {
					gotMode = m
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":"0","message":"ok","data":{"total":0,"data":[]}}`))
			}))
			defer srv.Close()
			hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
			m := NewMTeam(hc)
			def := &site.SiteMetadata{Schema: "mteam", ExtraParam: map[string]string{"mode": "normal"}}
			if _, err := m.Search(context.Background(), def, srv.URL, "key", "kw", tc.category, 1); err != nil {
				t.Fatal(err)
			}
			if gotMode != tc.wantMode {
				t.Errorf("category=%q mode=%q want %q", tc.category, gotMode, tc.wantMode)
			}
		})
	}
}
