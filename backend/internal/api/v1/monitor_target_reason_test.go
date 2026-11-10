// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #373 AcceptHostKey 把 DB 真故障说成「监控目标不存在」，而同文件 Update 早就修好了。
//
// monitor.go:211 这段的注释本身就把结论写出来了：
//
//	// 原因仍要挂上：op.Get 底层是 db.First，可能是「记录不存在」也可能是 DB 真故障，
//	// 两者混成一句话会把真故障说成「没这台」。
//	errcode.Respond(c, errcode.MonitorTargetNotFound.WithErr(err))
//
// 也就是说 #341 写下这条注释时就知道该分开，注释也把理由讲清楚了，
// 但代码仍是一句 WithErr —— 注释描述的是意图，实现没跟上。
// 同一文件 monitor.go:69（Update）已经用的是 respondRecordError：
//
//	respondRecordError(c, err, errcode.MonitorTargetNotFound, errcode.InvalidInput, errcode.SaveFailed)
//
// 后果与 #372（Downloader Get）完全相同：数据库一挂，用户在删 SSH 密钥时看到的
// 也是「监控目标不存在」，于是去运维列表找那台根本还在的主机。
//
// 判据钉「该走 respondRecordError」，并保留状态码划界：真不存在仍是 404。

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"gorm.io/gorm"
)

func classifyMonitorTarget(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondRecordError(c, err, errcode.MonitorTargetNotFound, errcode.InvalidInput, errcode.SaveFailed)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	return rec.Code, env
}

func TestMonitorTargetGetSeparatesNotFoundFromDBFailure(t *testing.T) {
	status, env := classifyMonitorTarget(t, gorm.ErrRecordNotFound)
	if status != http.StatusNotFound || env["code"] != errcode.MonitorTargetNotFound.Code {
		t.Fatalf("记录不存在应 404 + %q，got %d + %v", errcode.MonitorTargetNotFound.Code, status, env["code"])
	}
	status2, env2 := classifyMonitorTarget(t, errors.New("sql: database is closed"))
	if status2 != http.StatusInternalServerError {
		t.Fatalf("DB 真故障应 500，got %d（旧形状：被说成「监控目标不存在」）", status2)
	}
	msg, _ := env2["message"].(string)
	if strings.Contains(msg, "不存在") {
		t.Errorf("DB 故障不许说成「不存在」：%q", msg)
	}
}

// TestAcceptHostKeyOutletUsesClassifier 接线护栏：AcceptHostKey 的 op.Get 必须走分类器。
func TestAcceptHostKeyOutletUsesClassifier(t *testing.T) {
	src, err := os.ReadFile("monitor.go")
	if err != nil {
		t.Fatalf("读不到 monitor.go：%v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")
	at := strings.Index(code, "func (h *MonitorHandler) AcceptHostKey(")
	if at < 0 {
		t.Fatal("找不到 AcceptHostKey（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, "errcode.MonitorTargetNotFound.WithErr(err)") {
		t.Error("AcceptHostKey 不许把 DB 真故障说成「监控目标不存在」" +
			"（本函数上方的注释写明了该分开，代码没跟上；同文件 Update 已经修好）")
	}
	if !strings.Contains(tail, "respondRecordError(c, err, errcode.MonitorTargetNotFound") {
		t.Error("必须走 respondRecordError（与同文件 Update 同一判据）")
	}
}
