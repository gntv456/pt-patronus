// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #402 「这个目标不支持该操作」与「命令太长」被压进 502「上游服务不可达」。
//
// monitor/command.go 与 terminal.go 有六条哨兵，原文全是英文：
//   ErrUnsupportedCommandTarget   目标不是 host/nas（或 NAS 没配 SSH）
//   ErrUnsupportedCommandRunner   该采集器不支持执行命令
//   ErrUnsupportedTerminalTarget / ErrUnsupportedTerminalRunner  终端侧同理
//   ErrCommandTooLong             命令超长
// 它们此前无一处出口在读 —— respondOpsTargetError 只认 gorm.ErrRecordNotFound，
// 其余一律 BadGateway.WithErr ⇒ 界面显示「上游服务不可达，请稍后重试」。
//
// 为什么这一档值得分出来：它是**用户自己能改**的（换目标类型 / 先给 NAS 配 SSH /
// 命令写短点）。套「上游不可用」会把方向指向网络与 agent，用户查一圈网络也改不动。
//
// 判据走真实出口（Docker 列表 → RunCommandNow → respondOpsTargetError），
// 与 ops_error_test.go 同法；钉「其余仍 502」防止把真上游故障一起吞进来。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/monitor"
)

// TestOpsTargetUnsupportedIs400Not502 目标不支持该操作：应 400 + ops_target_unsupported。
func TestOpsTargetUnsupportedIs400Not502(t *testing.T) {
	cases := map[string]error{
		"目标类型不支持":    monitor.ErrUnsupportedCommandTarget,
		"采集器不支持命令":   monitor.ErrUnsupportedCommandRunner,
		"终端侧：目标不支持":  monitor.ErrUnsupportedTerminalTarget,
		"终端侧：采集器不支持": monitor.ErrUnsupportedTerminalRunner,
	}
	for name, err := range cases {
		h := NewDockerHandler(&mockRunner{err: err})
		w := doJSONReq(t, newDockerTestRouter(h), http.MethodGet, "/list/1", "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: 应 400（是请求选错了目标，不是上游故障），got %d body=%s", name, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "ops_target_unsupported") {
			t.Fatalf("%s: 应带 ops_target_unsupported，got %s", name, w.Body.String())
		}
	}
}

// TestOpsCommandTooLongHasOwnCode 命令超长单独一档：
// 混进「目标不支持」会让文案叫用户「换一台主机」——换了还是超长，方向全错。
func TestOpsCommandTooLongHasOwnCode(t *testing.T) {
	h := NewDockerHandler(&mockRunner{err: fmt.Errorf("%w: max 65536 bytes", monitor.ErrCommandTooLong)})
	w := doJSONReq(t, newDockerTestRouter(h), http.MethodGet, "/list/1", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("命令超长应 400，got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "ops_command_too_long") {
		t.Fatalf("应带 ops_command_too_long，got %s", body)
	}
	// 文案不许把方向指向「换主机」——那是「目标不支持」那一档的话。
	if strings.Contains(body, "换一台") {
		t.Errorf("命令超长的文案不许指向「换主机」（换了还是超长）：%s", body)
	}
	// 必须说「缩短」——这是这一档唯一能照做的动作。
	if !strings.Contains(body, "缩短") {
		t.Errorf("必须说清要缩短命令，got %s", body)
	}
}

// TestOpsOtherFailuresStill502 反向对照：真上游故障（超时 / SSH 连不上）仍必须 502。
// 这一条防的是「把 isOpsTargetUnsupported 写得太宽、把所有错误都吞进 400」。
func TestOpsOtherFailuresStill502(t *testing.T) {
	for name, err := range map[string]error{
		"超时":       context.DeadlineExceeded,
		"SSH 连不上":  errors.New("ssh: handshake failed: connection refused"),
		"认不出的下游错误": errors.New("某个第三方的怪错误"),
	} {
		h := NewDockerHandler(&mockRunner{err: err})
		w := doJSONReq(t, newDockerTestRouter(h), http.MethodGet, "/list/1", "")
		if w.Code != http.StatusBadGateway {
			t.Errorf("%s: 真上游故障仍应 502，got %d body=%s", name, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "ops_target_unsupported") ||
			strings.Contains(w.Body.String(), "ops_command_too_long") {
			t.Errorf("%s: 不该被分到 #402 那两档：%s", name, w.Body.String())
		}
	}
}

// TestOpsUnsupportedJudgeCoversAllFiveSentinals 接线护栏：六条哨兵必须全部被认。
// 少认一条，那一类失败就静默退回 502（#402 要消灭的正是这个）。
func TestOpsUnsupportedJudgeCoversAllFiveSentinals(t *testing.T) {
	all := []error{
		monitor.ErrUnsupportedCommandTarget,
		monitor.ErrUnsupportedCommandRunner,
		monitor.ErrUnsupportedTerminalTarget,
		monitor.ErrUnsupportedTerminalRunner,
	}
	for _, e := range all {
		if !isOpsTargetUnsupported(e) {
			t.Errorf("%v: 漏认了（会退回 502「上游不可用」）", e)
		}
		if isOpsCommandTooLong(e) {
			t.Errorf("%v: 不该被判成「命令超长」", e)
		}
	}
	if !isOpsCommandTooLong(monitor.ErrCommandTooLong) {
		t.Error("ErrCommandTooLong 漏认了")
	}
	if isOpsTargetUnsupported(monitor.ErrCommandTooLong) {
		t.Error("ErrCommandTooLong 不该进「目标不支持」那一档（换主机解决不了）")
	}
	// 包装过也要认（monitor 内部都是 fmt.Errorf("%w: …") 包出来的）。
	if !isOpsTargetUnsupported(fmt.Errorf("run: %w", monitor.ErrUnsupportedCommandTarget)) {
		t.Error("包装过的哨兵也必须被认")
	}
}
