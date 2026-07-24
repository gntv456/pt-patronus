package chain

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/cookiecloud"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// memStore 是 ccStore 的内存实现（无 DB 单测用）。
type memStore struct{ m map[string]string }

func (s *memStore) Get(k string) (string, error)  { return s.m[k], nil }
func (s *memStore) Set(k, v string) error {
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[k] = v
	return nil
}

// ccEncrypt 用标准 CookieCloud 协议加密明文，供 mock server 构造响应。
func ccEncrypt(t *testing.T, plain, uuid, password string) string {
	t.Helper()
	key := cookiecloud.DeriveKey(uuid, password)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append([]byte(plain), strings.Repeat(string(rune(pad)), pad)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out)
}

// mockCCServer 起一个返回标准加密 payload 的假 CookieCloud 服务器，返回其 URL。
func mockCCServer(t *testing.T, uuid, password, plain string) string {
	t.Helper()
	enc := ccEncrypt(t, plain, uuid, password)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET /get/{uuid} 拉；POST /update 推（双向同步需要两者）。
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"action":"done"}`))
			return
		}
		_, _ = w.Write([]byte(`{"encrypted":"` + enc + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestCookieForSite(t *testing.T) {
	payload := map[string][]cookiecloud.CookieEntry{
		"demo.pt":      {{Name: "sess", Value: "abc"}, {Name: "x", Value: "1"}},
		"sub.demo.pt":  {{Name: "sub", Value: "v"}},
		"other.io":     {{Name: "o", Value: "z"}},
	}
	got := cookieForSite(payload, "https://www.demo.pt/")
	for _, want := range []string{"sess=abc", "x=1", "sub=v"} {
		if !strings.Contains(got, want) {
			t.Errorf("cookieForSite = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "o=z") {
		t.Errorf("cookieForSite should not include other domain: %q", got)
	}
}

func TestFindDefinitionByHost(t *testing.T) {
	defs := []*site.SiteMetadata{
		{ID: "demo", Name: "Demo", Schema: "nexusphp", URLs: []string{"https://www.demopt.com/"}},
		{ID: "other", Name: "Other", Schema: "nexusphp", URLs: []string{"https://other.io/"}},
	}
	if d := findDefinitionByHost("www.demopt.com", defs); d == nil || d.ID != "demo" {
		t.Errorf("expected demo, got %+v", d)
	}
	if d := findDefinitionByHost("demopt.com", defs); d == nil || d.ID != "demo" {
		t.Errorf("expected demo by apex, got %+v", d)
	}
	if d := findDefinitionByHost("unknown.xyz", defs); d != nil {
		t.Errorf("expected nil for unknown, got %+v", d)
	}
}

func TestSaveGetSettings_PasswordEncryptAndKeep(t *testing.T) {
	crypto := security.NewCrypto("seed")
	ch := NewCookieCloudChain(&memStore{}, crypto, nil, nil, nil, nil)

	// 首次保存：密码加密入库
	s, err := ch.SaveSettings(CookieCloudSettingsInput{
		Enabled: true, Server: "https://cc.example.com", UUID: "u1", Password: "secret", IntervalMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.PasswordEnc == "" || s.PasswordEnc == "secret" {
		t.Fatalf("password not encrypted: %q", s.PasswordEnc)
	}
	// 密文应能解回明文
	dec, err := crypto.Decrypt(s.PasswordEnc)
	if err != nil || dec != "secret" {
		t.Errorf("decrypt password = %q err=%v, want secret", dec, err)
	}

	got := ch.GetSettings()
	if !got.Enabled || got.Server != "https://cc.example.com" || got.UUID != "u1" || got.IntervalMin != 60 {
		t.Errorf("GetSettings mismatch: %+v", got)
	}
	if !ch.passwordConfigured() {
		t.Error("passwordConfigured should be true")
	}

	// 再次保存：密码留空 = 保留旧密码
	s2, err := ch.SaveSettings(CookieCloudSettingsInput{
		Enabled: false, Server: "https://cc2.example.com", UUID: "u2", Password: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s2.PasswordEnc != s.PasswordEnc {
		t.Errorf("empty password should keep old enc, got %q want %q", s2.PasswordEnc, s.PasswordEnc)
	}
}

// TestDoSync_UpdateFailedSiteAndAddNew siteChain=nil → 全判失效 → 更新已有站 + 新增匹配定义的站。
func TestDoSync_UpdateFailedSiteAndAddNew(t *testing.T) {
	uuid, password := "u-123", "pass"
	plain := `{"cookie_data":{"demo.pt":[{"name":"sess","value":"new","domain":"demo.pt","path":"/"}],"other.io":[{"name":"t","value":"v","domain":"other.io","path":"/"}]}}`
	srvURL := mockCCServer(t, uuid, password, plain)

	crypto := security.NewCrypto("seed")
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, crypto)
	demo, _ := siteOp.Create(oper.SiteInput{Name: "Demo", Platform: "nexusphp", BaseURL: "https://demo.pt", Cookie: "sess=old", IsActive: true})

	reg := site.NewRegistry()
	reg.AddDefinition(&site.SiteMetadata{ID: "other", Name: "Other", Schema: "nexusphp", URLs: []string{"https://other.io/"}})

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true}) // httptest=127.0.0.1，绕过 SSRF

	ch := NewCookieCloudChain(&memStore{}, crypto, siteOp, reg, nil, hc) // siteChain=nil → isConnected 恒 false
	s := CookieCloudSettings{Server: srvURL, UUID: uuid}
	s.PasswordEnc, _ = crypto.Encrypt(password)

	report, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 1 {
		t.Errorf("Updated = %d, want 1", report.Updated)
	}
	if report.Added != 1 {
		t.Errorf("Added = %d, want 1 (other.io 应自动新增)", report.Added)
	}
	// UpdateCookie 改 DB 但不刷新 demo 指针的 CookieEnc，需重新 Get 再读。
	demo2, _ := siteOp.Get(demo.ID)
	c, _ := siteOp.Cookie(demo2)
	if !strings.Contains(c, "sess=new") {
		t.Errorf("demo cookie = %q, want sess=new (失效站应被覆盖)", c)
	}
}

// TestDoSync_SkipConnectedSite 已登录站（TestConnection 成功）应跳过不覆盖。
func TestDoSync_SkipConnectedSite(t *testing.T) {
	uuid, password := "u-123", "pass"
	plain := `{"cookie_data":{"demo.pt":[{"name":"sess","value":"new","domain":"demo.pt","path":"/"}]}}`
	srvURL := mockCCServer(t, uuid, password, plain)

	crypto := security.NewCrypto("seed")
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, crypto)
	demo, _ := siteOp.Create(oper.SiteInput{Name: "Demo", Platform: "nexusphp", BaseURL: "https://demo.pt", Cookie: "sess=keepold", IsActive: true})

	reg := site.NewRegistry()
	mm := module.New()
	mm.Register(&fakeAdapter{schema: "nexusphp", ui: &site.UserInfo{Level: "User"}}) // FetchUserInfo 成功 → TestConnection 成功
	siteChain := NewSiteChain(NewBase(mm, event.New()), siteOp, reg)

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ch := NewCookieCloudChain(&memStore{}, crypto, siteOp, reg, siteChain, hc)
	s := CookieCloudSettings{Server: srvURL, UUID: uuid}
	s.PasswordEnc, _ = crypto.Encrypt(password)

	report, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 0 {
		t.Errorf("Updated = %d, want 0 (已登录站不应覆盖)", report.Updated)
	}
	c, _ := siteOp.Cookie(demo)
	if !strings.Contains(c, "keepold") {
		t.Errorf("demo cookie = %q, want keepold (有效 cookie 不应被覆盖)", c)
	}
}

// TestMaybeSync_RespectsInterval 软间隔：刚同步过（LastSyncUnix=now）→ 不应再触发 doSync。
func TestMaybeSync_RespectsInterval(t *testing.T) {
	crypto := security.NewCrypto("seed")
	srvURL := mockCCServer(t, "u", "p", `{"cookie_data":{}}`)
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ch := NewCookieCloudChain(&memStore{}, crypto, nil, nil, nil, hc)

	// 配置完整 + LastSyncUnix 设为当前 → 间隔内，MaybeSync 应直接返回不拉取
	now := time.Now().Unix()
	store := &memStore{m: map[string]string{}}
	enc, _ := crypto.Encrypt("p")
	settings := CookieCloudSettings{Enabled: true, Server: srvURL, UUID: "u", PasswordEnc: enc, IntervalMin: 60, LastSyncUnix: now}
	ch.store = store
	_ = ch.saveSettings(settings)
	ch.MaybeSync(context.Background()) // 不应 panic / 不应改变 last_sync

	got := ch.GetSettings()
	if got.LastSyncUnix != now {
		t.Errorf("MaybeSync should be skipped within interval; LastSyncUnix changed to %d, want %d", got.LastSyncUnix, now)
	}
}


// TestDoSync_AlwaysModeUpdatesConnected update_mode=always 时，即使站点仍登录也覆盖 cookie。
func TestDoSync_AlwaysModeUpdatesConnected(t *testing.T) {
	uuid, password := "u-123", "pass"
	plain := `{"cookie_data":{"demo.pt":[{"name":"sess","value":"from-cloud","domain":"demo.pt","path":"/"}]}}`
	srvURL := mockCCServer(t, uuid, password, plain)

	crypto := security.NewCrypto("seed")
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, crypto)
	demo, _ := siteOp.Create(oper.SiteInput{Name: "Demo", Platform: "nexusphp", BaseURL: "https://demo.pt", Cookie: "sess=keepold; local_only=1", IsActive: true})

	reg := site.NewRegistry()
	mm := module.New()
	mm.Register(&fakeAdapter{schema: "nexusphp", ui: &site.UserInfo{Level: "User"}})
	siteChain := NewSiteChain(NewBase(mm, event.New()), siteOp, reg)

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ch := NewCookieCloudChain(&memStore{}, crypto, siteOp, reg, siteChain, hc)
	s := CookieCloudSettings{Server: srvURL, UUID: uuid, UpdateMode: "always"}
	s.PasswordEnc, _ = crypto.Encrypt(password)

	report, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 1 {
		t.Errorf("Updated = %d, want 1 (always mode should update connected site)", report.Updated)
	}
	demo2, _ := siteOp.Get(demo.ID)
	c, _ := siteOp.Cookie(demo2)
	if !strings.Contains(c, "sess=from-cloud") {
		t.Errorf("cookie = %q, want sess=from-cloud", c)
	}
	if !strings.Contains(c, "local_only=1") {
		t.Errorf("cookie = %q, want preserve local_only=1 via merge", c)
	}
}

func TestCookieForSiteDedupesBySpecificHost(t *testing.T) {
	payload := map[string][]cookiecloud.CookieEntry{
		"demo.pt":     {{Name: "sess", Value: "root"}},
		"www.demo.pt": {{Name: "sess", Value: "www"}, {Name: "x", Value: "1"}},
	}
	got := cookieForSite(payload, "https://www.demo.pt/")
	if !strings.Contains(got, "sess=www") {
		t.Fatalf("want more-specific host to win, got %q", got)
	}
	if strings.Contains(got, "sess=root") {
		t.Fatalf("root value should be overwritten, got %q", got)
	}
	if !strings.Contains(got, "x=1") {
		t.Fatalf("missing x=1 in %q", got)
	}
}


// TestDoSync_PushMergesLocalCookies direction=push 时把本地 cookie 推到云端（mock 接受 POST）。
func TestDoSync_PushMergesLocalCookies(t *testing.T) {
	uuid, password := "u-push", "pass"
	// 云端已有 unrelated host，推送后应保留（merge）——本测试主要断言 push 不报错且 Pushed>=1。
	plain := `{"cookie_data":{"other.io":[{"name":"o","value":"1","domain":"other.io","path":"/"}]}}`
	srvURL := mockCCServer(t, uuid, password, plain)

	crypto := security.NewCrypto("seed")
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, crypto)
	_, _ = siteOp.Create(oper.SiteInput{Name: "Demo", Platform: "nexusphp", BaseURL: "https://demo.pt", Cookie: "sess=local1; uid=9", IsActive: true})

	reg := site.NewRegistry()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ch := NewCookieCloudChain(&memStore{}, crypto, siteOp, reg, nil, hc)
	s := CookieCloudSettings{Server: srvURL, UUID: uuid, Direction: "push"}
	s.PasswordEnc, _ = crypto.Encrypt(password)

	report, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if report.Pushed != 1 {
		t.Errorf("Pushed=%d, want 1", report.Pushed)
	}
	if report.Updated != 0 {
		t.Errorf("push-only should not update local, Updated=%d", report.Updated)
	}
}

func TestNormalizeCookieCloudDirection(t *testing.T) {
	if normalizeCookieCloudDirection("") != "both" {
		t.Fatal("empty -> both")
	}
	if normalizeCookieCloudDirection("PULL") != "pull" {
		t.Fatal("PULL -> pull")
	}
	if normalizeCookieCloudDirection("nope") != "" {
		t.Fatal("invalid -> empty")
	}
}
