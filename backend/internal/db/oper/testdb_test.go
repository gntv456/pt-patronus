// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openIsolatedDB 给每条用例一个只属于它的内存库。
//
// 从前三个测试文件都直接写 `file::memory:?cache=shared`：共享缓存的匿名内存库在**同一进程内
// 是同一个库**，于是 `-count>1` 的第二轮、以及同包别家用例留下的行会串进来——实测
// TestUpsertConnectivityAtomicSQL 第二轮读到 success=4（want 1）、
// TestSiteCreateConcurrentSameDomain 第二轮 ok=0 dup=8（want 1/7）。
// 那种红与实现无关，是闸门自己在说谎，所以按「清理要显式放进被断言那一段」的口径修在这里。
//
// 命名库 + cache=shared 仍然让同一库跨连接可见（并发建同域那条用例正需要多连接共享），
// 只是不同用例（以及同一用例的不同次数）各用各的名字。
func openIsolatedDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := "file:" + name + "_" + strconv.FormatInt(time.Now().UnixNano(), 16) +
		"?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("take sqlite handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
