// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

// TestWebhookTriggerNotice 判定层：账面写失败这一态必须换说法、换级别。
func TestWebhookTriggerNotice(t *testing.T) {
	ok := webhookTriggerNotice("部署钩子", 128, true, nil)
	if ok.Level != "info" {
		t.Errorf("正常一次该是 info, got %q", ok.Level)
	}
	if strings.Contains(ok.Body, "没能记进面板") {
		t.Errorf("正常文案不该出现失败说明, got %q", ok.Body)
	}
	if !strings.Contains(ok.Body, "已触发工作流") {
		t.Errorf("触发了工作流要说出来, got %q", ok.Body)
	}

	bad := webhookTriggerNotice("部署钩子", 128, true, errBoom)
	if bad.Level != "warn" {
		t.Errorf("账面写失败该升成 warn, got %q", bad.Level)
	}
	for _, want := range []string{"模拟落库失败", "面板", "上一次"} {
		if !strings.Contains(bad.Body, want) {
			t.Errorf("通知里要点名 %q, got %q", want, bad.Body)
		}
	}
}

var errBoom = &stubErr{"模拟落库失败"}

type stubErr struct{ s string }

func (e *stubErr) Error() string { return e.s }

// TestWebhookReceiveSurfacesRecordFailureInNotice 出口层：真 sqlite 上用一条
// `BEFORE UPDATE ... RAISE(ABORT)` 触发器造出「读得到、写不进」——正是 MarkTriggered
// 会失败而 Receive 仍要正常应答的形状。旧写法 `_ = h.op.MarkTriggered(...)` 把错误丢掉，
// 通知中心仍记一条 info「已接收」，面板停在上一次触发，两头都在说谎。
func TestWebhookReceiveSurfacesRecordFailureInNotice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "webhook_record.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Webhook{}, &models.Notice{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewWebhookOper(db, security.NewCrypto("probe-key"))
	wh := &models.Webhook{Name: "deploy", Token: "tok-rec", Enabled: true}
	if err := op.Create(wh, ""); err != nil {
		t.Fatal(err)
	}
	// 只挡 UPDATE，SELECT 与 notices 表的写入照常。
	if err := db.Exec(`CREATE TRIGGER block_webhook_update BEFORE UPDATE ON webhooks
BEGIN SELECT RAISE(ABORT, '模拟落库失败'); END`).Error; err != nil {
		t.Fatalf("装触发器失败：%v", err)
	}

	ns := notice.NewService(oper.NewNoticeOper(db), nil, nil)
	h := NewWebhookHandler(op, nil, event.New(), ns)
	router := gin.New()
	router.POST("/webhooks/in/:token", h.Receive)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/in/tok-rec", bytes.NewReader([]byte(`{"kind":"push"}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200（外部系统仍该收到应答）, body=%q", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if resp["ok"] != true {
		t.Errorf("应答要如实 ok:true（工作流确实没被这次账面失败影响）, got %#v", resp)
	}

	var got []models.Notice
	if err := db.Order("id desc").Limit(3).Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("通知中心一条都没记 ⇒ 这条出口没有任何人出声")
	}
	n := got[0]
	if n.Level != "warn" {
		t.Errorf("账面写失败的通知该是 warn（旧写法恒 info）, got %q body=%q", n.Level, n.Body)
	}
	if !strings.Contains(n.Body, "模拟落库失败") || !strings.Contains(n.Body, "面板") {
		t.Errorf("通知要说清「面板上的触发次数/时间是旧的」, got %q", n.Body)
	}
}
