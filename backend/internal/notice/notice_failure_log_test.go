// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notice

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// failCreateStore 只让落库失败；其余方法沿用包内 fakeStore。
type failCreateStore struct{ *fakeStore }

func (s failCreateStore) Create(n *models.Notice) error {
	return errors.New("database is locked")
}

// failExistsStore 只让去重查询失败（这条失败决定了提醒到底发得出发不出）。
type failExistsStore struct{ *fakeStore }

func (s failExistsStore) ExistsSince(source, key string, since time.Time) (bool, error) {
	return false, errors.New("no such table: notices")
}

func captureSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)
	fn()
	return buf.String()
}

// TestNoticeLogsStoreFailures 本仓 20+ 个调用点一律写 `_ = svc.Notice(...)`（chain/automation/app/
// backup/webhook/ws……），其中不少正是「读不出来要出声」那类自曝通知（目录同步清单读不出、
// 刷流选种清单读不出、订阅追更记录读不出、站点掉线……）。Service 从前只把错误 return 出去、
// 自己不记：调用方既然丢弃，这条提醒就凭空消失，日志里也找不到任何痕迹——
// 提醒机制自己坏了没人知道，被提醒的事也就没人知道。
// 判据因此修在这一个共用出口，而不是逐个调用点补日志。
func TestNoticeLogsStoreFailures(t *testing.T) {
	base := &fakeStore{}

	t.Run("落库失败要点名是哪条提醒", func(t *testing.T) {
		logged := captureSlog(t, func() {
			svc := NewService(failCreateStore{base}, nil, nil)
			err := svc.Notice(context.Background(), Input{
				Source: SourceDirSyncListFail, Key: "dirsync-listfail:2026-10-13",
				Title: "目录同步：任务清单读不出来", Level: "warning",
			}, false)
			if err == nil {
				t.Fatal("错误仍要回传给调用方（出声不等于替调用方决定吞掉）")
			}
		})
		if strings.TrimSpace(logged) == "" {
			t.Fatal("一条日志都没打 ⇒ 旧行为：错误只回传、调用点 `_ =` 丢弃，全程静默")
		}
		// source 用常量本身比对：字面值（dirsync_list_fail）我第一版凭印象写成 dir_sync_list_fail，
		// 红的是我的断言而不是日志——这条提醒能不能被归因，不该跟着常量的拼写漂移。
		for _, want := range []string{"通知落库失败", "目录同步：任务清单读不出来", "database is locked", string(SourceDirSyncListFail)} {
			if !strings.Contains(logged, want) {
				t.Errorf("日志里要点名 %q, got %q", want, logged)
			}
		}
	})

	t.Run("去重查询失败也要出声", func(t *testing.T) {
		logged := captureSlog(t, func() {
			svc := NewService(failExistsStore{base}, nil, nil)
			if err := svc.Notice(context.Background(), Input{
				Source: SourceBrushflowListFail, Key: "brushflow-select-listfail:2026-10-13",
				Title: "刷流选种：任务清单读不出来",
			}, true); err == nil {
				t.Fatal("ExistsSince 失败必须回传")
			}
		})
		if strings.TrimSpace(logged) == "" {
			t.Fatal("一条日志都没打 ⇒ 去重查询失败同样是全程静默")
		}
		for _, want := range []string{"通知去重查询失败", "刷流选种：任务清单读不出来", "no such table"} {
			if !strings.Contains(logged, want) {
				t.Errorf("日志里要点名 %q, got %q", want, logged)
			}
		}
	})

	// 正控：正常发出时不该出现这两句（否则日志会被「失败」刷满，真失败反而看不见）。
	t.Run("正常一条不该记失败", func(t *testing.T) {
		logged := captureSlog(t, func() {
			svc := NewService(&fakeStore{}, nil, nil)
			if err := svc.Notice(context.Background(), Input{
				Source: SourceBackupDone, Key: "scheduled-backup:2026-10-13", Title: "定时备份完成", Level: "success",
			}, false); err != nil {
				t.Fatalf("正常路径不该失败: %v", err)
			}
		})
		if strings.Contains(logged, "通知落库失败") || strings.Contains(logged, "去重查询失败") {
			t.Errorf("成功路径混进了失败日志, got %q", logged)
		}
	})
}
