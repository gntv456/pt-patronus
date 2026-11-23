// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 登录成功后回跳原页：http.ts 会话过期时发出 /login?redirect=<原路径>，
// 但 Login.vue 此前固定跳 dashboard——「登录后回原页」的契约断了。
// redirect 来自 URL query（用户可控），必须校验为站内相对路径，防开放重定向。

const DEFAULT_REDIRECT = "/dashboard"

/**
 * 把外部传入的 redirect 收敛为安全的站内路径。
 * 只接受以单个 / 开头、无协议/反斜杠、非登录页自身的相对路径。
 * 非法输入一律回退首页——宁可跳错页也不开放重定向。
 */
export function safeRedirect(raw: unknown): string {
  if (typeof raw !== "string" || !raw.startsWith("/")) return DEFAULT_REDIRECT
  // //host 是协议相对 URL，会被浏览器当成跳转到外部主机。
  if (raw.startsWith("//")) return DEFAULT_REDIRECT
  // 反斜杠在部分浏览器/框架里等价于 /，可绕过 // 检查。
  if (raw.includes("\\")) return DEFAULT_REDIRECT
  // 绝对 URL / 协议注入。
  if (raw.includes("://")) return DEFAULT_REDIRECT
  // 跳回登录页会造成循环（登录后又回登录）。
  if (
    raw === "/login" ||
    raw.startsWith("/login?") ||
    raw.startsWith("/login#")
  ) {
    return DEFAULT_REDIRECT
  }
  return raw
}
