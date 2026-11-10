// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// 分身写面的三档必须分开（#72/#73）：
//
//	分身不存在 → 404「这个分身实例不存在，可能已被删除：请刷新插件详情」
//	市场运营层没装配 → DependencyMissing（去开配置，重试无用）
//	其它写失败 → SaveFailed
//
// 都不许落到「请求参数有误」那句上——它会让用户去改那个改不了的插件 id。
func TestInstanceErrClassifiesThreeSituations(t *testing.T) {
	miss := fmt.Errorf("wrap: %w", oper.ErrPluginInstanceMissing)
	if got := instanceErr(miss); got == nil || got.Code != errcode.PluginInstanceNotFound.Code {
		t.Errorf("分身缺失应给 PluginInstanceNotFound，实得 %+v", got)
	}
	if got := instanceErr(plugins.ErrMarketOperUnavailable); got == nil || got.Code != errcode.DependencyMissing.Code {
		t.Errorf("市场层没装配应给 DependencyMissing，实得 %+v", got)
	}
	if got := instanceErr(errors.New("disk full")); got == nil || got.Code != errcode.SaveFailed.Code {
		t.Errorf("其它写失败仍应归 SaveFailed，实得 %+v", got)
	}
	// 正控：字面相似但不经哨兵包装的错误，不许被误判成「分身不存在」。
	if got := instanceErr(errors.New("plugin instance missing-ish")); got != nil && got.Code == errcode.PluginInstanceNotFound.Code {
		t.Error("判据过宽：相似字面也被当成了分身缺失")
	}
}
