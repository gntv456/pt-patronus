// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Assistant.vue 配置面板的两处「读不出来」判定：抽成纯函数，好让分支能被断言
// （面板本身要真挂载才碰得到，jsdom 里点不进 el-dialog）。
// 文案与仓内 composable 一致，直接中文。

// 工具目录读失败时锁死「启用全部工具」开关：勾选清单的初值取自这份目录，
// 空目录会让「自选」被存成 enabled_tools=[]，而后端把空清单解释成全开
// （aiagent/tools.go: toolAllowed 空 whitelist ⇒ true）——一次读取失败被放大成权限变更。
export function allowAllToggleBlocked(toolsError: string): boolean {
  return toolsError !== ''
}

export function toolsReadFailText(toolsError: string, normalHint: string): string {
  if (!toolsError) return normalHint
  return `工具目录读不出来：${toolsError}——此时不能改「启用全部工具」，关掉它会把白名单存成空，而空清单在服务器上等于「全部允许」`
}

// 配置本体读不出来时绝不带着默认值打开面板：面板是整份 PUT，点一次「保存」就把真配置覆盖成默认值。
export function cfgReadFailText(reason: string): string {
  return `配置读不出来：${reason || '未知原因'}（设置面板未打开，避免用默认值覆盖真实配置）`
}
