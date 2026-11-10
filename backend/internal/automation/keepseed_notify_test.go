// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

// #352 自动辅种主流程失败只写日志、不出声——而同文件「读设置失败」那处已经发了通知。
//
// keepseed.go 的形状不一致得很明显：
//
//	:28 ListAutoReseedSettings 失败 → notif.Notify，注释写明「静默 = 自动辅种整轮不跑、
//	                                pending 候选停更，读失败必须出声」
//	:45 Reseed（逐台跑辅种）失败      → 只 slog.Warn 然后 continue   ← 洞
//	:52 Resolve("add")（自动添加）失败 → 只 slog.Warn 然后 continue   ← 洞
//
// 后两处恰恰是「整轮不跑」的主体：某台下载器辅种连续失败，用户在界面上看不到任何东西，
// 只会发现「pending 候选一直不更新」，却不知道是辅种没跑、跑了失败、还是本来就没有。
//
// 对照组（说明这不是「自动化域都不出声」，而是这一处漏了）：
//	keepalive.go:27  读站点失败 → notice.Notice（另一个通道）有出声
//	hr.go:60         单站失败  → 收集 failReasons，在 hrGapNote 里聚合播报，有出声
//	stats.go:55      签到失败  → fail++ 计数，:74 拼「成功 N，失败 M」，:84 通知，有出声
//	keepseed.go:46/53          → 无出声

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// fakeKeepSeedFail 让 Reseed / Resolve 按需失败（沿用 keepseed_test.go 的接口）。
type fakeKeepSeedFail struct {
	reseedErr  error
	resolveErr error
	reseeded   []uint
}

func (f *fakeKeepSeedFail) ListAutoReseedSettings() ([]models.CrossSeedSetting, error) {
	return []models.CrossSeedSetting{{DownloaderID: 1, AutoReseed: true, AutoReseedAdd: true}}, nil
}
func (f *fakeKeepSeedFail) Reseed(_ context.Context, dl uint) ([]models.CrossSeedTask, error) {
	f.reseeded = append(f.reseeded, dl)
	if f.reseedErr != nil {
		return nil, f.reseedErr
	}
	return []models.CrossSeedTask{{ID: 1, DownloaderID: dl, Status: "pending"}}, nil
}
func (f *fakeKeepSeedFail) Resolve(_ context.Context, taskID uint, action string) error {
	return f.resolveErr
}

// fakeNotifier 记录收到的通知。
type fakeNotifier struct{ got []notification.Message }

func (n *fakeNotifier) Notify(_ context.Context, m notification.Message) []notification.SendResult {
	n.got = append(n.got, m)
	return nil
}

// TestRunKeepSeedFailureMustNotify 辅种主流程失败必须出声，不能只写日志。
func TestRunKeepSeedFailureMustNotify(t *testing.T) {
	cases := []struct {
		name  string
		fake  *fakeKeepSeedFail
		about string
	}{
		{"Reseed 失败", &fakeKeepSeedFail{reseedErr: errors.New("IYUU 接口连不上")}, "辅种"},
		{"自动添加失败", &fakeKeepSeedFail{resolveErr: errors.New("下载器拒绝了这个种子")}, "添加"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			notif := &fakeNotifier{}
			svc := New(nil, nil, nil, notif, nil, c.fake, nil, Config{}, context.Background(), nil, nil, nil, nil)
			svc.runKeepSeed()
			if len(notif.got) == 0 {
				t.Errorf("%s 只写了日志、没有通知：用户在界面上看不到「自动辅种没跑/没加上」", c.name)
				return
			}
			body := notif.got[0].Body
			if !strings.Contains(body, c.about) {
				t.Errorf("通知要说清是%s这件事，got %q", c.about, body)
			}
			if !strings.Contains(body, "辅种") {
				t.Errorf("通知要带上「辅种」这个上下文，got %q", body)
			}
		})
	}
}

// TestRunKeepSeedSilentPathsKeepQuiet 反向对照：零动作与成功路径不许发通知
// （无下载器启用就是正常的空转，通知它是噪声）。
func TestRunKeepSeedSilentPathsKeepQuiet(t *testing.T) {
	notif := &fakeNotifier{}
	fk := &fakeKeepSeed{}
	svc := New(nil, nil, nil, notif, nil, fk, nil, Config{}, context.Background(), nil, nil, nil, nil)
	svc.runKeepSeed() // 无下载器启用
	if len(notif.got) != 0 {
		t.Errorf("零动作不许发通知（那是噪声），got %+v", notif.got)
	}

	notif2 := &fakeNotifier{}
	fk2 := &fakeKeepSeed{settings: []models.CrossSeedSetting{{DownloaderID: 1, AutoReseed: true}}}
	svc2 := New(nil, nil, nil, notif2, nil, fk2, nil, Config{}, context.Background(), nil, nil, nil, nil)
	svc2.runKeepSeed() // 正常跑完
	if len(notif2.got) != 0 {
		t.Errorf("成功路径不许发通知，got %+v", notif2.got)
	}
}

// TestRunKeepSeedAggregatedFailureStillNotifies 「逐条失败 → 循环外汇总播报」这种
// 合法形态（逐条播报会刷屏）也必须真的发出通知。
//
// 这一条替代了先前那个「按源码扫描判接线」的护栏：那个护栏连续三次因窗口/正则
// 细节打滑（扫到别的函数、扫不到块外汇总、被中间的花括号截断），每修一次就有
// 人会去改判据而不是改代码——护栏一旦不可靠，不如换成直接验行为。
// 接线本身由 TestRunKeepSeedFailureMustNotify 与下面这条一起覆盖：
// 覆盖不到的是「只留 Reseed 失败那处通知、删掉 add 的」这种半吊子。
func TestRunKeepSeedAggregatedFailureStillNotifies(t *testing.T) {
	notif := &fakeNotifier{}
	fk := &fakeKeepSeedFail{resolveErr: errors.New("下载器拒绝了这个种子")}
	svc := New(nil, nil, nil, notif, nil, fk, nil, Config{}, context.Background(), nil, nil, nil, nil)
	svc.runKeepSeed()
	if len(notif.got) == 0 {
		t.Fatal("逐条 add 失败后必须汇总播报（只写日志的话，界面看不出「开了自动辅种却什么都没加」）")
	}
	body := notif.got[0].Body
	if !strings.Contains(body, "1") {
		t.Errorf("汇总播报要说清有几个没加上，got %q", body)
	}
	if !strings.Contains(body, "下载器拒绝了这个种子") {
		t.Errorf("汇总播报要带上真实原因，got %q", body)
	}
}
