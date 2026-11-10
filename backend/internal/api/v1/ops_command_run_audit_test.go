// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #391 批量运维命令的**审计写失败**出口只说「服务内部错误，请稍后重试」。
//
// ops_command_run.go 的形状（Run）：
//
//	run := &models.OpsCommandRun{ ... Status: "running", TotalCount: len(targetIDs) ... }
//	if err := h.runs.Create(run); err != nil {
//	    errcode.Respond(c, errcode.Internal.WithErr(err))   // ← 500「服务内部错误，请稍后重试」
//	    return
//	}
//	// …之后才是几十台目标的批量执行…
//
// 审计写在执行之前，这个顺序本身是对的（审计写不进去就不该执行无记录的命令）。
// 但失败时用户只看到一句「服务内部错误，请稍后重试」，而真实后果是：
//
//	**一条命令都没有执行**。
//
// 「请稍后重试」在这里是最坏的一种引导：他会重试、重试、再重试，
// 而如果第一次其实在某台机器上跑了一半（DB 在 Create 之后、执行之前挂的边界情形），
// 重试就是重复执行。而他完全不知道第一次到底跑了没有。
//
// 同族且更早的一条是 #336（手动转存那次把原因说成「请求的资源不存在」）：
// 同一句话「把原因钉成一个猜测」。这条更重 —— 猜错的代价从「以为被删了」变成
// 「以为没执行 / 或者以为执行了却重复执行」。
//
// 正确形状：明确说「本次没有执行任何命令」，并带上真原因（磁盘满/连接断/锁），
// 让人能判断是重试（确认没跑过才安全）还是先修库。

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// TestOpsAuditWriteFailureMustSayNothingRan 审计写失败必须明说「什么都没执行」。
func TestOpsAuditWriteFailureMustSayNothingRan(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondOpsAuditWriteErr(c, errors.New("sql: database is locked"))

	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	msg, _ := env["message"].(string)

	// 1) 必须明说「没有执行」——这是用户判断能不能安全重试的唯一依据。
	if !strings.Contains(msg, "没有执行") && !strings.Contains(msg, "未执行") {
		t.Errorf("审计写失败必须明说「本次没有执行任何命令」（用户据此判断能否安全重试），got %q", msg)
	}
	// 2) 真实原因要带出来（DB 故障与参数错误的下一步完全不同）。
	if !strings.Contains(msg, "database is locked") {
		t.Errorf("原因必须带进响应体（运维靠它判断是磁盘满/连接断还是自己填错），got %q", msg)
	}
	// 3) 通用话术就是这次要消灭的那个「钉死的猜测」——
	//    「请稍后重试」在这里会诱导重复执行。
	if strings.Contains(msg, "请稍后重试") {
		t.Errorf("不许说「请稍后重试」：审计没写成、命令一条都没跑，重试是盲目的，got %q", msg)
	}
	if env["code"] == errcode.Internal.Code {
		t.Errorf("该是本域专属码（让前端能分开处理），got %v", env["code"])
	}
}

// TestOpsAuditOutletUsesTheNewHelper 接线护栏：Run 的审计出口必须走新判据。
func TestOpsAuditOutletUsesTheNewHelper(t *testing.T) {
	src, err := os.ReadFile("ops_command_run.go")
	if err != nil {
		t.Fatalf("读不到 ops_command_run.go: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")

	at := strings.Index(code, "h.runs.Create(run)")
	if at < 0 {
		t.Fatal("找不到 h.runs.Create(run)（判据没有落点）")
	}
	// 只看紧跟其后的 200 字：第一版按「到下一个 \nfunc 」切片，结果切到了
	// ops_error.go 那边同名的 helper 定义（测试与被测在不同文件，注释还各写各的），
	// 于是无论出口改没改都在判一段无关代码 —— 判据错位比判据宽更难发现。
	after := code[at:]
	if len(after) > 200 {
		after = after[:200]
	}
	if strings.Contains(after, "errcode.Internal.WithErr(err)") {
		t.Error("审计写出口还在用 Internal.WithErr（旧形状：用户只看到「服务内部错误，请稍后重试」，" +
			"不知道一条命令都没执行）")
	}
	if !strings.Contains(after, "respondOpsAuditWriteErr(c, err)") {
		t.Error("必须走 respondOpsAuditWriteErr（分类对了出口没接上是空转）")
	}
}
