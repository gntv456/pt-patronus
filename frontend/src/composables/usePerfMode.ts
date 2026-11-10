// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// usePerfMode — 性能模式（阶段 1）：默认关闭全站 backdrop-filter。
//
// 187 处 blur+saturate 是 GPU 合成性能杀手（见 glass.css 注释）。
// 优先级：强制降级（perf-glass-flat=1 或低端机） > 用户显式恢复（perf-glass-full=1）。
// 设置面板可调用 setGlassBlur() 开关；main.ts 挂载前调 applyGlassMode() 避免首帧闪烁。
import { readonly, ref } from 'vue'

const KEY_FULL = 'perf-glass-full'
const KEY_FLAT = 'perf-glass-flat'

function detectLowEnd(): boolean {
  if (typeof navigator === 'undefined') return false
  const nav = navigator as Navigator & { deviceMemory?: number }
  return (
    (navigator.hardwareConcurrency > 0 && navigator.hardwareConcurrency <= 4) ||
    (typeof nav.deviceMemory === 'number' && nav.deviceMemory <= 4)
  )
}

const lowEnd = detectLowEnd()
if (lowEnd && typeof localStorage !== 'undefined') localStorage.setItem(KEY_FLAT, '1')
const forcedLowEnd = ref(
  typeof localStorage !== 'undefined' && localStorage.getItem(KEY_FLAT) === '1',
)
const glassBlur = ref(!forcedLowEnd.value && typeof localStorage !== 'undefined' && localStorage.getItem(KEY_FULL) === '1')

function applyGlassMode() {
  if (typeof document === 'undefined') return
  document.body.classList.toggle('glass-reduced', !glassBlur.value)
}

function setGlassBlur(on: boolean) {
  if (forcedLowEnd.value) return // 强制降级不可被设置项覆盖
  glassBlur.value = on
  if (typeof localStorage !== 'undefined') {
    if (on) localStorage.setItem(KEY_FULL, '1')
    else localStorage.removeItem(KEY_FULL)
  }
  applyGlassMode()
}

export function usePerfMode() {
  return {
    glassBlur,
    forcedLowEnd: readonly(forcedLowEnd),
    setGlassBlur,
    applyGlassMode,
  }
}