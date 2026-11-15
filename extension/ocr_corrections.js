// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// OCR 纠错词表：服务端识别结果 → 正确文本的持久映射。
// 动机（借鉴 MoviePilot-Tools 的 corrections 设计）：同一站点验证码的字形是固定的，
// 服务端 OCR 对某几处字形总有稳定误读（0↔O、8↔B、1↔I 这类）；用户在设置页纠正一次，
// 之后该字形组合全站长期自动纠正。词表是正交增强层——叠在服务端 OCR 之上，
// 不改变识别链路本身（服务端没配/没认出时照旧人工输入兜底）。
// 存 chrome.storage.local.ptp_ocr_corrections（{ 误读: 正确 }）。

const KEY = 'ptp_ocr_corrections'

// sanitizeCaptchaText 统一为 PT 验证码字形集：大写字母 + 数字。
// 写入词表与查询词表必须走同一规范，否则映射永远查不中；
// 服务端返回的多余字符（空格/符号）也在这里被剔掉。
export function sanitizeCaptchaText(raw) {
  return String(raw || '').toUpperCase().replace(/[^A-Z0-9]/g, '')
}

export async function loadCorrections() {
  try {
    const v = await chrome.storage.local.get(KEY)
    const m = v && v[KEY]
    if (!m || typeof m !== 'object' || Array.isArray(m)) return {}
    return Object.fromEntries(
      Object.entries(m).filter(([k, val]) => k && typeof val === 'string'),
    )
  } catch {
    return {}
  }
}

export async function saveCorrections(map) {
  await chrome.storage.local.set({ [KEY]: map || {} })
}

// upsertCorrection 记一条 wrong→right；right 为空或与 wrong 相同 → 删除该映射。
// 返回动作（added/updated/removed），设置页据此提示「新增/更新/移除」。
export async function upsertCorrection(wrong, right) {
  const w = sanitizeCaptchaText(wrong)
  const r = sanitizeCaptchaText(right)
  if (!w) throw new Error('识别结果不能为空')
  const map = await loadCorrections()
  if (!r || w === r) {
    if (!(w in map)) return 'removed'
    delete map[w]
    await saveCorrections(map)
    return 'removed'
  }
  const existed = w in map
  map[w] = r
  await saveCorrections(map)
  return existed ? 'updated' : 'added'
}

// removeCorrection 删除一条映射（设置页列表点击）；不存在返回 false。
export async function removeCorrection(wrong) {
  const w = sanitizeCaptchaText(wrong)
  if (!w) return false
  const map = await loadCorrections()
  if (!(w in map)) return false
  delete map[w]
  await saveCorrections(map)
  return true
}

// applyCorrections 子串替换，先长后短：长键（AB→X）先于短键（A→Y）执行，
// 避免短键先把长键拆散导致无法命中。
export function applyCorrections(raw, map) {
  let out = String(raw || '')
  const keys = Object.keys(map || {}).filter(Boolean).sort((a, b) => b.length - a.length)
  for (const k of keys) {
    const v = map[k]
    if (v == null) continue
    out = out.split(k).join(v)
  }
  return out
}

// correctOcrText background 应用入口：读词表 → 规范化 → 替换。
// 词表为空时原样透传——未使用该功能的用户行为与从前完全一致（不白白改写识别结果）。
export async function correctOcrText(raw) {
  const orig = String(raw || '')
  const map = await loadCorrections()
  if (!Object.keys(map).length) return orig
  const s = sanitizeCaptchaText(orig)
  if (!s) return orig
  return applyCorrections(s, map)
}
