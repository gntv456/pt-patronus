// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

// #406 接线护栏：直链签名密钥的装配必须走 HKDF 域派生 + 带上升级前的旧钥。
//
// 行为测试（chain 包）能证明「给了两把钥会怎样」，证明不了 app **真的**给了两把。
// 而这条恰好是本仓反复出现的形状：判据写好了、测试也绿，但某个出口/装配点
// 没跟着换，于是用户拿到的是 404 而不是新签名。所以这里钉装配点本身。

import (
	"os"
	"strings"
	"testing"
)

// TestAppWiring_DirectLinkSignKeyIsDomainSeparated app 注入的必须是派生密钥 + 旧钥。
func TestAppWiring_DirectLinkSignKeyIsDomainSeparated(t *testing.T) {
	body := appFuncBody(t, "SetDirectLinkSignKeys")

	// 正向：新密钥来自 DeriveKey(...) 而不是直接 []byte(cfg.SecretKey)。
	if !strings.Contains(body, "DeriveKey(") {
		t.Errorf("直链签名新密钥必须经 DeriveKey 域派生（#406），实得：%s", body)
	}
	// 第二段必须是升级前的旧原始字节 —— 没有它，存量 .strm 的签名验不过，
	// 升级当天播放器全库 404。这里钉住「旧钥还得在」，不钉变量名。
	if !strings.Contains(body, "[]byte(cfg.SecretKey)") {
		t.Errorf("直链校验侧必须同时带上升级前的旧钥（[]byte(cfg.SecretKey)），否则存量 .strm 全 404：%s", body)
	}
	// 反向（重点）：不许再出现单参 SetDirectLinkSignKey([]byte(cfg.SecretKey))。
	if strings.Contains(body, "SetDirectLinkSignKey(") {
		t.Errorf("app 装配不许再走单参 SetDirectLinkSignKey（没有 legacy 回退，升级即全库 404）：%s", body)
	}
	// 反向：整个 app 包里都不该再有「原始 SECRET_KEY 直接当签名密钥」。
	src := stripLineComments(t, "app.go")
	if strings.Contains(src, "SetDirectLinkSignKey([]byte(cfg.SecretKey))") {
		t.Error("app.go 仍把原始 SECRET_KEY 当直链签名密钥 —— 域分隔没生效（#406）")
	}
}

// appFuncBody 读 app.go，剥掉整行注释后返回指定函数/调用的调用块（含括号配平）。
func appFuncBody(t *testing.T, call string) string {
	t.Helper()
	src := stripLineComments(t, "app.go")
	idx := strings.Index(src, call)
	if idx < 0 {
		t.Fatalf("app.go 里找不到 %s —— 装配点挪走了？请复核（#406）", call)
	}
	open := strings.Index(src[idx:], "(")
	if open < 0 {
		t.Fatalf("%s 后面没有左括号", call)
	}
	depth, end := 0, -1
	for i := idx + open; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatalf("%s 的括号没配平", call)
	}
	return src[idx : end+1]
}

// stripLineComments 去掉整行 // 与 /* */ 注释，避免注释里的旧形状被当成证据。
func stripLineComments(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读不到 %s: %v", file, err)
	}
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	inBlock := false
	for _, ln := range lines {
		s := strings.TrimSpace(ln)
		if inBlock {
			if strings.Contains(s, "*/") {
				inBlock = false
			}
			continue
		}
		if strings.HasPrefix(s, "/*") {
			if !strings.Contains(s, "*/") {
				inBlock = true
			}
			continue
		}
		if strings.HasPrefix(s, "//") || s == "" {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}
