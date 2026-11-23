// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 未保存修改守护：用户视角的数据丢失防线。
// 设置页 6+ 配置面板都是「显式保存」——改了表单没点保存，刷新/关标签页静默丢失。
// 此前只有 WorkflowEditor 有 dirtyLeave 提示，且全项目无 beforeunload。
//
// 设计要点：用「表单快照对比」而非布尔开关，避免 loadXxx() 重置表单时误判 dirty。
// watch 触发时机与 load 完成时点可能交错，快照对比保证任何顺序下结果都正确。
import { ref, watch, type Ref } from "vue"

const dirty = ref(false)
const baselines = new Map<symbol, string>()
let unloadInstalled = false

function snapshot(v: unknown): string {
  try { return JSON.stringify(v) } catch { return "" }
}

/** 追踪一个表单 ref：与最近一次 markSaved 的快照不一致时标记为未保存。 */
export function trackForm<T>(form: Ref<T>): symbol {
  const key = Symbol("form-guard")
  baselines.set(key, snapshot(form.value))
  watch(form, () => {
    dirty.value = snapshot(form.value) !== baselines.get(key)
  }, { deep: true })
  return key
}

/** 表单已落库（load 重置后或 save 成功后调用）：刷新基线并清除 dirty。 */
export function markSaved(key: symbol, form: Ref<unknown>): void {
  baselines.set(key, snapshot(form.value))
  dirty.value = false
}

/** 主动标记脏（用于无 ref 托盘的临时场景）。 */
export function markDirty(): void {
  dirty.value = true
}

/**
 * 安装浏览器原生 beforeunload 守卫（只需一次，幂等）。
 * dirty 时触发浏览器原生「离开此站点？」确认，文案由浏览器控制，无法自定义。
 * 只在有真实未保存修改时拦截，干净状态刷新不打扰用户。
 */
export function installUnloadGuard(): void {
  if (unloadInstalled || typeof window === "undefined") return
  unloadInstalled = true
  window.addEventListener("beforeunload", (e: BeforeUnloadEvent) => {
    if (!dirty.value) return
    e.preventDefault()
    e.returnValue = ""
  })
}

export function useUnsavedGuard() {
  return { dirty, trackForm, markSaved, markDirty, installUnloadGuard }
}

// 测试辅助：重置模块级状态（仅测试用）。
export function __resetGuardForTest(): void {
  dirty.value = false
  baselines.clear()
  unloadInstalled = false
}
