// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"strings"
	"testing"
)

// TestAgentUpgradeCommandRequiresChecksum 服务端不下发无校验的自更新。
// agent 侧本来就会拒绝空校验值，但那边只会回一句「缺少 sha256 校验值」——
// 用户看不出这是服务端少配了环境变量、更不知道该配什么；提前拒绝才能给出可操作原因。
func TestAgentUpgradeCommandRequiresChecksum(t *testing.T) {
	t.Setenv("AGENT_CHECKSUM", "  ")
	if _, err := agentUpgradeCommand("https://example/agent-{arch}"); err == nil {
		t.Fatal("未配置 AGENT_CHECKSUM 时应拒绝下发")
	} else if !strings.Contains(err.Error(), "AGENT_CHECKSUM") {
		t.Errorf("原因必须点名要配哪个环境变量，got %q", err)
	}

	t.Setenv("AGENT_CHECKSUM", " amd64=aaaa; arm64=bbbb ")
	cmd, err := agentUpgradeCommand("https://example/agent-{arch}")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Op != "upgrade" || cmd.Dst != "https://example/agent-{arch}" {
		t.Errorf("命令组装不对：%+v", cmd)
	}
	if cmd.Checksum != "amd64=aaaa; arm64=bbbb" {
		t.Errorf("校验值应原样透传给 agent 按架构自选，got %q", cmd.Checksum)
	}
}
