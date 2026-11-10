// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notice

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// fakeStore 实现 Store 接口，记录调用以便断言去重/写库/广播路径。
type fakeStore struct {
	notices     []models.Notice
	exists      bool // ExistsSince 固定返回值
	existsCalls int
	createCalls int
}

func (f *fakeStore) Create(n *models.Notice) error {
	f.createCalls++
	n.ID = uint(len(f.notices) + 1)
	f.notices = append(f.notices, *n)
	return nil
}
func (f *fakeStore) List(unread bool, limit int) ([]models.Notice, error) { return f.notices, nil }
func (f *fakeStore) Count(unread bool) (int64, error) {
	return int64(len(f.notices)), nil
}
func (f *fakeStore) MarkRead(id uint) error { return nil }
func (f *fakeStore) MarkAllRead() error     { return nil }
func (f *fakeStore) Delete(id uint) error   { return nil }
func (f *fakeStore) ClearAll() (int64, error) {
	n := int64(len(f.notices))
	f.notices = nil
	return n, nil
}
func (f *fakeStore) ExistsSince(source, key string, since time.Time) (bool, error) {
	f.existsCalls++
	return f.exists, nil
}
func (f *fakeStore) CountUnread() (int64, error)             { return 0, nil }
func (f *fakeStore) Cleanup(before time.Time) (int64, error) { return 0, nil }

// 当天已存在 → dedup 跳过（不写库、不广播）。
func TestNoticeDedupSkipsExisting(t *testing.T) {
	store := &fakeStore{exists: true}
	var broadcasted int
	svc := NewService(store, func([]byte) { broadcasted++ }, nil)

	if err := svc.Notice(context.Background(), Input{Source: SourceAnnounce, Key: "a1", Title: "t"}, true); err != nil {
		t.Fatalf("notice: %v", err)
	}
	if store.createCalls != 0 || store.existsCalls != 1 || broadcasted != 0 {
		t.Fatalf("dedup should skip create/broadcast, got create=%d exists=%d bcast=%d",
			store.createCalls, store.existsCalls, broadcasted)
	}
}

// 未去重/当天不存在 → 写库 + 广播 notice WS 消息。
func TestNoticeWritesAndBroadcasts(t *testing.T) {
	store := &fakeStore{exists: false}
	var payload []byte
	svc := NewService(store, func(p []byte) { payload = p }, nil)

	if err := svc.Notice(context.Background(), Input{Source: SourceBackupDone, Key: "b1", Title: "备份完成", Body: "ok", Level: "success"}, false); err != nil {
		t.Fatalf("notice: %v", err)
	}
	if store.createCalls != 1 {
		t.Fatalf("expected 1 create, got %d", store.createCalls)
	}
	var msg noticeMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("broadcast payload not a noticeMsg: %v", err)
	}
	if msg.Type != "notice" || msg.Data.Title != "备份完成" || msg.Data.Level != "success" || msg.Data.ID == 0 {
		t.Fatalf("unexpected broadcast msg: %+v", msg)
	}
}

// 空 Source/Title → 静默 no-op（防误发空通知）。
func TestNoticeNoopOnEmpty(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, nil, nil)
	_ = svc.Notice(context.Background(), Input{Source: "", Title: "x"}, false)
	_ = svc.Notice(context.Background(), Input{Source: "s", Title: ""}, false)
	if store.createCalls != 0 {
		t.Fatalf("empty source/title should be no-op, create=%d", store.createCalls)
	}
}

// 空级别兜底为 info。
func TestNoticeLevelDefault(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, nil, nil)
	_ = svc.Notice(context.Background(), Input{Source: "s", Title: "t"}, false)
	if got := store.notices[0].Level; got != "info" {
		t.Fatalf("empty level should default to info, got %q", got)
	}
}

// countingChannel 记录发送次数的假渠道；failFirst 模拟首次失败（重试后成功）。
type countingChannel struct {
	name      string
	failFirst bool
	mu        sync.Mutex
	sends     int
}

func (c *countingChannel) Name() string { return c.name }
func (c *countingChannel) Send(context.Context, notification.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sends++
	if c.failFirst && c.sends == 1 {
		return errors.New("boom")
	}
	return nil
}
func (c *countingChannel) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sends
}

// TestNoticeRetryOnlyFailedChannels 外发失败重试只重发失败渠道：多渠道中一个失败时，
// 已成功渠道不得重复收到推送（此前对全部渠道整体重发，用户收到重复消息）。
func TestNoticeRetryOnlyFailedChannels(t *testing.T) {
	oldDelay := noticeRetryDelay
	noticeRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { noticeRetryDelay = oldDelay })

	okCh := &countingChannel{name: "ok"}
	badCh := &countingChannel{name: "bad", failFirst: true}
	mgr := notification.NewManager(okCh, badCh)
	svc := NewService(&fakeStore{}, nil, mgr)

	if err := svc.Notice(context.Background(), Input{Source: "s", Title: "t", Level: "danger"}, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && badCh.count() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := badCh.count(); got != 2 {
		t.Fatalf("失败渠道应重发一次: sends=%d", got)
	}
	if got := okCh.count(); got != 1 {
		t.Fatalf("已成功渠道不得重复推送: sends=%d", got)
	}
}

// List/ClearAll 透传到 store。
func TestNoticeListAndClearAll(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, nil, nil)
	_ = svc.Notice(context.Background(), Input{Source: "s", Title: "t1"}, false)
	_ = svc.Notice(context.Background(), Input{Source: "s", Title: "t2"}, false)
	list, err := svc.List(false, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: len=%d err=%v", len(list), err)
	}
	n, err := svc.ClearAll()
	if err != nil || n != 2 {
		t.Fatalf("clearall: n=%d err=%v", n, err)
	}
}
