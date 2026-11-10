// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"errors"
	"strings"
	"testing"
)

// #310 监控目标校验的中文原因必须有「写给人看」的载体。
// validateTargetInput / validateRTSPScheme 此前一律返回裸 fmt.Errorf：
// InputReasonErr 认不出、errors.Is(ErrInvalidInput) 也不成立 ⇒
// handler 落到 fallback（errcode.SaveFailed = HTTP 500「保存失败」），
// 于是「脚本监控没填命令」这种纯输入错被报成服务器内部错误，
// 界面只剩「保存失败：保存失败」，真原因躺在日志里（默认级别看不见）。
// 修法：改走包内 invalidInput 载体（Unwrap 到 ErrInvalidInput），
// handler 就能出 400 + 人话；移动端 catch 直接把 userMessage 弹成 SnackBar。

func assertInputReason(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("校验必须拦住（否则界面读不到原因这条判据是空的）")
	}
	if got := InputReasonErr(err); got == nil {
		t.Errorf("校验原因要走 invalidInputError 载体；实际 InputReasonErr=nil，err=%q", err.Error())
		return
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("要能 errors.Is(ErrInvalidInput)，否则分类层判不出 400；实际 %v", err)
	}
	if msg := InputReasonErr(err).Error(); !strings.Contains(msg, want) {
		t.Errorf("原因文案：期望含 %q，实际 %q", want, msg)
	}
}

func TestValidateTargetInputCarriesHumanReason(t *testing.T) {
	cases := []struct {
		name string
		in   TargetInput
		want string
	}{
		{
			name: "脚本监控缺命令（移动端没有这项本地校验）",
			in:   TargetInput{Name: "s", Type: "script", Host: "10.0.0.8", Protocol: "ssh"},
			want: "脚本监控需要填写命令",
		},
		{
			name: "进程监控缺进程名",
			in:   TargetInput{Name: "p", Type: "process", Host: "10.0.0.8", Protocol: "ssh"},
			want: "进程监控需要填写进程名称",
		},
		{
			name: "HTTP 地址不是完整 URL",
			in:   TargetInput{Name: "h", Type: "http", Host: "10.0.0.8"},
			want: "HTTP 监控地址必须是 http/https 完整 URL",
		},
		{
			name: "TCP 端口越界",
			in:   TargetInput{Name: "t", Type: "tcp", Host: "10.0.0.8", Port: 70000},
			want: "TCP 端口必须在 1-65535 之间",
		},
		{
			name: "进程监控只支持 SSH",
			in:   TargetInput{Name: "q", Type: "process", Host: "10.0.0.8", Protocol: "api", Thresholds: `{"process_name":"nginx"}`},
			want: "进程监控仅支持 SSH 协议",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertInputReason(t, validateTargetInput(c.in), c.want)
		})
	}
}

func TestValidateRTSPSchemeCarriesHumanReason(t *testing.T) {
	assertInputReason(t, validateRTSPScheme("http://10.0.0.8/stream"), "RTSP")
	assertInputReason(t, validateRTSPScheme("rtsp://"), "主机")
}

// 正控：合法输入不许被顺手拦掉（改载体不能同时把判据放宽或收紧）。
func TestValidateTargetInputStillAcceptsValid(t *testing.T) {
	if err := validateTargetInput(TargetInput{
		Name: "ok", Type: "script", Host: "10.0.0.8", Protocol: "ssh",
		Thresholds: `{"script_command":"df -P"}`,
	}); err != nil {
		t.Errorf("合法脚本监控不该被拦：%v", err)
	}
	if err := validateTargetInput(TargetInput{Name: "n", Type: "host", Protocol: "local"}); err != nil {
		t.Errorf("本机服务器设备不该被拦：%v", err)
	}
	if err := validateRTSPScheme("rtsp://user:pass@10.0.0.8:554/stream"); err != nil {
		t.Errorf("合法 rtsp 地址不该被拦：%v", err)
	}
}
