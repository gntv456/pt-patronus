// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// stubNoticeStore 只实现 notice.Store：Create 可注入错误，模拟写库失败。
type stubNoticeStore struct{ createErr error }

func (s stubNoticeStore) Create(*models.Notice) error             { return s.createErr }
func (s stubNoticeStore) List(bool, int) ([]models.Notice, error) { return nil, nil }
func (s stubNoticeStore) Count(bool) (int64, error)               { return 0, nil }
func (s stubNoticeStore) MarkRead(uint) error                     { return nil }
func (s stubNoticeStore) MarkAllRead() error                      { return nil }
func (s stubNoticeStore) Delete(uint) error                       { return nil }
func (s stubNoticeStore) ClearAll() (int64, error)                { return 0, nil }
func (s stubNoticeStore) ExistsSince(string, string, time.Time) (bool, error) {
	return false, nil
}
func (s stubNoticeStore) CountUnread() (int64, error)      { return 0, nil }
func (s stubNoticeStore) Cleanup(time.Time) (int64, error) { return 0, nil }

// TestSendMessageNodeFailsLoudly 回归：站内通知节点此前把 Notice 的返回丢给 _ =，
// 通知服务没装配、写库失败都照样往运行日志里写「已发送通知：X」并返回 nil，
// 工作流记成 success —— 用户以为这条通知会提醒自己，实际一条都没落地。
func TestSendMessageNodeFailsLoudly(t *testing.T) {
	params := map[string]any{"title": "下载完成提醒", "body": "种子已完成", "level": "info"}

	// 1. 通知服务未装配
	ecx := &workflow.Context{Variables: map[string]any{}}
	if err := NewSendMessageNode(nil).Execute(context.Background(), params, ecx); err == nil {
		t.Fatal("notice 为 nil 时必须报错，不能报「已发送」")
	}
	if len(ecx.Messages) != 0 {
		t.Errorf("未发送成功就不该写「已发送通知」运行日志，got %v", ecx.Messages)
	}

	// 2. 写库失败
	boom := errors.New("database is locked")
	ecx2 := &workflow.Context{Variables: map[string]any{}}
	err := NewSendMessageNode(notice.NewService(stubNoticeStore{createErr: boom}, nil, nil)).
		Execute(context.Background(), params, ecx2)
	if err == nil {
		t.Fatal("Notice 返回错误时必须向上冒泡（节点失败 = 工作流失败），got nil")
	}
	if !strings.Contains(err.Error(), "database is locked") {
		t.Errorf("错误要带上底层原因，got %v", err)
	}
	if len(ecx2.Messages) != 0 {
		t.Errorf("失败时运行日志不该出现「已发送」，got %v", ecx2.Messages)
	}

	// 3. 正控：真发出去才报成功
	ecx3 := &workflow.Context{Variables: map[string]any{}}
	if err := NewSendMessageNode(notice.NewService(stubNoticeStore{}, nil, nil)).
		Execute(context.Background(), params, ecx3); err != nil {
		t.Fatalf("写库成功应返回 nil，got %v", err)
	}
	if len(ecx3.Messages) != 1 || !strings.Contains(ecx3.Messages[0], "已发送通知：下载完成提醒") {
		t.Errorf("成功时要留一条具名运行日志，got %v", ecx3.Messages)
	}
}
