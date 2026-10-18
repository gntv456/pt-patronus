package oper

import (
	"strings"
	"testing"
)

// #311 阈值 JSON 读不出来时，后端 ParseThresholds 静默回退内置默认（90/90/90/75），
// 于是「写进去一双括号打错的规则」= 用户以为设了 50%，实际按 90% 报警，且没有任何读数。
// 写入侧必须在边界拒掉（400 + 人话），解析侧要留 Warn（存量脏数据得能被看见）。
// 空串是合法值（= 用内置默认），不能顺手拦掉——Web 表单对没有自定义配置的设备就发空串。

func TestValidateTargetInputRejectsUnreadableThresholds(t *testing.T) {
	err := validateTargetInput(TargetInput{
		Name: "h", Type: "host", Protocol: "local",
		Thresholds: "{cpu_usage:50}", // 键没引号：json 非法，ParseThresholds 会静默回默认
	})
	assertInputReason(t, err, "告警阈值")

	// 合法但语义空的 JSON 仍放行（后端会按默认补齐缺省项）。
	if e := validateTargetInput(TargetInput{
		Name: "h", Type: "host", Protocol: "local", Thresholds: `{"cpu_usage":50,"offline":false}`,
	}); e != nil {
		t.Errorf("合法阈值 JSON 不该被拦：%v", e)
	}
	// 顶层不是对象（数组/裸数字）也会被 ParseThresholds Unmarshal 成错误 ⇒ 同样要拦。
	if e := validateTargetInput(TargetInput{
		Name: "h", Type: "host", Protocol: "local", Thresholds: `[{"cpu_usage":50}]`,
	}); e == nil || !strings.Contains(e.Error(), "告警阈值") {
		t.Errorf("非对象阈值要拦，实际 %v", e)
	}
}

// 正控：空串 = 交给内置默认，是设计而不是脏数据。
func TestValidateTargetInputAcceptsEmptyThresholds(t *testing.T) {
	for _, in := range []TargetInput{
		{Name: "h", Type: "host", Protocol: "local"},
		{Name: "h", Type: "host", Protocol: "local", Thresholds: ""},
		{Name: "s", Type: "script", Host: "10.0.0.8", Protocol: "ssh", Thresholds: `{"script_command":"df -P"}`},
	} {
		if err := validateTargetInput(in); err != nil {
			t.Errorf("type=%s 空/合法阈值不该被拦：%v", in.Type, err)
		}
	}
}
