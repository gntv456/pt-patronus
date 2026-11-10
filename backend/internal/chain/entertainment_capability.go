// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"errors"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/entertainment"
)

// ErrGameUnsupported 是「这个游戏的适配层没有这个能力」的前缀载体：
// 它后面接的是给人看的具体一句（该游戏不支持连抽/购卡/合成…）。
// 这类原因从前被接口层压成 400「请求参数有误，请检查后重试」，
// 而用户没有任何输入可检查——他要的是「换一个支持这个功能的站点/游戏」，
// 花钱操作被这样压掉还会让人以为要重试（可能重复消耗游戏币）。
//
// 单独立文件：chain/entertainment.go 已在行数基线上（只许缩不许涨）。
var ErrGameUnsupported = errors.New("该游戏不支持")

// ErrSiteCredentialUnreadable 是「这个站点配过凭据、但现在解不开」（主密钥换过 / 密文损坏）。
// 从前 resolve() 写成 `cookie, _ := siteOp.Cookie(s)`：错误被丢掉、空凭据照发请求，
// 用户看到的是「游戏失败」「余额 0」，而这条路是要消耗游戏币的。
// Decrypt("") 归一成 ("", nil)，所以这里的非 nil 错误一定是真故障，不会误伤没配凭据的站点。
var ErrSiteCredentialUnreadable = errors.New("站点凭据读不出来")

// siteCredUnreadable 把「解不开哪一项凭据」说成一句可照做的话：点名站点、说清下一步去哪儿，
// 并钉死「没有向站点发出请求」——这一句同时排除了「扣了一半游戏币」的猜疑。
// Cookie 与密码两个出口共用，抄第二遍必然漂移。
func siteCredUnreadable(siteName, field string, err error) error {
	return fmt.Errorf("%w：站点「%s」的%s无法解密（主密钥可能已更换）——请到「站点」里重新保存该站点凭据；本次未向站点发请求，游戏币未消耗（%v）",
		ErrSiteCredentialUnreadable, siteName, field, err)
}

// resolve 取站点+游戏+凭据+baseURL（娱乐域所有读/写出口的公共前置）。
func (c *EntertainmentChain) resolve(siteID uint, gameID string) (entertainment.LotteryGame, string, entertainment.Creds, error) {
	s, err := c.siteOp.Get(siteID)
	if err != nil {
		return nil, "", entertainment.Creds{}, err
	}
	g, ok := c.reg.Game(s.BaseURL, gameID)
	if !ok {
		return nil, "", entertainment.Creds{}, ErrGameNotFound
	}
	cookie, cerr := c.siteOp.Cookie(s)
	if cerr != nil {
		return nil, "", entertainment.Creds{}, siteCredUnreadable(s.Name, "Cookie", cerr)
	}
	pw, perr := c.siteOp.Password(s) // 财神等 API 登录用；其余站点为空不影响
	if perr != nil {
		return nil, "", entertainment.Creds{}, siteCredUnreadable(s.Name, "密码", perr)
	}
	return g, s.BaseURL, entertainment.Creds{Cookie: cookie, Username: s.Username, Password: pw}, nil
}
