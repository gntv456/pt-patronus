// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #377 第二步：把 credential 的 20 个调用方迁到 credentialWithErr，
// 让各出口真的说出「本地凭据读不出来」而不是站外话术。
//
// 第一步只让 credentialWithErr 返回可判别的错误，并保留了旧签名的兼容包装；
// 但 20 个调用方仍调旧签名 ⇒ 错误在最后一步被 `_` 丢掉，第一步等于没生效。
// 这一步把它们逐个迁完，兼容包装随之删除（`rg "credential\("` 必须为空）。
//
// 迁移前先钉住判据：本文件里不再有任何 `c.credential(` 调用。
// 那 20 处跨 10 个文件、每个出口的语义不同（消息进 results[].error、签到走 guard、
// 搜索走 SSE），逐个手改容易漏；用「旧调用为零」当守门，比逐个断言调用点更抗改。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoCallerUsesLegacyCredential(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		// 剥整行注释，免得注释里引用的旧形状被判成调用点。
		var lines []string
		for _, l := range strings.Split(src, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "//") {
				continue
			}
			lines = append(lines, l)
		}
		body := strings.Join(lines, "\n")
		// 只认调用形状 `c.credential(`（带第二个参数），不含定义处 credentialWithErr。
		if strings.Contains(body, "c.credential(") {
			offenders = append(offenders, f)
		}
	}
	if len(offenders) > 0 {
		t.Errorf("这些文件仍在用会吞掉错误的旧签名 c.credential(...)：%v\n"+
			"它们拿到的仍是空凭据，#377 的第一步等于没生效（见 credentialWithErr 的注释）",
			offenders)
	}
}

// 兼容包装删掉后不该再有它自己。
func TestLegacyCredentialWrapperRemoved(t *testing.T) {
	b, err := os.ReadFile("site_credential.go")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	body := strings.Join(lines, "\n")
	if strings.Contains(body, "func (c *SiteChain) credential(") {
		t.Error("兼容包装 credential() 该删了：20 个调用方已全部迁走，留着它就等于留了个" +
			"会吞掉凭据错误的入口（下次有人图省事又会调它）")
	}
}

// TestMigratedCallersDoNotSwallowError 迁移后的调用方不许再把错误丢掉。
//
// 这一条是被负控逼出来的：上一条只查「还有没有 c.credential( 调用」，
// 而负控把某处改成 `token, _ := c.credentialWithErr(...)`（错误被吞）时它**依然全绿**
// —— 签名对了、错误照样没了。这正是 #377 本来要消灭的形状，只是换了个函数名。
//
// 判据不逐个列调用点（改了函数名就漏），而是扫描所有迁过的文件里
// 「接住 credentialWithErr 的返回值却没有判 err」这一形状。
func TestMigratedCallersDoNotSwallowError(t *testing.T) {
	files := []string{
		"harem.go", "message.go", "site_capability.go", "site_checkin.go",
		"site_connection.go", "site_info.go", "site_request.go", "site_search.go",
		"site_torrent.go", "snapshot.go", "assessment.go", "medal.go", "medal_actions.go",
	}
	const call = "credentialWithErr(s, def)"
	var offenders []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		var lines []string
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "//") {
				continue
			}
			lines = append(lines, l)
		}
		body := strings.Join(lines, "\n")
		for i, l := range strings.Split(body, "\n") {
			if !strings.Contains(l, call) {
				continue
			}
			// 用 _ 接住错误就是旧形状。
			if strings.Contains(l, ", _ :=") || strings.Contains(l, ", _ =") {
				offenders = append(offenders, f+": 用 _ 丢弃了错误")
				continue
			}
			// 下一行必须是 err 判断；否则等于接住了却不处理。
			next := ""
			if i+1 < len(strings.Split(body, "\n")) {
				next = strings.Split(body, "\n")[i+1]
			}
			if !strings.Contains(next, "!= nil") {
				offenders = append(offenders, f+": 接住错误后没有判断")
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("这些调用点接住 credentialWithErr 的错误又丢掉了（旧形状）：\n  %v\n"+
			"签名对了、错误照样没了 —— 那正是 #377 要消灭的形状",
			strings.Join(offenders, "\n  "))
	}
}
