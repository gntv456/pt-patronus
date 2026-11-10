// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #393 云盘 Generate 出口是 7 处同类出口里唯一手抄的一份，而且是「退步」的那份。
//
// 同一个文件里同类形状的分布：
//
//	cloud.go:47   Create            → respondCloudErr(c, err)               ← 带原因
//	cloud.go:65   Delete            → respondCloudErr(c, err)               ← 带原因
//	cloud.go:79   Get               → respondRecordError(...)                ← 带原因
//	cloud.go:109  （上传/移动类）     → respondRecordError(..., BadGateway)   ← 带原因
//	cloud.go:124  （下载/列举类）     → respondRecordError(..., BadGateway)   ← 带原因
//	cloud.go:220  Generate          → 手抄：
//	    if errors.Is(err, gorm.ErrRecordNotFound) {
//	        errcode.Respond(c, errcode.CloudDriveNotFound)      // ← 没有 WithDetail
//	        return
//	    }
//	    errcode.Respond(c, errcode.Internal.WithErr(err))
//	cloud.go:242  respondCloudErr 本体
//
// 分类本身是对的（record not found 与真故障分开了），所以它不像 #345 那样说错话。
// 但它**丢掉了原因**：respondRecordError 那一族全部走 WithDetail/WithErr，
// 用户能看到「是哪个云盘账号没找到」；Generate 这条只剩一句「云盘账号不存在」。
//
// 危害不在分类，在可诊断性：云盘账号被删/被改名后，Generate（文件页选中目录即时生成 STRM）
// 会持续失败，而用户/运维从响应里看不到任何定位线索，只能去翻日志。
//
// 与 #385/#387 同族——「同一族里有一份没跟上」，只是这次退步的是细节而非分类。

import (
	"os"
	"strings"
	"testing"
)

// TestCloudGenerateOutletUsesSharedClassifier Generate 必须走统一判据。
func TestCloudGenerateOutletUsesSharedClassifier(t *testing.T) {
	src, err := os.ReadFile("cloud.go")
	if err != nil {
		t.Fatalf("读不到 cloud.go: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")

	at := strings.Index(code, "func (h *CloudHandler) Generate(")
	if at < 0 {
		t.Fatal("找不到 Generate（判据没有落点）")
	}
	body := code[at:]
	if n := strings.Index(body[1:], "\nfunc "); n > 0 {
		body = body[:1+n]
	}
	// 反向断言：手抄的旧形状（裸 Respond(CloudDriveNotFound) 不带原因）。
	if strings.Contains(body, "errcode.Respond(c, errcode.CloudDriveNotFound)") {
		t.Error("Generate 还在手抄 `errcode.Respond(c, errcode.CloudDriveNotFound)`：" +
			"它是 7 处同类出口里唯一不带原因的一份（其余走 respondRecordError/respondCloudErr 都有）")
	}
	if !strings.Contains(body, "respondRecordError(") && !strings.Contains(body, "respondCloudErr(") {
		t.Error("Generate 必须走 respondRecordError/respondCloudErr（分类对了出口没接上是空转）")
	}
}

// TestCloudRecordNotFoundKeepsErrForLogs 正向钉住「统一判据会让原因进日志」。
//
// 严重性要说准：respondRecordError 的 404 档是 `notFound.WithErr(err)`，
// **不是** WithDetail —— 原因只进日志、不进响应体（这是对的，云盘/站点这一族
// 不该把内部错误原文泄给界面）。所以 #393 修的不是「给用户补一句原因」，
// 而是**可诊断性**：手抄版裸 Respond(CloudDriveNotFound) 时 e.Err 为 nil，
// errcode.Respond 连 slog 都不写，运维在日志里只看到一句 404。
//
// 早期版本把这条写成「必须 WithDetail（原因进响应体）」——那是把 #393 的危害夸大了
// （读错了 respondRecordError 的实现）。判据本身没错，但它论证的东西是假的。
func TestCloudRecordNotFoundKeepsErrForLogs(t *testing.T) {
	src, err := os.ReadFile("input_error.go")
	if err != nil {
		t.Fatalf("读不到 input_error.go: %v", err)
	}
	code := string(src)
	if !strings.Contains(code, "notFound.WithErr(err)") {
		t.Error("respondRecordError 的 404 档必须是 notFound.WithErr(err)：" +
			"原因要进日志（不进响应体）——否则这一族全部出口都会在运维侧「凭空消失」")
	}
}
