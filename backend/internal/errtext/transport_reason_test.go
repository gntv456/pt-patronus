// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package errtext

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// #321 传输层的两种常见失败此前没有中文映射，用户拿到的是 Go/os 原文：
//   - 保存的地址漏了协议：Get "example.com/feed": unsupported protocol scheme ""
//     （#318 之后写入面拦住了新数据，但 cron 与存量行仍会走到这里）
//   - 连不上/域名解析不了：dial tcp …: connectex: No connection could be made…
//
// 这两类都有明确的下一步动作，该翻成人话；站点适配器/下载器自己返回的原文照旧透传。

func TestTransportReasonsAreHumanReadable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
		deny string
	}{
		{
			name: "缺协议前缀",
			err:  fmt.Errorf(`抓取 RSS 失败: Get "example.com/feed": unsupported protocol scheme ""`),
			want: "http://",
			deny: "unsupported protocol scheme",
		},
		{
			name: "连不上（连接被拒）",
			err:  fmt.Errorf(`抓取 RSS 失败: Get "http://127.0.0.1:9/feed": dial tcp: connectex: No connection could be made because the target machine actively refused it.`),
			want: "连不上",
			deny: "connectex",
		},
		{
			name: "域名解析不了",
			err:  fmt.Errorf(`站点请求失败: Get "https://did-not-exist.invalid/rss": dial tcp: lookup did-not-exist.invalid: no such host`),
			want: "连不上",
			deny: "no such host",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Text(c.err)
			if !strings.Contains(got, c.want) {
				t.Errorf("要翻成可操作的中文（含 %q），实际 %q", c.want, got)
			}
			if strings.Contains(got, c.deny) {
				t.Errorf("Go/os 原文不许留在界面文案里（含 %q）：%q", c.deny, got)
			}
		})
	}
}

// 正控：已定性的哨兵与「适配器自己写的原文」都不许被抢走。
func TestTransportReasonsDoNotStealExistingText(t *testing.T) {
	if got := Text(fmt.Errorf("包一层: %w", httpclient.ErrBlocked)); got != SSRFHint {
		t.Errorf("ErrBlocked 必须仍走 SSRFHint（它带 ALLOW_PRIVATE_SITES 这条下一步），实际 %q", got)
	}
	own := errors.New("站点返回：用户名或密码错误")
	if got := Text(own); got != own.Error() {
		t.Errorf("适配器自己写的中文原因必须原样透传，实际 %q", got)
	}
	if Text(nil) != "" {
		t.Errorf("nil 必须回空串")
	}
}
