// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// #253：过滤规则/规则组的写出口从前把三类完全不同的事压成同一句
// 「400 请求参数有误，请检查后重试」——
//   ① 记录不存在（别人删掉了 / 名字打错）→ 该 404 + 本域码，并点名是哪个名字；
//   ② 我们自己说清楚了的原因（id 已占用、被规则组引用、正则/表达式非法）→ 400 但要带那句人话；
//   ③ 真·存量数据坏了（读/解析 system_config 失败）→ 500，原因进日志不进界面。
// 这里的出口用例走真生产通路：handler → filter.Store → SystemConfigOper → SQLite。

func TestFilterGroupMissingIs404(t *testing.T) {
	r := setupFilterServer(t)

	rec := doFilter(t, r, http.MethodDelete, "/groups/ghost", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删除不存在的规则组应 404，got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "filter_group_not_found") {
		t.Fatalf("要用本域码，不许退回 NoRoute 的通用 not_found：body=%s", body)
	}
	if !strings.Contains(body, "ghost") || !strings.Contains(body, "规则组") {
		t.Fatalf("要点名是哪个规则组没了：body=%s", body)
	}

	// PUT 同一条路（改一个不存在的规则组）。注意 body 要引用真实存在的规则 id：
	// 组校验在存在性查找之前，引用悬空会先被判 400（那是对的，别拿它当 404 的证据）。
	doFilter(t, r, http.MethodPost, "/custom", `{"id":"k1","name":"K1"}`)
	rec = doFilter(t, r, http.MethodPut, "/groups/ghost", `{"name":"改名","rule_string":"k1"}`)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "filter_group_not_found") {
		t.Fatalf("改不存在的规则组应 404，got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFilterDeleteReferencedRuleExplainsWhy(t *testing.T) {
	r := setupFilterServer(t)
	doFilter(t, r, http.MethodPost, "/custom", `{"id":"used1","name":"U1"}`)
	rec := doFilter(t, r, http.MethodPost, "/groups", `{"name":"我的组","rule_string":"used1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("前置建组失败 code=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = doFilter(t, r, http.MethodDelete, "/custom/used1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("被引用的规则删除应 400（是业务拒绝，不是记录不存在），got %d", rec.Code)
	}
	body := rec.Body.String()
	// 关键是这句必须进响应：从前只有「请求参数有误」，用户不知道该先拆哪个组。
	if !strings.Contains(body, "我的组") || !strings.Contains(body, "引用") {
		t.Fatalf("响应要说出被哪个规则组引用：body=%s", body)
	}
}

func TestFilterInvalidExpressionCarriesReason(t *testing.T) {
	r := setupFilterServer(t)

	// 悬空引用（组里写了不存在的规则 id）：400 + 我们那句原因，而不是通用「参数有误」
	rec := doFilter(t, r, http.MethodPost, "/groups", `{"name":"g1","rule_string":"nope & x"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "不存在") {
		t.Fatalf("悬空引用应 400 并说明缺哪个 id，got %d body=%s", rec.Code, rec.Body.String())
	}

	// 规则测试口：非法表达式同样要把语法原因交回界面（不许只回通用文案，也不许升格成 500）
	rec = doFilter(t, r, http.MethodPost, "/test", `{"rule_string":"( 未闭合"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法表达式应 400，got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "请求参数有误，请检查后重试") {
		t.Fatalf("测试口不许只回通用文案，body=%s", body)
	}
	if !strings.Contains(body, "位置") && !strings.Contains(body, "括号") && !strings.Contains(body, "意外结束") {
		t.Fatalf("测试口要带表达式语法原因（至少点名出错位置），body=%s", body)
	}
}

// 结构护栏：六个写出口必须走同一份判据；剩下的通用 400 只允许是「body 绑定失败」。
// 只扫真源码并先剥 // 注释——我自己写的说明里也出现了旧写法字样，不剥就会自造假红
// （同 TestCloudOutletsShareTheClassifier / TestLocalDockerOutletsAllClassified 的做法）。
func TestFilterOutletsShareTheClassifier(t *testing.T) {
	src, err := os.ReadFile("filter_rule.go")
	if err != nil {
		t.Fatalf("读不到被测源文件: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(src), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		lines = append(lines, line)
	}
	joined := strings.Join(lines, "\n")

	if got := strings.Count(joined, "respondFilterErr(c, err,"); got < 7 {
		t.Errorf("走统一判据的出口 = %d, want ≥7（六个增删改出口 + 测试口）⇒ 又有出口自己抄归类", got)
	}
	if got := strings.Count(joined, "notFound.WithDetail(err)"); got != 1 {
		t.Errorf("「不存在 ⇒ 404」的判定应只写一处 = %d, want 1", got)
	}
	orphan := 0
	for i, line := range lines {
		if !strings.Contains(line, "errcode.Respond(c, errcode.InvalidInput.WithErr(err))") {
			continue
		}
		if i == 0 || !strings.Contains(lines[i-1], "ShouldBindJSON") {
			orphan++
		}
	}
	if orphan != 0 {
		t.Errorf("有 %d 处把非绑定失败的错误压成通用 400（该走 respondFilterErr）", orphan)
	}
}
