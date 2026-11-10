// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 容器「创建（并按勾选自动启动）」结果的反馈口径。
// 两个创建入口（简易表单 DockerCreateDialog / 向导 DockerCreateWizard）共用这一份判定——
// 各写一遍必然漂移：一处提醒了「没起来」、另一处还一律报创建成功。
export type CreateAck = {
  ok: boolean
  id?: string
  error?: string
  started?: boolean
  start_error?: string
}

export type CreateNotice = { kind: 'success' | 'warning' | 'error'; msg: string; close: boolean }

export function createNotice(
  res: CreateAck,
  t: (key: string, params?: Record<string, unknown>) => string,
): CreateNotice {
  if (!res.ok) return { kind: 'error', msg: res.error || t('dw.createFailed'), close: false }
  // 只有后端明确说「没起来」才提醒；started 缺失=没勾自动启动，不是失败。
  if (res.started === false) {
    return { kind: 'warning', msg: t('dw.createdNotStarted', { reason: res.start_error || '' }), close: true }
  }
  return { kind: 'success', msg: t('dw.created'), close: true }
}
