// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect, beforeEach } from "vitest"
import { ref, nextTick } from "vue"
import {
  trackForm, markSaved, markDirty, isDirty,
  useUnsavedGuard, __resetGuardForTest,
} from "./useUnsavedGuard"

describe("useUnsavedGuard", () => {
  beforeEach(() => { __resetGuardForTest() })

  it("load 重置表单（markSaved）后不误报 dirty", async () => {
    const form = ref({ token: "" })
    const key = trackForm(form)
    // 模拟 loadXxx() 用权威配置回填
    form.value = { token: "saved-token" }
    markSaved(key, form)
    await nextTick()
    expect(useUnsavedGuard().dirty.value).toBe(false)
  })

  it("用户修改表单后标记 dirty", async () => {
    const form = ref({ token: "" })
    const key = trackForm(form)
    markSaved(key, form)
    form.value = { token: "user-typed" }
    await nextTick()
    expect(useUnsavedGuard().dirty.value).toBe(true)
  })

  it("保存成功（markSaved）后清除 dirty", async () => {
    const form = ref({ token: "" })
    const key = trackForm(form)
    markSaved(key, form)
    form.value = { token: "edited" }
    await nextTick()
    expect(useUnsavedGuard().dirty.value).toBe(true)
    // 模拟 save 成功后 load 回填 + markSaved
    form.value = { token: "edited" }
    markSaved(key, form)
    await nextTick()
    expect(useUnsavedGuard().dirty.value).toBe(false)
  })

  it("改回原值视为 clean（快照对比而非布尔）", async () => {
    const form = ref({ enabled: false })
    const key = trackForm(form)
    markSaved(key, form)
    form.value = { enabled: true }
    await nextTick()
    expect(useUnsavedGuard().dirty.value).toBe(true)
    form.value = { enabled: false }
    await nextTick()
    expect(useUnsavedGuard().dirty.value).toBe(false)
  })

  it("markDirty 可主动标记", () => {
    markDirty()
    expect(useUnsavedGuard().dirty.value).toBe(true)
  })
  it("按表单独立判定脏状态（A 改了不影响 B 的按钮）", async () => {
    const formA = ref({ v: "" })
    const formB = ref({ v: "" })
    const keyA = trackForm(formA)
    const keyB = trackForm(formB)
    markSaved(keyA, formA)
    markSaved(keyB, formB)
    formA.value = { v: "edited" }
    await nextTick()
    expect(isDirty(keyA)).toBe(true)
    expect(isDirty(keyB)).toBe(false)
    expect(useUnsavedGuard().dirty.value).toBe(true)
  })

})
