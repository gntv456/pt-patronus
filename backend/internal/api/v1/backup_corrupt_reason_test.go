// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #375 加密备份文件损坏时，界面说「请求参数有误」并甩一句英文 `bad salt`/`bad iv`/`bad data`。
//
// 备份导入里 Decrypt 的三条解析失败（encrypt.go:84/88/92）都返回英文原因，
// 而接口层那个分支是：
//
//	if errors.Is(err, backup.ErrBadPassword) { …BackupPasswordIncorrect… }
//	errcode.Respond(c, errcode.InvalidInput.WithErr(err))   // ← 400「没能指出具体是哪一项」
//
// WithErr 把英文丢进日志、界面只显示通用话术。用户此时的处境是：
// **他手上的文件坏了**（复制截断、被别的工具改过、传输损坏）。
// 而「请求参数有误」会让他反复点导入、反复检查自己没改过的表单 —— 与 #335
// 那句「请先到设置页解锁凭据库」同一形状：原因被钉成了一个与事实无关的猜测。
//
// 正确说法：该文件已损坏/不是有效备份，请重新导出一次。文件一旦损坏就再也救不回来，
// 这点必须说清，否则用户会以为是程序的问题。
//
// 判据钉三件事：文案是中文、点名「文件已损坏」这类事实、带上底层原因便于定位。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/backup"
)

func TestBackupCorruptFileReasonReachesClient(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"salt 段坏", fmt.Errorf("bad salt: %w", errors.New("illegal base64 data"))},
		{"iv 段坏", fmt.Errorf("bad iv: %w", errors.New("illegal base64 data"))},
		{"密文段坏", fmt.Errorf("bad data: %w", errors.New("illegal base64 data"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			respondBackupDecryptErr(c, tc.err)
			var env map[string]any
			if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
				t.Fatalf("响应不是 JSON：%q", rec.Body.String())
			}
			if env["code"] != errcode.BackupFileCorrupt.Code {
				t.Errorf("损坏文件该有自己的码，got %v（旧形状：invalid_input「参数有误」）", env["code"])
			}
			msg, _ := env["message"].(string)
			if !strings.Contains(msg, "损坏") && !strings.Contains(msg, "无效") {
				t.Errorf("文案要点明「文件已损坏/无效」，让用户知道要重新导出而不是改表单：%q", msg)
			}
			if strings.Contains(msg, "bad salt") || strings.Contains(msg, "bad iv") || strings.Contains(msg, "bad data") {
				t.Errorf("英文原文不该直接进界面：%q", msg)
			}
			// 刻意**不**要求界面带出 `bad salt: …` 这句英文：把库层原文塞进用户界面
			// 正是本条要消灭的行为（它既不可读、也把「文件坏了」这件事实挤掉了）。
			// 定位线索由 respond 的 WithErr 挂在 Err 上进日志，那里才是它的位置。
		})
	}
}

// 反向划界：口令错仍走 BackupPasswordIncorrect，别被这次改动带偏。
func TestBackupBadPasswordKeepsOwnCode(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondBackupDecryptErr(c, backup.ErrBadPassword)
	if code := backupEnvCode(t, rec); code != errcode.BackupPasswordIncorrect.Code {
		t.Errorf("口令错该是 %q，got %q", errcode.BackupPasswordIncorrect.Code, code)
	}
}

// 反向划界：空口令仍走 BackupPasswordRequired（NeedsPassword 分支已拦，这里是兜底）。
func TestBackupPasswordRequiredKeepsOwnCode(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondBackupDecryptErr(c, backup.ErrPasswordRequired)
	if code := backupEnvCode(t, rec); code != errcode.BackupPasswordRequired.Code {
		t.Errorf("未提供口令该是 %q，got %q", errcode.BackupPasswordRequired.Code, code)
	}
}

func backupEnvCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	c, _ := env["code"].(string)
	return c
}

// TestBackupImportOutletUsesClassifier 接线护栏：Import 的 Decrypt 出口必须走分类器。
//
// 这条是被负控逼出来的：4 条测试都直接调 respondBackupDecryptErr，把 backup.go 的
// 出口还原成 `InvalidInput.WithErr(err)` 后它们**依然全绿** —— 因为从没驱动过 handler。
// 分类对了、出口没接上，本轮已栽过多次。
func TestBackupImportOutletUsesClassifier(t *testing.T) {
	src, err := os.ReadFile("backup.go")
	if err != nil {
		t.Fatalf("读不到 backup.go：%v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")
	at := strings.Index(code, "func (h *BackupHandler) Import(")
	if at < 0 {
		t.Fatal("找不到 Import（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	decAt := strings.Index(tail, "backup.Decrypt(")
	if decAt < 0 {
		t.Fatal("找不到 backup.Decrypt 调用（判据没有落点）")
	}
	after := tail[decAt:]
	if strings.Contains(after, "errcode.InvalidInput.WithErr(err)") {
		t.Error("解密失败不许回 InvalidInput（旧形状：文件明明坏了，却让用户去查表单参数）")
	}
	if !strings.Contains(after, "respondBackupDecryptErr(c, err)") {
		t.Error("必须走 respondBackupDecryptErr 三档分类（分类对了出口没接上是空转）")
	}
}
