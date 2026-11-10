// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// #280 Trakt 面板的失败口径。
//
// 后端这次把四档分开写了人话（凭据没填去哪填 / 还没授权先点发起授权 / 授权码已过期重拿一张 /
// 其余才是上游没完成）。面板从前对每一档都只说同一句本地兜底：
//   `ElMessage.error(t('trakt.authStartFailed'))` —— catch 里拿到了 e 却一个字都没读。
// 更坏的是轮询：授权码过期后仍每 3s 打一次 /trakt/device-status，界面挂着一张废码干等。

/** 后端为这次失败写的那句话（{code, message}）；读不到才退回本地兜底文案。 */
export function traktFailText(e: unknown, fallback: string): string {
  const data = (e as { response?: { data?: { message?: string; error?: string } } })?.response?.data
  const msg = (data?.message || data?.error || '').trim()
  return msg || fallback
}

/**
 * 轮询要不要停下来。后端用 terminal 字段直说，这里再兼容 code（万一响应形状来自旧版或被网关改写）。
 * 判不准时按「继续等」处理——误停比误等多问一次更糟（用户会以为授权失败了）。
 */
export function devicePollTerminal(e: unknown): boolean {
  const data = (e as { response?: { data?: Record<string, unknown> } })?.response?.data
  if (!data) return false
  if (data.terminal === true) return true
  const code = typeof data.code === 'string' ? data.code : ''
  return ['trakt_code_expired', 'trakt_not_configured', 'trakt_not_authorized'].includes(code)
}

/** 连续这么多次「什么都读不到」才放弃轮询（一次抖动不该被说成坏了）。 */
export const TRAKT_POLL_GIVEUP = 5

/**
 * 放弃轮询的措辞：只能说「读不到状态」，不能说「授权失败」——
 * 用户在浏览器里可能已经输完码，只是这一侧的网络断了。
 */
export function traktPollGiveUpText(fails: number): string {
  return `读不到 Trakt 授权状态（已连续 ${fails} 次，可能是网络问题；若你已在浏览器完成输码，可稍后刷新本页确认）`
}
