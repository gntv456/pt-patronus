// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #383 passkey 侧的凭据读取仍在吞错 —— #377 把 Cookie/APIKey/Password 都接上了可判别的
// 错误出口，却漏了 passkey 这一列。
//
// db/oper/site.go 的形状：
//
//	func (o *SiteOper) Passkey(s *models.Site) (string, error) {
//	    if s == nil || s.PasskeyEnc == "" { return "", nil }   // 没配 = 正常
//	    return o.c.Decrypt(s.PasskeyEnc)                          // 换密钥/密文损坏 = 真故障
//	}
//
//	func (o *SiteOper) DecryptedPasskey(s *models.Site) string {
//	    pk, _ := o.Passkey(s)     // ← 错误丢掉
//	    return pk
//	}
//
// 前者分得清清楚楚，后者把两者压成同一个 ""。而 credentialWithErr 的五个 schema
// （f3nix / fsm / rousi / fluxtorrent / avistaz）全都经由 DecryptedPasskey 取 passkey；
// site_torrent.go / keepseed_resolve.go / site_connection.go 也各有一处。
//
// 后果与 #377 同形但更难认：换了主密钥后，用户在 f3nix 站看到「搜索无结果」、
// 在 fluxTorrent 站看到「下载链接拿不到」、在 avistaz 站看到「资料页空白」——
// 全被误读成站外行为或站点挂了，没有一条说「本地解不开 passkey」。而 Cookie 那列会
// 大声报出来（#377 的改动），同一份站点数据里两列态度相反。
//
// 判据用真 sqlite + 真 oper + 换一把密钥驱动（与 #377 那份同法），不 mock：
// 本洞要证明的正是「真解不开时会发生什么」，mock 掉 crypto 就没有意义。

import (
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// rotatedPasskeySite 造一个「用 A 加密、用 B 解密」且**配了 passkey** 的站点。
// 与 #377 那份 rotatedSiteChain 同法（真 sqlite + 真 oper + 真轮换），只多写一个 Passkey。
func rotatedPasskeySite(t *testing.T) (*SiteChain, *models.Site) {
	t.Helper()
	db := newChainDB(t)
	writer := oper.NewSiteOper(db, security.NewCrypto("old-key"))
	s, err := writer.Create(oper.SiteInput{
		Name: "馒头", Platform: "nexusphp", BaseURL: "https://x.test",
		Cookie: "uid=7; pass=secret", APIKey: "ak-123", Passkey: "REAL-PASSKEY",
	})
	if err != nil {
		t.Fatal(err)
	}
	return &SiteChain{sites: oper.NewSiteOper(db, security.NewCrypto("rotated-key"))}, s
}

// TestPasskeyDecryptFailureIsNotSilent 换密钥后 passkey 必须报错，不许退化成「没配」。
func TestPasskeyDecryptFailureIsNotSilent(t *testing.T) {
	c, s := rotatedPasskeySite(t)

	pk, err := c.sites.Passkey(s)
	if err == nil {
		t.Fatalf("换了密钥后 Passkey 竟没报错，返回 %q（解密失败被当成了空值）", pk)
	}
	if pk != "" {
		t.Errorf("解密失败时不该返回任何 passkey，实际 %q", pk)
	}
}

// 反向对照：压根没配 passkey 是正常状态，不许误报成故障。
// 这条最要紧 —— 误判会让所有没填 passkey 的站点整片说成「凭据读不出来」。
func TestPasskeyAbsentIsNotFailure(t *testing.T) {
	db := newChainDB(t)
	o := oper.NewSiteOper(db, security.NewCrypto("same-key"))
	blank := &models.Site{Name: "无 passkey 的站", PasskeyEnc: ""}
	pk, err := o.Passkey(blank)
	if err != nil {
		t.Fatalf("没配 passkey 是正常状态，不该报错: %v", err)
	}
	if pk != "" {
		t.Errorf("没配 passkey 时应返回空串，实际 %q", pk)
	}
}

// TestCredentialWithErrReportsPasskeyFailure 真正扎人的那一格：**只配了 passkey** 的站点。
//
// #383 之前我按「配齐 cookie+apiKey+passkey」写判据，结果全绿 —— 因为那样 Cookie/APIKey
// 会替我们报错（#377 的功劳），把 passkey 这列的漏洞盖住了。真实用户里「只填了 passkey」
// 恰恰是最常见的最小配置（FluxTorrent/Rousi 就是这个用法：搜索要 token，下载要 passkey）。
// 那种站点上五条分支全都返回 err=nil + 空凭据 ⇒ 拿空凭据真的去请求站点，
// 用户看到的是「搜索无结果 / 下载链接拿不到」，与 #377 修掉的那批症状一字不差。
func TestCredentialWithErrReportsPasskeyFailure(t *testing.T) {
	// 只写 Passkey，Cookie 与 APIKey 一律留空 —— 走最小配置那条真实路径。
	for _, schema := range []string{"f3nix", "fsm", "rousi", "fluxtorrent", "avistaz"} {
		t.Run(schema, func(t *testing.T) {
			db := newChainDB(t)
			w := oper.NewSiteOper(db, security.NewCrypto("old-key"))
			s, err := w.Create(oper.SiteInput{
				Name: "只配passkey的站", Platform: "nexusphp", BaseURL: "https://x.test",
				Passkey: "REAL-PASSKEY",
			})
			if err != nil {
				t.Fatal(err)
			}
			c := &SiteChain{sites: oper.NewSiteOper(db, security.NewCrypto("rotated-key"))}

			got, cerr := c.credentialWithErr(s, metaWithSchema(schema))
			if cerr == nil {
				t.Fatalf("%s：只配了 passkey 且它解不开，却返回 err=nil + 凭据 %q —— "+
					"空凭据会照发请求，用户看到的是站外行为（「搜索无结果」），"+
					"与 #377 修掉的那批症状一字不差", schema, got)
			}
			if !errors.Is(cerr, ErrSiteCredentialUnreadable) {
				t.Errorf("%s：该是可判别的 ErrSiteCredentialUnreadable，got %v", schema, cerr)
			}
			if !strings.Contains(cerr.Error(), s.Name) {
				t.Errorf("%s：原因里必须带出站点名，got %q", schema, cerr.Error())
			}
		})
	}
}

// TestPasskeyUnreadableNotSilentlyDegraded 直接钉住吞错的那个包装：
// DecryptedPasskey 把「解不开」压成 ""，与「没配」同形。
func TestPasskeyUnreadableNotSilentlyDegraded(t *testing.T) {
	c, s := rotatedPasskeySite(t)

	if got := c.sites.DecryptedPasskey(s); got != "" {
		t.Errorf("解密失败却返回了 passkey %q —— 这就是 #377 要消灭的形状（错误被丢掉、退化成空值）", got)
	}
	// 反向断言：Passkey 本身（带 error 的那个）必须报错。
	if _, err := c.sites.Passkey(s); err == nil {
		t.Error("Passkey 竟然没报错 —— 若这一条也绿，说明整条链的判据已经失效（crypto 没真正轮换）")
	}
}

// metaWithSchema 造一个只有 Schema 字段的元数据（credentialWithErr 只看它）。
func metaWithSchema(schema string) *site.SiteMetadata {
	return &site.SiteMetadata{Schema: schema}
}
