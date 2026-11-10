// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #401 下载 .torrent 的两类失败（cookie 失效 / 拿到的是 HTML 不是种子）原文是英文，
// 而且全仓没有一处 errors.Is 在读这两个哨兵。
//
// 旧形状（chain/download.go 的 applyFetchedTorrent）：
//
//	default:
//		return fmt.Errorf("下载 .torrent 失败: %w", ferr)
//
// 这条错经 opResult → userErrText(=errtext.Text) 到界面。errtext.Text 从前不认这两个哨兵，
// 落到兜底 `return err.Error()` 原样返回英文，于是用户看到：
//
//	下载 .torrent 失败: site cookie invalid or expired (download redirected to login)
//
// Cookie 失效是下载失败里最常见的一种（站点把 download 重定向到登录页）。这句话既没点名
// 「哪个站的 Cookie」，也没给下一步（该去「站点」页重登），用户只会反复点推送。
//
// 修法：哨兵由本包 init 登记进 errtext（errtext 被 chain 依赖，反向 import 会成环；
// 判据仍留在 errtext 一处，文案仍是单一来源）。

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

func TestTorrentFetchFailuresAreTranslated(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"cookie 失效", ErrCookieInvalid, "Cookie"},
		{"拿到的是 HTML 不是种子", ErrNotTorrent, "种子文件"},
	} {
		// 复现真实出口的包装方式：applyFetchedTorrent 的 default 档。
		wrapped := fmt.Errorf("下载 .torrent 失败: %w", tc.err)
		got := errtext.Text(wrapped)

		if strings.Contains(got, "cookie invalid") || strings.Contains(got, "not a valid torrent") {
			t.Errorf("%s：英文原文仍漏到界面（errtext 没接上这个哨兵）：%q", tc.name, got)
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s：文案缺 %q，got %q", tc.name, tc.want, got)
		}
	}
}

func TestTorrentCookieInvalidTellsNextStep(t *testing.T) {
	got := errtext.Text(ErrCookieInvalid)
	// 英文版没有任何下一步；中文版必须点名去哪儿。
	if !strings.Contains(got, "站点") {
		t.Errorf("cookie 失效必须说清去「站点」页，got %q", got)
	}
	if !strings.Contains(got, "重新") {
		t.Errorf("必须说清要重新获取（用户反复点推送不会有帮助），got %q", got)
	}
}

// 反向对照：登记表不许吞掉别的错误。errtext 的兜底（认不出的原样返回）与内置档都必须仍在。
func TestRegistryDoesNotSwallowOtherErrors(t *testing.T) {
	other := errors.New("某个认不出的下游错误")
	if got := errtext.Text(other); got != other.Error() {
		t.Errorf("登记口不许改变其它错误的兜底行为，got %q", got)
	}
	if got := errtext.Text(httpclient.ErrBlocked); !strings.Contains(got, "ALLOW_PRIVATE_SITES") {
		t.Errorf("内置 SSRF 档必须仍生效，got %q", got)
	}
}
