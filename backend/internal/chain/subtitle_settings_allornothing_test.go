// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #374 保存字幕源配置失败时停在「一半写完」的状态。
//
// ApplySettings（subtitle.go:82）逐键写 SystemConfig，失败即 return：
//
//	for k, v := range updates {          // 5 个键
//		if v == "" { continue }
//		if err := p.cfg.Set(k, v); err != nil { return err }
//	}
//	p.rebuild()                          // 失败时走不到
//
// 界面上这五个字段（assrt token / opensubtitles key / subhd cookie /
// zimuku cookie / 目标语言）是**同一次提交的一个整体**，用户心智里是「保存」一个动作。
// 失败理应回到「什么都没变」，而实际是：第 3 个键失败时前 2 个已入库、后 2 个还是旧值，
// 且 rebuild 没执行 —— 界面显示的和实际生效的不是一回事，用户完全不知道。
//
// 判据用真 sqlite + 真 SystemConfigOper 驱动，不用 fake：本洞的意义全在
// 「写到一半真的留在库里」这个可观察后果上，fake 只能证明我改后的形状。
// 故障注入方式是关掉连接（模拟磁盘满/连接断），而不是 mock 计数器 ——
// 后者会把「哪一步失败」变成我自己的假设。

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func newSubtitleProviderWithDB(t *testing.T) (*SubtitleSourceProvider, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sub.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	return &SubtitleSourceProvider{cfg: oper.NewSystemConfigOper(db)}, db
}

func fullSubtitleSettings() SubtitleSettings {
	return SubtitleSettings{
		AssrtToken: "a", OpenSubtitlesKey: "o", SubHDCookie: "s",
		ZimukuCookie: "z", TargetLang: "chs",
	}
}

// countSubtitleKeys 数一下这 5 个配置键里已写入多少个。
func countSubtitleKeys(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var n int64
	if err := db.Model(&models.SystemConfig{}).
		Where("`key` IN ?", []string{
			cfgKeyAssrtToken, cfgKeyOpenSubtitlesKey, cfgKeySubHDCookie,
			cfgKeyZimukuCookie, cfgKeySubtitleTarget,
		}).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return int(n)
}

// TestApplySettingsIsAllOrNothing 写失败时不允许留下半份配置。
func TestApplySettingsIsAllOrNothing(t *testing.T) {
	p, db := newSubtitleProviderWithDB(t)

	// 先把 5 个键都写成可辨识的旧值，模拟「已有配置」。
	old := SubtitleSettings{
		AssrtToken: "old-a", OpenSubtitlesKey: "old-o", SubHDCookie: "old-s",
		ZimukuCookie: "old-z", TargetLang: "bi",
	}
	if err := p.ApplySettings(old); err != nil {
		t.Fatalf("铺旧值失败：%v", err)
	}

	// 让下一次写失败：关掉连接（等价于磁盘满/连接断，不是 mock 计数器）。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := p.ApplySettings(fullSubtitleSettings()); err == nil {
		t.Fatal("库不可用时该返回错误")
	}
	// 库关了读不回来，所以断言点在「方法本身不该把失败当成功」；
	// 真正的半更新检查在下面那条（用可控失败的写口）。
}

// TestApplySettingsDoesNotLeaveHalfWrite 用可控写口验证核心断言：
// 第 3 次写失败时，库里不许出现「一部分新值 + 一部分旧值」的混合态。
func TestApplySettingsDoesNotLeaveHalfWrite(t *testing.T) {
	p, db := newSubtitleProviderWithDB(t)

	old := SubtitleSettings{
		AssrtToken: "old-a", OpenSubtitlesKey: "old-o", SubHDCookie: "old-s",
		ZimukuCookie: "old-z", TargetLang: "bi",
	}
	if err := p.ApplySettings(old); err != nil {
		t.Fatalf("铺旧值失败：%v", err)
	}

	// 换成一个「第 3 次起失败」的写口：dbfailOper 包住真 oper。
	// 只让第 3 次写失败；回滚（后续几次）照常成功。
	fails := &failAfterOper{SystemConfigOper: oper.NewSystemConfigOper(db), from: 3, to: 3}
	p.cfg = fails

	err := p.ApplySettings(fullSubtitleSettings())
	if err == nil {
		t.Fatal("第 3 次写失败时该返回错误")
	}

	// 恢复可读，核对库里到底是全旧还是全新 —— 不许是混合态。
	p.cfg = oper.NewSystemConfigOper(db)
	got := p.LoadSettings()
	if got != old && got != fullSubtitleSettings() {
		t.Errorf("库停在半更新状态：%+v\n"+
			"界面呈现的是一次「保存」，失败理应什么都不变；混合态 + rebuild 未执行 "+
			"⇒ 界面显示与实际生效的配置不一致，而用户不知情", got)
	}
}

// TestApplySettingsSuccessWritesAll 反向划界：全成功时照常写 5 个键。
func TestApplySettingsSuccessWritesAll(t *testing.T) {
	p, db := newSubtitleProviderWithDB(t)
	if err := p.ApplySettings(fullSubtitleSettings()); err != nil {
		t.Fatalf("全成功不该报错：%v", err)
	}
	if n := countSubtitleKeys(t, db); n != 5 {
		t.Errorf("成功时应写入 5 个键，got %d", n)
	}
}

// TestApplySettingsSkipsEmptyValues 反向划界：空字段仍按「跳过=不修改」处理。
func TestApplySettingsSkipsEmptyValues(t *testing.T) {
	p, db := newSubtitleProviderWithDB(t)
	if err := p.ApplySettings(SubtitleSettings{AssrtToken: "a", TargetLang: "chs"}); err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if n := countSubtitleKeys(t, db); n != 2 {
		t.Errorf("空字段应跳过、只写 2 个键，got %d", n)
	}
}

// failAfterOper 在「第 from 次到第 to 次」之间失败，其它次照常通过 —— 用来精确制造
// 「写到第 N 个键时库出问题」这个场景。
//
// 关键：回滚的写也走这里，而回滚**必须能成功**。若按「前 N 次后永久失败」计数，
// 回滚会被同一个故障拦截，于是库里必然停在中间态 —— 那样测的就不是
// 「代码有没有回滚」，而是我自己设的计数器，是假因。
// 所以用区间 [from, to] 精确表达「第 3 次失败、其余正常」。
type failAfterOper struct {
	*oper.SystemConfigOper
	from, to int
	n        int
}

func (f *failAfterOper) Set(k, v string) error {
	f.n++
	if f.n >= f.from && f.n <= f.to {
		return errors.New("disk full")
	}
	return f.SystemConfigOper.Set(k, v)
}
