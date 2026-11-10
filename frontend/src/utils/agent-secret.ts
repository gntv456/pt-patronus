// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { h } from 'vue'
import { ElMessageBox } from 'element-plus'
import { copyText } from './clipboard'
import type { Agent } from '@/api/monitor'

// 拼接 agent 一键安装命令（创建/重置后展示给用户复制）。
export function agentInstallCmd(agent: Agent, plainSecret: string): string {
  const wsProto = location.protocol === 'https:' ? 'wss' : 'ws'
  const installURL = `${location.origin}/api/v1/monitor/agents/install.sh`
  const wsURL = `${wsProto}://${location.host}`
  return `curl -fsSL ${installURL} | AGENT_ID=${agent.id} AGENT_SECRET='${plainSecret}' DASHBOARD_URL='${wsURL}' bash`
}

// 弹出 agent secret + 安装命令（创建/重置后调用；secret 仅显示一次，确认按钮一键复制命令）。
// 被「+ 添加设备」选 Agent 协议、AgentManager 重置 secret 两处复用。
export function showAgentSecret(agent: Agent, plainSecret: string) {
  const cmd = agentInstallCmd(agent, plainSecret)
  ElMessageBox({
    title: `Agent「${agent.name}」的 Secret`,
    type: 'warning',
    confirmButtonText: '复制安装命令',
    cancelButtonText: '关闭',
    showCancelButton: true,
    message: h('div', { style: 'line-height:1.7; font-size:13px' }, [
      h('p', { style: 'color:#e6a23c' }, 'Secret 仅显示此次，请立即保存！泄露后可在「Agent 管理」中重置。'),
      h('p', { style: 'margin-top:10px' }, 'Secret：'),
      h(
        'pre',
        {
          style:
            'background:var(--glass-bg);padding:8px;border-radius:6px;font-size:12px;word-break:break-all;white-space:pre-wrap',
        },
        plainSecret,
      ),
      h('p', { style: 'margin-top:10px' }, '一键安装命令（在被监控机 Linux / root 执行）：'),
      h(
        'pre',
        {
          style:
            'background:var(--glass-bg);padding:8px;border-radius:6px;font-size:12px;word-break:break-all;white-space:pre-wrap',
        },
        cmd,
      ),
      h(
        'p',
        { style: 'margin-top:8px;color:var(--text-muted);font-size:12px' },
        '需先在 GitHub Release 发布 agent 二进制；命令会自动下载对应架构并设为开机自启。',
      ),
    ]),
  })
    .then(() => copyText(cmd, '安装命令已复制，去被监控机粘贴执行', '复制失败，请手动选中复制'))
    .catch(() => {})
}
