// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"os"
	"sync"
)

// siteIndexEnabled 控制 MatchByURL 是否走编译期索引快速路径（阶段 4）。
// PTP_SITE_FASTPATH=0 可秒级回退到全量遍历语义。
var indexFlagOnce sync.Once
var indexFlag bool

func siteIndexEnabled() bool {
	indexFlagOnce.Do(func() { indexFlag = os.Getenv("PTP_SITE_FASTPATH") != "0" })
	return indexFlag
}
