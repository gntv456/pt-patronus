// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #377 SiteChain.credential 把「解不开凭据」压成空串，于是 20 个调用方拿空凭据真的去请求站点。
//
// site_credential.go 里 9 处 `cookie, _ := c.sites.Cookie(s)`（同形状还有 APIKey /
// DecryptedPasskey）把 error 丢掉，返回值退化成 ""。
//
// 这个形状在**同一个包的另一个域**上已被点名并修过 ——
// chain/entertainment_capability.go:20 的注释原文：
//
//	// 从前 resolve() 写成 `cookie, _ := siteOp.Cookie(s)`：错误被丢掉、空凭据照发请求，
//	// 用户看到的是「游戏失败」「余额 0」，而这条路是要消耗游戏币的。
//	// Decrypt("") 归一成 ("", nil)，所以这里的非 nil 错误一定是真故障，
//	// 不会误伤没配凭据的站点。
//
// 那次只修了娱乐域 4 个出口。而 credential() 是全站通用凭据打包，调用方 20 处：
// 消息（5）、站点能力（3）、H&R（2）、签到（2）、种子（2）、站点信息（2）、
// 连接测试 / 搜索 / 快照 / 请求（各 1）。
//
// 后果与娱乐域同类、只是更宽：主密钥换过（或密文损坏）时，用户会看到
// 「消息拉不出来」「签到失败」「搜索无结果」，而真相是**本地解不开凭据** ——
// 这些全是站外行为，用户会以为站点挂了或自己被封。
//
// 判据用真 sqlite + 真 oper + 换一把密钥驱动（与 entertainment_creds_test.go 同法），
// 不 mock：那个文件里同一场景的注释说明 mock 掉 crypto 就没有意义了 ——
// 本洞要证明的正是「真解不开时会发生什么」。

import (
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// rotatedSiteChain 造一条「用 A 加密、用 B 解密」的链 —— 等价于主密钥换过。
func rotatedSiteChain(t *testing.T) (*SiteChain, *models.Site) {
	t.Helper()
	db := newChainDB(t)
	writer := oper.NewSiteOper(db, security.NewCrypto("old-key"))
	s, err := writer.Create(oper.SiteInput{
		Name: "馒头", Platform: "nexusphp", BaseURL: "https://x.test",
		Cookie: "uid=7; pass=secret", APIKey: "ak-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	return &SiteChain{sites: oper.NewSiteOper(db, security.NewCrypto("rotated-key"))}, s
}

func TestCredentialPropagatesUnreadableCookie(t *testing.T) {
	c, s := rotatedSiteChain(t)
	// nil def ⇒ cookie 分支（绝大多数站点走这条）。
	_, err := c.credentialWithErr(s, nil)
	if err == nil {
		t.Fatal("cookie 解不开时必须返回错误（旧形状：吞掉 error、返回空凭据，" +
			"20 个调用方拿空 cookie 真的去请求站点）")
	}
	if !errors.Is(err, ErrSiteCredentialUnreadable) {
		t.Errorf("该是可判别的 ErrSiteCredentialUnreadable（与娱乐域同一哨兵），got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "馒头") {
		t.Errorf("该点名站点（用户看到的是「消息拉不出来」，不知道该去哪个站重存凭据）：%q", msg)
	}
	if !strings.Contains(msg, "Cookie") {
		t.Errorf("该说清是 Cookie 这一项：%q", msg)
	}
}

// 反向划界：本来就没配 cookie（Decrypt("") → ("", nil)）不该被判成故障。
// 这条最要紧 —— 若误判，所有没配 cookie 的站点会整片说成「凭据读不出来」。
func TestCredentialNoCookieIsNotFailure(t *testing.T) {
	db := newChainDB(t)
	op := oper.NewSiteOper(db, security.NewCrypto("k"))
	s, err := op.Create(oper.SiteInput{Name: "空站", Platform: "nexusphp", BaseURL: "https://y.test"})
	if err != nil {
		t.Fatal(err)
	}
	c := &SiteChain{sites: op}
	got, err := c.credentialWithErr(s, nil)
	if err != nil {
		t.Fatalf("没配 cookie 不该报错（旧形状会把它误判成「解不开」，让正常站点全废）：%v", err)
	}
	if got != "" {
		t.Errorf("没配 cookie 时凭据串应为空，got %q", got)
	}
}

// 反向划界：密钥没换时照常拿到凭据（否则这次修复会把所有站点一起说成坏的）。
func TestCredentialReadableUnaffected(t *testing.T) {
	db := newChainDB(t)
	op := oper.NewSiteOper(db, security.NewCrypto("same-key"))
	s, err := op.Create(oper.SiteInput{
		Name: "好站", Platform: "nexusphp", BaseURL: "https://z.test",
		Cookie: "uid=7; pass=secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &SiteChain{sites: op}
	got, err := c.credentialWithErr(s, nil)
	if err != nil {
		t.Fatalf("密钥没换时不该报错：%v", err)
	}
	if got != "uid=7; pass=secret" {
		t.Errorf("该原样拿到 cookie，got %q", got)
	}
}

// schema=mteam 走 APIKey 分支：那一项解不开也要能判别。
func TestCredentialPropagatesUnreadableAPIKey(t *testing.T) {
	c, s := rotatedSiteChain(t)
	_, err := c.credentialWithErr(s, &site.SiteMetadata{Schema: "mteam"})
	if err == nil {
		t.Fatal("APIKey 解不开时必须返回错误（旧形状：return \"\"，调用方拿空 key 去请求）")
	}
	if !errors.Is(err, ErrSiteCredentialUnreadable) {
		t.Errorf("该是 ErrSiteCredentialUnreadable，got %v", err)
	}
	if !strings.Contains(err.Error(), "APIKey") {
		t.Errorf("该说清是哪一项凭据：%q", err.Error())
	}
}
