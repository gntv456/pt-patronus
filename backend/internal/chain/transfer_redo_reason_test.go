// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/errtext"
)

// #323 「重做转存」找不到种子时，从前只回一句英文，而且不分处境：
// 所有下载器都关着（压根没拿到列表）与「拿到了列表但里面没这颗种子」
// 是完全不同的下一步 —— 前者该去开客户端/查地址，后者才该怀疑种子被清理。

func TestRedoNotFoundReasonSeparatesCauses(t *testing.T) {
	t.Run("一个下载器都没配", func(t *testing.T) {
		got := redoNotFoundReason(0, false, nil, "ABC").Error()
		if !strings.Contains(got, "还没有配置下载器") {
			t.Errorf("该说清是没配下载器，实际 %q", got)
		}
	})

	t.Run("配了但全都问不到列表", func(t *testing.T) {
		err := redoNotFoundReason(2, false, downloader.ErrCannotConnect, "ABC")
		got := err.Error()
		if !strings.Contains(got, "没有任何下载器") {
			t.Errorf("要说「问不到列表」，实际 %q", got)
		}
		if strings.Contains(got, "找不到") {
			t.Errorf("没拿到列表就不许断言种子「找不到」（张冠李戴）：%q", got)
		}
		if !errors.Is(err, downloader.ErrCannotConnect) {
			t.Errorf("必须用 %%w 保住哨兵，界面才能拿到下载器方向：%q", got)
		}
		if txt := errtext.Text(err); !strings.Contains(txt, "下载器") {
			t.Errorf("经 errtext 后界面该看到下载器方向，实际 %q", txt)
		}
	})

	t.Run("问到了列表但里面没有", func(t *testing.T) {
		got := redoNotFoundReason(2, true, nil, "ABC").Error()
		if !strings.Contains(got, "已在所有下载器中找不到") || !strings.Contains(got, "ABC") {
			t.Errorf("这时候才该说种子没了，并带上 hash：实际 %q", got)
		}
	})

	t.Run("问不到列表且没有底层错误", func(t *testing.T) {
		got := redoNotFoundReason(1, false, nil, "ABC").Error()
		if !strings.Contains(got, "没有任何下载器") || strings.Contains(got, "%!") {
			t.Errorf("lastErr 为 nil 时不许出现 %%w 残迹，实际 %q", got)
		}
	})
}
