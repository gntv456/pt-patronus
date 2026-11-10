// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/cookiecloud"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// newBuiltinChain 构造 builtin 模式测试链（siteChain=nil → 全判失效 → 便于断言更新）。
func newBuiltinChain(t *testing.T) (*CookieCloudChain, *oper.SiteOper, *security.Crypto) {
	t.Helper()
	crypto := security.NewCrypto("seed")
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, crypto)
	reg := site.NewRegistry()
	reg.AddDefinition(&site.SiteMetadata{ID: "other", Name: "Other", Schema: "nexusphp", URLs: []string{"https://other.io/"}})
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	return NewCookieCloudChain(&memStore{}, crypto, siteOp, reg, nil, hc), siteOp, crypto
}

// TestSaveSettings_BuiltinAutoGen builtin 首次保存自动生成 uuid+密码；regenerate 重生成。
func TestSaveSettings_BuiltinAutoGen(t *testing.T) {
	ch, _, _ := newBuiltinChain(t)

	s, err := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != CookieCloudModeBuiltin {
		t.Fatalf("mode=%q", s.Mode)
	}
	if len(s.UUID) != 22 {
		t.Errorf("uuid auto-gen len=%d want 22 (%q)", len(s.UUID), s.UUID)
	}
	if s.PasswordEnc == "" {
		t.Error("password should be auto-generated")
	}
	if plain, _ := ch.DecryptedPassword(s); len(plain) != 22 {
		t.Errorf("password len=%d want 22", len(plain))
	}

	// regenerate：uuid 与密码都换新
	s2, err := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin", Regenerate: true})
	if err != nil {
		t.Fatal(err)
	}
	if s2.UUID == s.UUID {
		t.Error("regenerate should rotate uuid")
	}

	// external 模式不自动生成（uuid 清空沿用提交值；已有密码保留）
	s3, err := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "external"})
	if err != nil {
		t.Fatal(err)
	}
	if s3.UUID != "" {
		t.Errorf("external should not auto-gen uuid: %+v", s3)
	}
	if s3.PasswordEnc != s2.PasswordEnc {
		t.Errorf("switching to external should keep existing password enc")
	}
}

// TestBuiltinServerUpdateAndSync 全链路：扩展推密文 → ServerUpdate 存储 → doSync(builtin)
// 直读 store 更新本地站（不走 HTTP）。
func TestBuiltinServerUpdateAndSync(t *testing.T) {
	ch, siteOp, _ := newBuiltinChain(t)

	s, err := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	password, err := ch.DecryptedPassword(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := siteOp.Create(oper.SiteInput{Name: "Demo", Platform: "nexusphp", BaseURL: "https://demo.pt", Cookie: "sess=old", IsActive: true}); err != nil {
		t.Fatal(err)
	}

	// 模拟官方扩展推送：cookie_data 密文（fixed-IV 格式）
	enc, err := cookiecloud.EncryptPayload(map[string][]cookiecloud.CookieEntry{
		"demo.pt": {{Name: "sess", Value: "fresh", Domain: "demo.pt", Path: "/"}},
	}, s.UUID, password)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"uuid": s.UUID, "encrypted": enc})
	if _, err := ch.ServerUpdate(body); err != nil {
		t.Fatal(err)
	}

	// builtin doSync：不配 Server 字段也能拉（直读 store）
	report, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 1 {
		t.Fatalf("Updated=%d want 1 (report=%+v)", report.Updated, report)
	}
	sites, _ := siteOp.ListActive()
	if c, _ := siteOp.Cookie(&sites[0]); !strings.Contains(c, "sess=fresh") {
		t.Errorf("cookie=%q want sess=fresh", c)
	}
}

// TestServerEnvelope_Get ServerEnvelope 读 + miss；uuid 不匹配的 update 拒绝。
func TestServerEnvelope_Get(t *testing.T) {
	ch, _, _ := newBuiltinChain(t)
	s, _ := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin"})
	if _, ok := ch.ServerEnvelope("not-exist"); ok {
		t.Error("unknown uuid should miss")
	}
	enc, _ := cookiecloud.EncryptPayload(map[string][]cookiecloud.CookieEntry{
		"demo.pt": {{Name: "a", Value: "1"}},
	}, s.UUID, "pw")
	ch.serverStore().Put(s.UUID, cookiecloud.ServerEnvelope{Encrypted: enc})
	if _, ok := ch.ServerEnvelope(s.UUID); !ok {
		t.Error("stored uuid should hit")
	}

	// uuid 不匹配 → 拒绝（防探测他人 uuid 数据）
	other, _ := json.Marshal(map[string]string{"uuid": "intruder", "encrypted": enc})
	if _, err := ch.ServerUpdate(other); err == nil {
		t.Error("mismatched uuid should be rejected")
	}
}

// TestBuiltinEmptyStorePull store 无数据时 pull 不算错误（浏览器还没推过）。
func TestBuiltinEmptyStorePull(t *testing.T) {
	ch, _, _ := newBuiltinChain(t)
	s, _ := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin", Direction: "pull"})
	_, err := ch.doSync(context.Background(), s)
	if err != nil {
		t.Fatalf("empty builtin store pull should not error, got %v", err)
	}
}
