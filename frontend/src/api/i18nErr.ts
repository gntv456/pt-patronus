// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// api 层错误文案 i18n 化（收尾 .vue 清零后的最后一环）：
// src/api/*.ts 里 throw new Error('中文兜底') 的消息在英文界面下仍是中文——
// 数据层不该 import 组件上下文，这里给一个可静态导入的轻量 t()：
// 直接读全局 i18n 实例（main.ts 先于任何 API 调用装配），未命中回退中文原文。
import { i18n } from '@/i18n'

export function terr(key: string, fallback: string): string {
  const t = i18n.global.t as (k: string) => string
  try {
    const v = t(key)
    return v === key ? fallback : v
  } catch {
    return fallback
  }
}
