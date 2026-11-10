// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 一份坏定义不许带走整个外部目录（真机实测：一个 YAML 结构写错，日志只说
// "load external site definitions failed; using bundled definitions"，
// 于是同目录其余几十个自定义站点全部退回内置，而诊断账一条不记——
// 界面（#331 Web / #333 移动端）显示「没有任何配置问题」。
// 现在这一份跳过并记 load_failed，其余照常加载。
func TestLoadDefinitionsSkipsBrokenFileAndRecordsDiagnostic(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk("good.yaml", "id: goodone\nname: Good One\nschema: generic\n")
	mk("oksecond.yaml", "id: secondone\nname: Second One\nschema: generic\n")
	// urls 在结构体里不是 map（形状照抄真机里把整目录打回内置的那份 fixture）
	mk("broken.yaml", "id: brokensite\nname: Broken\nschema: nexusphp\nunknown_top_key_probe: 1\nurls:\n    base: \"https://broken.invalid\"\n")

	r := NewRegistry()
	if err := r.LoadDefinitions(dir); err != nil {
		t.Fatalf("一份文件坏掉不应让整个目录失败：%v", err)
	}

	// 其余两份必须仍然加载进来（旧行为：return err ⇒ 这三份一起蒸发）
	ids := map[string]bool{}
	for _, d := range r.Definitions() {
		ids[d.ID] = true
	}
	if !ids["goodone"] || !ids["secondone"] {
		t.Errorf("其余定义被一份坏文件带走了：%v", ids)
	}
	if ids["brokensite"] {
		t.Error("解析失败那份不该被加载")
	}

	// 账面必须恰好记一条 load_failed 指向坏文件
	var got []DefinitionDiagnostic
	for _, d := range r.Diagnostics() {
		if d.Kind == DiagLoadFailed {
			got = append(got, d)
		}
	}
	if len(got) != 1 {
		t.Fatalf("load_failed 账面应为 1 条，实得 %d（全部诊断：%+v）", len(got), r.Diagnostics())
	}
	if !strings.Contains(got[0].File, "broken.yaml") {
		t.Errorf("诊断没指向坏文件：%+v", got[0])
	}
	if !strings.Contains(got[0].Detail, "解析失败") {
		t.Errorf("诊断原因不该是光秃秃的一句：%q", got[0].Detail)
	}

	// 基数护栏：这一步只应新增这一类账面；若把未知键等重复计一遍，界面数字会撒谎
	total := len(r.Diagnostics())
	if total != 1 {
		t.Errorf("诊断总数应为 1，实得 %d：%+v", total, r.Diagnostics())
	}
}
