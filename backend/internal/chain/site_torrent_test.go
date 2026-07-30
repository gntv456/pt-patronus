package chain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// validTorrentFixture 是满足 isTorrentBytes sniff（首 'd' 末 'e' 含 info/announce）的最小 .torrent 字节，
// 仅供响应分类单测；不要求 bencode 语义合法（qb 真实解析由端到端测试覆盖）。
var validTorrentFixture = []byte("d8:announce28:http://tracker.example.test4:infod6:lengthi100e4:name8:testfile12:piece lengthi100e6:pieces20:00000000000000000000ee")

// setupChainWithSite 建一个指向 baseURL 的 SiteChain（hc 允许内网，供 httptest 用）+ 站点。
func setupChainWithSite(t *testing.T, baseURL, cookie, passkey, platform string) (*SiteChain, uint) {
	t.Helper()
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, err := siteOp.Create(oper.SiteInput{Name: "t", Platform: platform, BaseURL: baseURL, Cookie: cookie, Passkey: passkey})
	if err != nil {
		t.Fatal(err)
	}
	mm := module.New()
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true}) // httptest=127.0.0.1，绕过 SSRF
	ch.SetHTTPClient(hc)
	return ch, s.ID
}

// TestFetchTorrentFileOK 200 + 合法 .torrent → 返回字节，且请求带站点 cookie。
func TestFetchTorrentFileOK(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(validTorrentFixture)
	}))
	defer srv.Close()
	ch, sid := setupChainWithSite(t, srv.URL, "c_secure_uid=8; c_secure_pass=token", "", "nexusphp")

	content, _, err := ch.FetchTorrentFileByID(context.Background(), sid, srv.URL+"/download.php?id=1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(content) != string(validTorrentFixture) {
		t.Errorf("content mismatch")
	}
	if !strings.Contains(gotCookie, "c_secure_uid") {
		t.Errorf("request missing site cookie, got %q", gotCookie)
	}
}

// TestFetchTorrentFilePasskeyAppend 站点配 passkey 且 URL 不含 → 实际请求 URL 补 passkey=。
func TestFetchTorrentFilePasskeyAppend(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(validTorrentFixture)
	}))
	defer srv.Close()
	ch, sid := setupChainWithSite(t, srv.URL, "c_secure_uid=8", "pk-secret-123", "nexusphp")

	if _, _, err := ch.FetchTorrentFileByID(context.Background(), sid, srv.URL+"/download.php?id=9"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(gotQuery, "passkey=pk-secret-123") {
		t.Errorf("expected passkey appended, got query %q", gotQuery)
	}
}

// TestFetchTorrentFileLoginRedirect cookie 失效：download 302→login，FollowRedirect 跟到登录 HTML → ErrCookieInvalid。
func TestFetchTorrentFileLoginRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/download"):
			http.Redirect(w, r, "/login.php?returnto=download", http.StatusFound)
		case r.URL.Path == "/login.php":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body>please login <a href="takelogin.php">login</a></body></html>`))
		}
	}))
	defer srv.Close()
	ch, sid := setupChainWithSite(t, srv.URL, "c_secure_uid=8", "", "nexusphp")

	_, _, err := ch.FetchTorrentFileByID(context.Background(), sid, srv.URL+"/download.php?id=1")
	if !errors.Is(err, ErrCookieInvalid) {
		t.Fatalf("want ErrCookieInvalid, got %v", err)
	}
}

// TestFetchTorrentFileConfirmPage NexusPHP 首次下载确认页：GET 返 form → POST 确认 → 返 .torrent。
func TestFetchTorrentFileConfirmPage(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/download"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><form action="/download.php" method="post"><input type="hidden" name="formhash" value="hash9"/><input type="hidden" name="id" value="42"/></form></html>`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/download"):
			gotMethod = "posted"
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write(validTorrentFixture)
		}
	}))
	defer srv.Close()
	ch, sid := setupChainWithSite(t, srv.URL, "c_secure_uid=8", "", "nexusphp")

	content, _, err := ch.FetchTorrentFileByID(context.Background(), sid, srv.URL+"/download.php?id=42")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotMethod != "posted" {
		t.Errorf("expected confirm POST to occur, got %q", gotMethod)
	}
	if string(content) != string(validTorrentFixture) {
		t.Errorf("content mismatch after confirm")
	}
}

// TestFetchTorrentFileNotTorrent 200 但返 HTML（非 login/非确认页）→ ErrNotTorrent，防把 HTML 当种子。
func TestFetchTorrentFileNotTorrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>some random page about cats</body></html>`))
	}))
	defer srv.Close()
	ch, sid := setupChainWithSite(t, srv.URL, "c_secure_uid=8", "", "nexusphp")

	_, _, err := ch.FetchTorrentFileByID(context.Background(), sid, srv.URL+"/download.php?id=1")
	if !errors.Is(err, ErrNotTorrent) {
		t.Fatalf("want ErrNotTorrent, got %v", err)
	}
}

// TestFetchTorrentFileDelegated magnet / mteam / 无凭据 → ErrDelegatedToDownloader（沿用旧 URL 路径）。
func TestFetchTorrentFileDelegated(t *testing.T) {
	ch, sid := setupChainWithSite(t, "https://example.com", "c_secure_uid=8", "", "nexusphp")
	// magnet
	if _, _, err := ch.FetchTorrentFileByID(context.Background(), sid, "magnet:?xt=urn:btih:abc"); !errors.Is(err, ErrDelegatedToDownloader) {
		t.Errorf("magnet: want ErrDelegatedToDownloader, got %v", err)
	}
	// mteam（URL 自带 token，委托）
	chMT, sidMT := setupChainWithSite(t, "https://example.com", "", "", "mteam")
	if _, _, err := chMT.FetchTorrentFileByID(context.Background(), sidMT, "https://example.com/download/abc/token"); !errors.Is(err, ErrDelegatedToDownloader) {
		t.Errorf("mteam: want ErrDelegatedToDownloader, got %v", err)
	}
	// 无凭据（cookie+passkey 均空）→ 委托
	chNC, sidNC := setupChainWithSite(t, "https://example.com", "", "", "nexusphp")
	if _, _, err := chNC.FetchTorrentFileByID(context.Background(), sidNC, "https://example.com/download.php?id=1"); !errors.Is(err, ErrDelegatedToDownloader) {
		t.Errorf("no-cred: want ErrDelegatedToDownloader, got %v", err)
	}
}

// —— applyFetchedTorrent 拦截逻辑（stub torrentFetcher，无需 fake Downloader）——

type stubFetcher struct {
	content  []byte
	fileName string
	err      error
	called   bool
}

func (s *stubFetcher) FetchTorrentFileByID(_ context.Context, _ uint, _ string) ([]byte, string, error) {
	s.called = true
	return s.content, s.fileName, s.err
}

func TestApplyFetchedTorrentSuccess(t *testing.T) {
	tf := &stubFetcher{content: validTorrentFixture, fileName: "42.torrent"}
	opts := downloader.AddOptions{URL: "https://x.test/download.php?id=42", SiteID: 7}
	if err := applyFetchedTorrent(context.Background(), tf, &opts); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !tf.called {
		t.Error("FetchTorrentFileByID not called")
	}
	if opts.URL != "" { // ⠉ 关键：必须清空 URL，否则 qb methods.go:43 仍走 URL 路径
		t.Errorf("URL must be cleared, got %q", opts.URL)
	}
	if string(opts.FileContent) != string(validTorrentFixture) {
		t.Error("FileContent not filled")
	}
	if opts.FileName != "42.torrent" {
		t.Errorf("FileName = %q", opts.FileName)
	}
}

func TestApplyFetchedTorrentDelegated(t *testing.T) {
	tf := &stubFetcher{err: ErrDelegatedToDownloader}
	opts := downloader.AddOptions{URL: "https://x.test/download.php?id=42", SiteID: 7}
	if err := applyFetchedTorrent(context.Background(), tf, &opts); err != nil {
		t.Fatalf("delegated should not error, got %v", err)
	}
	if opts.URL == "" || len(opts.FileContent) > 0 {
		t.Error("delegated path must keep URL untouched and FileContent empty")
	}
}

func TestApplyFetchedTorrentFailure(t *testing.T) {
	tf := &stubFetcher{err: ErrCookieInvalid}
	opts := downloader.AddOptions{URL: "https://x.test/download.php?id=42", SiteID: 7}
	err := applyFetchedTorrent(context.Background(), tf, &opts)
	if err == nil {
		t.Fatal("expected error propagation, not silent success")
	}
}

func TestApplyFetchedTorrentSkipped(t *testing.T) {
	// SiteID=0 / magnet / FileContent 已有 → 不调 tf
	tf := &stubFetcher{content: validTorrentFixture}
	for name, opts := range map[string]downloader.AddOptions{
		"no-siteid": {URL: "https://x.test/dl?id=1"},
		"magnet":    {URL: "magnet:?xt=urn:btih:aa", SiteID: 7},
		"has-file":  {URL: "https://x.test/dl?id=1", SiteID: 7, FileContent: []byte("x")},
	} {
		o := opts
		if err := applyFetchedTorrent(context.Background(), tf, &o); err != nil {
			t.Errorf("%s: unexpected err %v", name, err)
		}
		if tf.called {
			t.Errorf("%s: tf should not be called", name)
		}
	}
}
