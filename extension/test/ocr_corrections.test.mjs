// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// OCR 纠错词表行为测试（node:test；chrome.storage 用内存 Map stub）。
// 跑法：node --test extension/test/ocr_corrections.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'

const memStore = new Map()
globalThis.chrome = {
  storage: {
    local: {
      async get(k) {
        const keys = Array.isArray(k) ? k : [k]
        const out = {}
        for (const key of keys) if (memStore.has(key)) out[key] = memStore.get(key)
        return out
      },
      async set(obj) { for (const [k, v] of Object.entries(obj)) memStore.set(k, v) },
    },
  },
}

const oc = await import('../ocr_corrections.js')

test('sanitizeCaptchaText 统一大写并剔除非字母数字', () => {
  assert.equal(oc.sanitizeCaptchaText('ab c8'), 'ABC8')
  assert.equal(oc.sanitizeCaptchaText('O0!i'), 'O0I')
  assert.equal(oc.sanitizeCaptchaText(''), '')
  assert.equal(oc.sanitizeCaptchaText(null), '')
})

test('upsertCorrection 新增/更新/删除三态', async () => {
  assert.equal(await oc.upsertCorrection('08o', '0BQ'), 'added')
  assert.equal(await oc.upsertCorrection('08O', '0BD'), 'updated')
  assert.equal(await oc.upsertCorrection('08o', '08o'), 'removed') // 相同 → 删除
  assert.equal(await oc.upsertCorrection('08O', ''), 'removed') // 右值为空 → 删除
  assert.deepEqual(await oc.loadCorrections(), {})
})

test('applyCorrections 先长后短（长键不被短键拆散）', () => {
  // A→X 先执行会把 AB 拆成 XB，AB→Y 永远命中不了；先长后短应得 Y。
  assert.equal(oc.applyCorrections('AB', { AB: 'Y', A: 'X' }), 'Y')
  assert.equal(oc.applyCorrections('08O', { '08O': '0BQ' }), '0BQ')
  assert.equal(oc.applyCorrections('abc', {}), 'abc')
})

test('correctOcrText 无词表透传（不白白改写识别结果）', async () => {
  assert.equal(await oc.correctOcrText('a b C'), 'a b C')
})

test('correctOcrText 有词表：规范化 + 替换', async () => {
  await oc.upsertCorrection('08O', '0BQ')
  assert.equal(await oc.correctOcrText('08o'), '0BQ')
  assert.equal(await oc.correctOcrText(' 08O '), '0BQ') // 多余字符被规范掉
  assert.equal(await oc.correctOcrText(''), '') // 空识别结果不动
})

test('removeCorrection 删指定条目', async () => {
  await oc.upsertCorrection('l1', 'I1')
  assert.equal(await oc.removeCorrection('L1'), true)
  assert.equal(await oc.removeCorrection('L1'), false)
  assert.equal(await oc.removeCorrection(''), false)
})

test('词表损坏（数组/垃圾）时 loadCorrections 容错回空表', async () => {
  memStore.set('ptp_ocr_corrections', ['bad'])
  assert.deepEqual(await oc.loadCorrections(), {})
  memStore.set('ptp_ocr_corrections', { GOOD: 'ok', BAD: 123, '': 'x' })
  assert.deepEqual(await oc.loadCorrections(), { GOOD: 'ok' }) // 值非字符串/键空被滤
})
