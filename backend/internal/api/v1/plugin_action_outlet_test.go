// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// #287 插件域三个出口（保存设置 / 调用插件方法 / 建分身）从前把一切都回成
// 400「请求参数有误，请检查后重试」：插件自己跑挂了、插件没启用、单实例插件不让建分身，
// 在界面上是同一句话，而这个表单没有可检查的输入。

func respondWith(t *testing.T, e *errcode.Error) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	errcode.Respond(c, e)
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	return rec.Code, env
}

func TestPluginActionErrorKeepsKnownBuckets(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		code    string
		status  int
		wantsub string
	}{
		{"插件没启用", plugins.ErrPluginDisabled, errcode.PluginDisabled.Code, http.StatusConflict, "启用"},
		{"本机没装这个插件", plugins.ErrPluginNotInstalled, errcode.PluginNotInstalled.Code, http.StatusNotFound, "已卸载"},
		{"设置读不出来", plugins.ErrPluginConfigUnreadable, errcode.PluginConfigUnreadable.Code, http.StatusConflict, "重新保存"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, env := respondWith(t, pluginActionErrorWith(tc.err, errcode.InvalidInput))
			if status != tc.status || env["code"] != tc.code {
				t.Fatalf("status=%d code=%v，期望 %d + %q", status, env["code"], tc.status, tc.code)
			}
			if msg, _ := env["message"].(string); !strings.Contains(msg, tc.wantsub) {
				t.Errorf("界面读不到下一步：%q", msg)
			}
		})
	}
}

// 兜底按出口分开：保存设置仍算 400（多半是配置值不合插件心意），
// 但「调用插件方法」是插件自己的故障 ⇒ 502，且不许出现「参数有误」。
func TestPluginFallbackDiffersByOutlet(t *testing.T) {
	scriptFail := errors.New("plugin raised: division by zero")

	_, env := respondWith(t, pluginActionErrorWith(scriptFail, errcode.PluginActionFailed))
	if env["code"] != errcode.PluginActionFailed.Code {
		t.Fatalf("Bridge 的未知失败应算插件故障：整包=%v", env)
	}
	msg, _ := env["message"].(string)
	if strings.Contains(msg, "参数有误") {
		t.Errorf("仍在让人检查输入：%q", msg)
	}
	if strings.Contains(msg, "division by zero") {
		t.Errorf("插件原文不该进界面（只进日志）：%q", msg)
	}

	status, env := respondWith(t, pluginActionErrorWith(scriptFail, errcode.InvalidInput))
	if status != http.StatusBadRequest || env["code"] != errcode.InvalidInput.Code {
		t.Fatalf("SaveConfig 的未知失败仍是 400：status=%d 整包=%v", status, env)
	}
}

// 建分身：三档各归各码（判据集中在 instanceErr 一处）。
func TestCreateInstanceSingleInstanceIsActionable(t *testing.T) {
	single := fmt.Errorf("%w：插件 fluxtorrent 为单实例插件，不支持分身", plugins.ErrSingleInstance)
	status, env := respondWith(t, instanceErr(single))
	if status != http.StatusConflict || env["code"] != errcode.PluginSingleInst.Code {
		t.Fatalf("status=%d 整包=%v，期望 409 + %q", status, env, errcode.PluginSingleInst.Code)
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "主实例") {
		t.Errorf("要给出可执行的下一步：%q", msg)
	}

	// 依赖没装配是配置问题（503），不是「参数有误」也不是「写失败」。
	status, env = respondWith(t, instanceErr(plugins.ErrMarketOperUnavailable))
	if status != http.StatusServiceUnavailable || env["code"] != errcode.DependencyMissing.Code {
		t.Fatalf("status=%d 整包=%v，期望 503 + %q", status, env, errcode.DependencyMissing.Code)
	}

	// 其它（写库失败等）落到 save_failed，不许再伪装成参数问题。
	status, env = respondWith(t, instanceErr(errors.New("no such table: plugin_instances")))
	if status != http.StatusInternalServerError || env["code"] != errcode.SaveFailed.Code {
		t.Fatalf("status=%d 整包=%v，期望 500 + %q", status, env, errcode.SaveFailed.Code)
	}
}

// 结构护栏：三个出口真的换成了分类器（helper 存在 ≠ 有人调用）。
func TestPluginOutletsAllClassified(t *testing.T) {
	code := func(path string) string {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读不到 %s: %v", path, err)
		}
		var b strings.Builder
		for _, line := range strings.Split(string(src), "\n") {
			if i := strings.Index(line, "//"); i >= 0 {
				line = line[:i]
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		return b.String()
	}
	p := code("plugin.go")
	if !strings.Contains(p, "pluginActionErrorWith(err, errcode.PluginActionFailed)") {
		t.Error("Bridge 没接上插件故障这一档")
	}
	if !strings.Contains(p, "p, err := h.manager.SaveConfig(c.Param(\"id\"), in.Config)") ||
		strings.Contains(afterLine(p, "h.manager.SaveConfig("), "errcode.InvalidInput.WithErr(err)") {
		t.Error("SaveConfig 仍把失败压成通用 400")
	}
	m := code("plugin_market.go")
	if !strings.Contains(m, "errors.Is(err, plugins.ErrSingleInstance)") {
		t.Error("CreatePluginInstance 没认单实例这一档")
	}
}

func afterLine(code, marker string) string {
	i := strings.Index(code, marker)
	if i < 0 {
		return ""
	}
	end := i + 300
	if end > len(code) {
		end = len(code)
	}
	return code[i:end]
}
