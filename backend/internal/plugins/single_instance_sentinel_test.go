// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"errors"
	"testing"
)

// TestSingleInstanceErrorUnwraps 钉住 #287 的前提：私有错误类型必须能被 errors.Is 认出来，
// 否则接口层那一档永远不会触发，用户又会拿到 400「请求参数有误」。
func TestSingleInstanceErrorUnwraps(t *testing.T) {
	err := error(errSingleInstance{"fluxtorrent"})
	if !errors.Is(err, ErrSingleInstance) {
		t.Fatalf("单实例错误没挂上哨兵：%v", err)
	}
	// 文案原句不许变（可能有别的用例/日志在按它匹配）。
	if got := err.Error(); got != "插件 fluxtorrent 为单实例插件，不支持分身" {
		t.Errorf("文案被改动：%q", got)
	}
	if !errors.Is(error(errMarketOper{}), ErrMarketOperUnavailable) {
		t.Error("市场运营层未装配这条得能被认出来")
	}
}
