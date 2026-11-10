// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// stubNoticeStore 只装收件箱读面需要的账面：List 按 limit 截断，Count 可控地出错。
type stubNoticeStore struct {
	ns          []models.Notice
	total       int64
	unreadTotal int64
	cntErr      error
	gotUnrd     bool
	gotLim      int
}

func (s *stubNoticeStore) Create(n *models.Notice) error { return nil }
func (s *stubNoticeStore) List(unread bool, limit int) ([]models.Notice, error) {
	s.gotUnrd, s.gotLim = unread, limit
	if limit > 0 && len(s.ns) > limit {
		return s.ns[:limit], nil
	}
	return s.ns, nil
}
func (s *stubNoticeStore) Count(unread bool) (int64, error) {
	s.gotUnrd = unread
	if unread {
		return s.unreadTotal, s.cntErr
	}
	return s.total, s.cntErr
}
func (s *stubNoticeStore) MarkRead(id uint) error                  { return nil }
func (s *stubNoticeStore) MarkAllRead() error                      { return nil }
func (s *stubNoticeStore) Delete(id uint) error                    { return nil }
func (s *stubNoticeStore) ClearAll() (int64, error)                { return 0, nil }
func (s *stubNoticeStore) CountUnread() (int64, error)             { return s.total, s.cntErr }
func (s *stubNoticeStore) Cleanup(before time.Time) (int64, error) { return 0, nil }
func (s *stubNoticeStore) ExistsSince(source, key string, since time.Time) (bool, error) {
	return false, nil
}

func callNoticeList(t *testing.T, st *stubNoticeStore, query string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewNoticeHandler(notice.NewService(st, nil, nil))
	router := gin.New()
	router.GET("/notices", h.List)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/notices"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func sixNotices() []models.Notice {
	ns := make([]models.Notice, 0, 6)
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		ns = append(ns, models.Notice{
			ID: uint(i + 1), Source: "cron", Title: "t", Level: "info",
			Read: i >= 4, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	return ns
}

// TestNoticeListAccount 收件箱取数账面：
// ① 被上限截断时必须报 total/truncated，否则「最近 200 条」被读成整个收件箱，
//
//	超出部分的未读永远看不见（点「全部已读」时也以为没剩多少）；
//
// ② 被归一的 limit 要回显生效值；
// ③ 计数读不出时报「可能被截断」并省略 total，不编一个 0；
// ④ unread=true 时 Count 必须走同一套筛选，否则「4 条未读」会被总数顶掉。
func TestNoticeListAccount(t *testing.T) {
	st := &stubNoticeStore{ns: sixNotices(), total: 6}
	out := callNoticeList(t, st, "?limit=3")
	if got, _ := out["notices"].([]any); len(got) != 3 {
		t.Errorf("清单应 3 条，got %v", out["notices"])
	}
	if out["total"] != float64(6) {
		t.Errorf("total 应为 6，got %v", out["total"])
	}
	if out["truncated"] != true {
		t.Errorf("只回 3 条时必须报截断，got %v", out["truncated"])
	}
	if out["limit"] != float64(3) {
		t.Errorf("limit 应回显 3，got %v", out["limit"])
	}

	st = &stubNoticeStore{ns: sixNotices(), total: 6}
	out = callNoticeList(t, st, "?limit=999999")
	if out["limit"] != float64(oper.NoticeListLimit) {
		t.Errorf("超上限请求应回显生效上限 %d，got %v", oper.NoticeListLimit, out["limit"])
	}
	if st.gotLim != oper.NoticeListLimit {
		t.Errorf("传给 List 的应是归一后的 limit %d，got %d", oper.NoticeListLimit, st.gotLim)
	}
	if out["truncated"] != false {
		t.Errorf("6 条一次拿全不该报截断，got %v", out["truncated"])
	}

	st = &stubNoticeStore{ns: sixNotices(), total: 6}
	out = callNoticeList(t, st, "")
	if out["limit"] != float64(oper.NoticeListDefault) {
		t.Errorf("留空应回显默认 %d，got %v", oper.NoticeListDefault, out["limit"])
	}

	st = &stubNoticeStore{ns: sixNotices(), total: 6, cntErr: errors.New("db down")}
	out = callNoticeList(t, st, "?limit=3")
	if _, ok := out["total"]; ok {
		t.Errorf("计数失败时不该给出 total，got %v", out["total"])
	}
	if out["truncated"] != true {
		t.Errorf("计数失败应保守报截断，got %v", out["truncated"])
	}

	st = &stubNoticeStore{ns: sixNotices(), total: 6, unreadTotal: 2}
	out = callNoticeList(t, st, "?unread=true&limit=3")
	if !st.gotUnrd {
		t.Error("Count 没收到 unread 筛选：未读账面会被全量总数顶掉")
	}
	if out["total"] != float64(2) {
		t.Errorf("未读总数应为 2，got %v", out["total"])
	}

	// 全量请求也要带未读总数：铃铛角标若从这一页里数，未读超过上限就会少报。
	st = &stubNoticeStore{ns: sixNotices(), total: 6, unreadTotal: 2}
	out = callNoticeList(t, st, "?limit=3")
	if out["unread_total"] != float64(2) {
		t.Errorf("unread_total 应为 2，got %v", out["unread_total"])
	}

	st = &stubNoticeStore{ns: sixNotices(), total: 6, unreadTotal: 2, cntErr: errors.New("db down")}
	out = callNoticeList(t, st, "")
	if _, ok := out["unread_total"]; ok {
		t.Errorf("未读计数读不出时不该给出 unread_total，got %v", out["unread_total"])
	}
}
