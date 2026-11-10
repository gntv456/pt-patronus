// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/hxpt/ptpatronus/internal/cookiecloud"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// newAuthChain 用指定主密钥构造 builtin 链（同一 db/store 可换密钥重建，模拟主密钥轮换）。
func newAuthChain(t *testing.T, crypto *security.Crypto, db gorm.DB, store ccStore) *CookieCloudChain {
	t.Helper()
	siteOp := oper.NewSiteOper(&db, crypto)
	reg := site.NewRegistry()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	return NewCookieCloudChain(store, crypto, siteOp, reg, nil, hc)
}

func pushBody(t *testing.T, uuid, encrypted string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]string{"uuid": uuid, "encrypted": encrypted})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestServerUpdateRequiresPasswordProof 回归：/cookiecloud/update 是公网无鉴权端点，
// 旧实现只比 uuid —— 知道 uuid 的人即可覆盖云端 blob，并且 ServerUpdate 会立刻 triggerSync
// 把这份外部输入写进本地站点 cookie。必须用「能否用本机口令解密」当写入门禁。
func TestServerUpdateRequiresPasswordProof(t *testing.T) {
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("cc-auth-seed")
	store := &memStore{}
	ch := newAuthChain(t, crypto, *db, store)
	s, err := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	password, err := ch.DecryptedPassword(s)
	if err != nil {
		t.Fatal(err)
	}

	// 正控：持同一口令的合法扩展推送要能落库
	enc, err := cookiecloud.EncryptPayload(map[string][]cookiecloud.CookieEntry{
		"demo.pt": {{Name: "sess", Value: "fresh", Domain: "demo.pt", Path: "/"}},
	}, s.UUID, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ch.ServerUpdate(pushBody(t, s.UUID, enc)); err != nil {
		t.Fatalf("合法推送应被接受：%v", err)
	}

	// 反例：uuid 对、密文是垃圾 → 拒，且不覆盖已存的好数据
	garbage := base64.StdEncoding.EncodeToString([]byte("not-a-cookiecloud-ciphertext-at-all"))
	if _, err := ch.ServerUpdate(pushBody(t, s.UUID, garbage)); err == nil {
		t.Fatal("知道 uuid 但解不开的推送必须被拒（否则任何拿到 uuid 的人都能污染 cookie 源）")
	}
	if env, ok := ch.ServerEnvelope(s.UUID); !ok || env.Encrypted != enc {
		t.Errorf("被拒的推送不得改动已存 envelope，got ok=%v env=%+v", ok, env)
	}

	// 反例：用别的口令加密（攻击者自有一套 uuid+password）→ 同样拒
	other, oerr := cookiecloud.EncryptPayload(map[string][]cookiecloud.CookieEntry{
		"evil.pt": {{Name: "sess", Value: "attacker", Domain: "evil.pt", Path: "/"}},
	}, s.UUID, "attacker-chosen-password")
	if oerr != nil {
		t.Fatal(oerr)
	}
	if _, err := ch.ServerUpdate(pushBody(t, s.UUID, other)); err == nil {
		t.Error("用其它口令加密的推送必须被拒")
	}
}

// TestServerUpdateFailsClosedWhenPasswordUnreadable 主密钥换过后口令解不开：
// 宁可拒绝一切推送（用户重置口令即恢复），也不能退回「只认 uuid」的裸奔状态。
func TestServerUpdateFailsClosedWhenPasswordUnreadable(t *testing.T) {
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("cc-rotate-seed")
	store := &memStore{}
	ch := newAuthChain(t, crypto, *db, store)
	s, err := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	password, _ := ch.DecryptedPassword(s)
	enc, err := cookiecloud.EncryptPayload(map[string][]cookiecloud.CookieEntry{
		"demo.pt": {{Name: "sess", Value: "x", Domain: "demo.pt", Path: "/"}},
	}, s.UUID, password)
	if err != nil {
		t.Fatal(err)
	}

	rotated := newAuthChain(t, security.NewCrypto("cc-rotated-seed"), *db, store)
	if _, err := rotated.ServerUpdate(pushBody(t, s.UUID, enc)); err == nil {
		t.Fatal("口令不可用时必须 fail-closed 拒绝写入")
	}
	if _, ok := rotated.ServerEnvelope(s.UUID); ok {
		t.Error("被拒的推送不得在云端留下数据")
	}
}

// TestServerUpdateAcceptsPayloadWeCannotParse 回归 #159：门禁证明的是「对端知道口令」，
// 不是「本仓的 cookie 解析器认得这份负载」。扩展把 cookie_data 写成 {name: value} 映射时
// DecryptPayload 会报错——拿它把关就等于把合法客户端 404 掉，用户在扩展侧只看到
// 「CookieCloud 404」，根本想不到是自己配得好好的推送被自家服务端拒了。
func TestServerUpdateAcceptsPayloadWeCannotParse(t *testing.T) {
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("cc-shape-seed")
	store := &memStore{}
	ch := newAuthChain(t, crypto, *db, store)
	s, err := ch.SaveSettings(CookieCloudSettingsInput{Enabled: true, Mode: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	password, err := ch.DecryptedPassword(s)
	if err != nil {
		t.Fatal(err)
	}

	// 映射式负载（扩展旧写法/自建客户端）：口令正确，但本仓解析不了。
	mapJSON := `{"cookie_data":{"demo.pt":{"sess":"abc"}},"local_storage_data":{}}`
	enc, err := cookiecloud.Encrypt([]byte(mapJSON), cookiecloud.DeriveKey(s.UUID, password))
	if err != nil {
		t.Fatal(err)
	}
	if _, perr := cookiecloud.DecryptPayload(enc, s.UUID, password); perr == nil {
		t.Fatal("用例前提失效：映射式负载已可解析，这份反例不再承重，需换一种解析不了的形状")
	}
	if _, err := ch.ServerUpdate(pushBody(t, s.UUID, enc)); err != nil {
		t.Fatalf("口令正确、只是形状陌生的推送必须落库：%v", err)
	}
	if env, ok := ch.ServerEnvelope(s.UUID); !ok || env.Encrypted != enc {
		t.Errorf("推送未落库，ok=%v env=%+v", ok, env)
	}

	// 反例：同样陌生的形状，但口令不对 → 仍然拒（放宽形状不能顺手放宽口令）。
	other, err := cookiecloud.Encrypt([]byte(mapJSON), cookiecloud.DeriveKey(s.UUID, "not-the-server-password"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ch.ServerUpdate(pushBody(t, s.UUID, other)); err == nil {
		t.Error("口令不对的推送必须被拒")
	}
}
