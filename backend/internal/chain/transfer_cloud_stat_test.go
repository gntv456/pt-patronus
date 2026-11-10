// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"os"
	"strings"
	"testing"
)

// #322 云上传排队循环里，本地文件 stat 失败曾是一句裸 continue：
// 同一个循环的另两支（映射网盘路径、入队失败）都写了 slog.Warn，只这支没有——
// 那一个文件就此不进云盘任务队列，而「已排队 N 个文件」的通知照发，用户看不出少传了哪个。
// （同函数兄弟分支的留痕方式就是这条判据的 A/B。）

func TestCloudUploadStatFailureIsLogged(t *testing.T) {
	src, err := os.ReadFile("transfer_cloud.go")
	if err != nil {
		t.Fatalf("读不到 transfer_cloud.go：%v", err)
	}
	code := stripStatComments(string(src))
	i := strings.Index(code, "fi, err := os.Stat(f)")
	if i < 0 {
		t.Fatalf("找不到 os.Stat 那一支（否则下面的判据是空的）")
	}
	branch := code[i:]
	branch = branch[:min(200, len(branch))]
	if !strings.Contains(branch, `slog.Warn("cloud upload: stat failed`) {
		t.Errorf("stat 失败必须留痕（否则少传文件无从排障）：实际片段 %q", firstCodeLines(branch, 3))
	}
	if strings.Contains(branch, "if err != nil {\n\t\t\t\t\tcontinue") {
		t.Errorf("这一支不许退回裸 continue：%q", firstCodeLines(branch, 4))
	}
}

// #323 同一个函数里另一支（cloudPathFor 映射网盘路径）此前也是裸 continue。
func TestCloudStrmMapPathFailureIsLogged(t *testing.T) {
	src, err := os.ReadFile("transfer_cloud.go")
	if err != nil {
		t.Fatalf("读不到 transfer_cloud.go：%v", err)
	}
	code := stripStatComments(string(src))
	// cloudPathFor 在本文件有两处调用：上传排队那一处早就有 Warn，要盯的是 STRM 生成这一处。
	from := strings.Index(code, "func (c *TransferChain) emitCloudStrm")
	if from < 0 {
		t.Fatalf("找不到 emitCloudStrm")
	}
	to := strings.Index(code[from+1:], "\nfunc ")
	fnBody := code[from:]
	if to > 0 {
		fnBody = code[from : from+1+to]
	}
	i := strings.Index(fnBody, "cloudPath, err := cloudPathFor(cu, res.DestDir, f)")
	if i < 0 {
		t.Fatalf("emitCloudStrm 里找不到 cloudPathFor 那一支")
	}
	branch := fnBody[i:]
	branch = branch[:min(240, len(branch))]
	if !strings.Contains(branch, `slog.Warn("cloud strm: map path failed`) {
		t.Errorf("映射不出网盘路径必须留痕（否则 .strm 静默不生成，媒体库少一部）：%q", firstCodeLines(branch, 3))
	}
}

func stripStatComments(s string) string {
	out := make([]string, 0, 96)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func firstCodeLines(s string, n int) string {
	parts := strings.Split(strings.TrimSpace(s), "\n")
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, " | ")
}
