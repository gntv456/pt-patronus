// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect } from "vitest"
import { safeRedirect } from "./safeRedirect"

describe("safeRedirect", () => {
  it("合法站内路径原样返回", () => {
    expect(safeRedirect("/sites")).toBe("/sites")
    const deep = "/site-detail/42?tab=hr"
    expect(safeRedirect(deep)).toBe(deep)
  })
  it("无 redirect 或非字符串回退首页", () => {
    expect(safeRedirect(undefined)).toBe("/dashboard")
    expect(safeRedirect("")).toBe("/dashboard")
    expect(safeRedirect(123)).toBe("/dashboard")
  })
  it("拒绝开放重定向载体", () => {
    expect(safeRedirect("//evil.com")).toBe("/dashboard")
    expect(safeRedirect("https://evil.com")).toBe("/dashboard")
    expect(safeRedirect("\\\\evil.com")).toBe("/dashboard")
    expect(safeRedirect("/\\evil.com")).toBe("/dashboard")
  })
  it("拒绝回跳登录页造成循环", () => {
    expect(safeRedirect("/login")).toBe("/dashboard")
    expect(safeRedirect("/login?redirect=/sites")).toBe("/dashboard")
  })
})
