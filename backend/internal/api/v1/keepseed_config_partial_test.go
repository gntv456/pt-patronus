// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #392 辅种配置保存是「多次独立写、无事务」，失败出口却只说「服务内部错误，请稍后重试」。
//
// keepseed.go SetConfig 的形状：
//
//	if tok := strings.TrimSpace(req.IYUUToken); tok != "" {
//	    if err := h.ks.SetToken(tok); err != nil {
//	        errcode.Respond(c, errcode.Internal.WithErr(err))   // ← 500
//	        return
//	    }
//	}
//	if req.SetTorznab {
//	    if err := h.ks.SetTorznab(req.TorznabURL, req.TorznabKey); err != nil {
//	        errcode.Respond(c, errcode.Internal.WithErr(err))   // ← 500，但 token 已经存进去了
//	        return
//	    }
//	}
//
// SystemConfigOper.Set 每个 key 一次独立读写，**没有事务**（systemconfig.go）：
//
//	err := o.db.Where("`key` = ?", key).First(&sc).Error   ← 查
//	… Create 或 Update                                      ← 写
//
// 于是 SetTorznab 自己就是两次写（先 URL、后 APIKey）。后果分两层：
//
//  1) IYUU token 已存、Torznab 失败 ⇒ 响应 500。用户以为「什么都没存」，
//     于是再提交一次（重复写无害），但界面上仍显示失败；更糟的是他不知道
//     token 已经生效——下次辅种会用新 token，而他的直觉是「保存没成功」。
//  2) Torznab URL 已存、APIKey 失败 ⇒ **URL 生效但 Key 是旧的**，于是
//     Jackett/Prowlarr 每次请求都 401，而界面说的是「服务内部错误」。
//
// 这与 #336（备份批量写失败必须说「已整体回滚」）同族，方向相反：
// 那边是「一个都没落库」必须说清，这边是「**部分已经落库**」必须说清。
// 两者都是同一件事：不许让用户在不知道「到底存进去多少」的情况下决定下一步。

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

// TestKeepSeedConfigPartialWriteMustSaySo 写失败必须说清「已经写进去的那部分」。
func TestKeepSeedConfigPartialWriteMustSaySo(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondKeepSeedConfigWriteErr(c, errors.New("sql: database is locked"), true /* token 已写入 */)

	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	msg, _ := env["message"].(string)

	// 判据按语义找「有一部分已经生效」，不钉死具体措辞（第一版查「已保存/已写入」，
	// 而文案写的是「已生效」，结果正确实现被判红 —— 判据比修法窄会逼后来人改文案迁就测试）。
	if !strings.Contains(msg, "已生效") && !strings.Contains(msg, "已保存") && !strings.Contains(msg, "已写入") {
		t.Errorf("写失败必须明说「哪些已经存进去了」（否则用户不知道该不该重试/会不会重复），got %q", msg)
	}
	if !strings.Contains(msg, "database is locked") {
		t.Errorf("原因必须带出来（运维靠它判断是磁盘满/连接断还是自己填错），got %q", msg)
	}
	// 通用话术是本次要消灭的那个「钉死的猜测」——
	// 「请稍后重试」在这里会让人以为什么都没存、再点一次保存。
	if strings.Contains(msg, "请稍后重试") {
		t.Errorf("不许说「请稍后重试」：前半截已经落库了，重试的前提是「什么都没存」这个假设不成立，got %q", msg)
	}
	if env["code"] == errcode.Internal.Code {
		t.Errorf("该是本域专属码（让前端能分开处理「部分生效」），got %v", env["code"])
	}
}

// 反向对照：第一步就失败（什么都没写）时，不许说「已保存」。
func TestKeepSeedConfigFirstWriteFailureSaysNothingSaved(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondKeepSeedConfigWriteErr(c, errors.New("sql: database is locked"), false)

	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	msg, _ := env["message"].(string)
	if strings.Contains(msg, "已保存") {
		t.Errorf("第一步就失败、什么都没写，不许说「已保存」（那是反方向的谎话），got %q", msg)
	}
	if !strings.Contains(msg, "database is locked") {
		t.Errorf("原因仍要带出来，got %q", msg)
	}
}

// TestKeepSeedConfigOutletsUseTheHelper 接线护栏：SetConfig 的两个写出口都要走新判据。
func TestKeepSeedConfigOutletsUseTheHelper(t *testing.T) {
	src, err := os.ReadFile("keepseed.go")
	if err != nil {
		t.Fatalf("读不到 keepseed.go: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")

	at := strings.Index(code, "func (h *KeepSeedHandler) SetConfig(")
	if at < 0 {
		t.Fatal("找不到 SetConfig（判据没有落点）")
	}
	// 只看 SetConfig 这个函数体（到下一个 func 为止）；上面的 respondKeepSeedErr
	// 属于另一个函数，不该被算进来（判据错位比判据宽更难发现）。
	body := code[at:]
	if n := strings.Index(body[1:], "\nfunc "); n > 0 {
		body = body[:1+n]
	}
	if strings.Contains(body, "errcode.Internal.WithErr(err)") {
		t.Error("SetConfig 的写出口还在用 Internal.WithErr（旧形状：500「服务内部错误，请稍后重试」，" +
			"而此时 IYUU token 可能已经存进去了）")
	}
	if strings.Count(body, "respondKeepSeedConfigWriteErr(") < 2 {
		t.Errorf("SetConfig 的两个写出口都要走新判据（当前 %d 处）",
			strings.Count(body, "respondKeepSeedConfigWriteErr("))
	}
}
