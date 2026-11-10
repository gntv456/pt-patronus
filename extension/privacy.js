// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 隐私模式（对标竞品「截图隐藏敏感数据」）：
// 开关存 chrome.storage.local.privacyMode；mask* 工具按开关打码指纹/魔力/域名。
// 值本身不删——只是展示层打码（前 4 位 + ***），导出/比对逻辑不受影响。

export async function privacyOn() {
  const { privacyMode } = await chrome.storage.local.get({ privacyMode: false })
  return !!privacyMode
}

// maskSecret 指纹类：d92777ecd423 → d927••••（保前 4 位可对比，尾部隐去）。
export function maskSecret(s) {
  const v = String(s || '')
  return v.length <= 4 ? '••••' : `${v.slice(0, 4)}••••`
}

// maskBonus 魔力/大数据：19266万 → 1.9万•（保留量级，隐去精确值）。
export function maskNumber(v) {
  return `${String(v).slice(0, 2)}••`
}

// maskDomain 域名：si-qi.xyz → s•-•i.•••（防截图暴露站点身份）。
export function maskDomain(d) {
  const v = String(d || '')
  if (!v || v === '—') return v
  const keep = v.slice(0, 2)
  return `${keep}${'•'.repeat(Math.min(v.length - 2, 6))}`
}
