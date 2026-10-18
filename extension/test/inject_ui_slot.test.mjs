// 注入 UI 的占位不冲突（node:test）。
// 跑法：node --test extension/test/inject_ui_slot.test.mjs
//
// #358 凭据气泡与种子页浮钮会互相压住（两者同页共存、坐标只差 6px、同 zIndex）。
//
// background.js 的 ALL_FILES = TORRENT_FILES + LOGIN_FILES，白名单站点**两者都注入**，
// 所以同页共存是常态而不是边角：
//
//	content_torrent.js  浮钮   right:18px  bottom:76px  zIndex:2147483646
//	                   高约 35px（padding 9px + 13px 字体）⇒ 占 76–111px
//	content_creds.js   气泡   right:18px  bottom:70px  zIndex:2147483646
//	                   向上生长 ⇒ 从 70px 起覆盖，正好压住浮钮
//
// 同 zIndex 下谁在上面取决于 DOM 插入顺序，而两者是异步注入的、不确定——
// 于是表现是「有时候浮钮能点，有时候点不到」，且很难复现。
//
// 修法：把注入 UI 的右下角纵向位置排成一条不重叠的槽位带：
//	浮钮（常驻，最低）→ 气泡（临时，压在浮钮上方）
// 即气泡 bottom 要高于浮钮顶边（76+35≈111），而不是 70。

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const creds = readFileSync(join(here, '..', 'content_creds.js'), 'utf8')
const torrent = readFileSync(join(here, '..', 'content_torrent.js'), 'utf8')
const bg = readFileSync(join(here, '..', 'background.js'), 'utf8')

// 取某个脚本里所有 fixed 定位元素的 bottom 值（px）
function bottomsOf(src) {
  const out = []
  const re = /position: 'fixed'[^}]*?bottom: '(\d+)px'/g
  let m
  while ((m = re.exec(src))) out.push(Number(m[1]))
  return out
}

test('同页共存是常态：两个脚本都在 ALL_FILES 里注入', () => {
  assert.match(bg, /const TORRENT_FILES = \[[^\]]*content_torrent\.js/, '浮钮脚本未在注入列表里')
  assert.match(bg, /const LOGIN_FILES = \[[^\]]*content_creds\.js/, '凭据脚本未在注入列表里')
})

test('凭据气泡必须避开浮钮（临时 UI 压常驻 UI 会让浮钮点不到）', () => {
  const floatBottom = bottomsOf(torrent).filter((b) => b > 0 && b < 200)
  const bubbleBottom = bottomsOf(creds)
  assert.ok(floatBottom.length > 0, '没解析到浮钮 bottom（判据没落点）')
  assert.ok(bubbleBottom.length > 0, '没解析到气泡 bottom（判据没落点）')

  // 浮钮顶边（≈ bottom + 高度 35）；气泡 bottom 必须高于它，两者才不重叠。
  const floatTop = Math.max(...floatBottom) + 35
  for (const b of bubbleBottom) {
    assert.ok(
      b >= floatTop,
      `气泡 bottom=${b}px 与浮钮顶边（约 ${floatTop}px）重叠：两者同页共存，会互相压住`,
    )
  }
})

test('气泡与浮钮的 zIndex 要有明确层级（临时 UI 压在常驻之上）', () => {
  const z = (src) => (src.match(/position: 'fixed'[^}]*?zIndex: (\d+)/g) || [])
  const floatZ = Math.min(...z(torrent).map((s) => Number(s.match(/(\d+)$/)[1])))
  const bubbleZ = Math.min(...z(creds).map((s) => Number(s.match(/(\d+)$/)[1])))
  assert.ok(
    bubbleZ > floatZ,
    `气泡 zIndex（${bubbleZ}）应高于浮钮（${floatZ}）：同 zIndex 下叠放顺序取决于注入时机，不确定`,
  )
})

// 四层槽位带的完整判据：任意两层都不许重叠（前面几条只验了部分配对）。
test('四层槽位带互不重叠（批量栏 / 菜单 / 凭据气泡 / 浮钮）', () => {
  // 各层的「底边 → 顶边」区间（高度按 CSS 字面量估：批量栏/浮钮约 35px，
  // 菜单 3 按钮 + gap + padding 约 118px，气泡单行约 40px）。
  // 底边必须从源码 CSS 解析（硬编码会在有人挪位置时变成假绿——本轮就踩过）。
  const LAYERS = [
    { name: '批量栏', bottom: bottomOfId(torrent, 'ptp-listfab'), height: 35 },
    { name: '菜单', bottom: bottomOfId(torrent, 'ptp-listmenu'), height: 118 },
    { name: '凭据气泡', bottom: bottomOfCredBubble(), height: 40 },
    { name: '浮钮', bottom: bottomOfId(torrent, 'ptp-float'), height: 35 },
  ]
  const sorted = [...LAYERS].sort((a, b) => a.bottom - b.bottom)
  for (let i = 0; i < sorted.length - 1; i++) {
    const lo = sorted[i]
    const hi = sorted[i + 1]
    const loTop = lo.bottom + lo.height
    assert.ok(
      hi.bottom >= loTop,
      `${hi.name}（bottom=${hi.bottom}）压住了 ${lo.name}（顶边≈${loTop}）——同一右下角槽位带里两层重叠了`,
    )
  }
})

test('槽位带顺序要写进注释（下一层挪位置时知道上一层的顶边）', () => {
  assert.match(torrent, /槽位带/, 'content_torrent.js 缺槽位带约定注释')
  assert.match(creds, /槽位带/, 'content_creds.js 缺槽位带约定注释（否则单看这个文件会挪错位置）')
})

// 从 content_torrent.js 解析某 id 的 bottom。
function bottomOfId(src, id) {
  const at = src.indexOf(`id = '${id}'`)
  assert.ok(at > 0, `找不到 ${id}`)
  const m = src.slice(at, at + 400).match(/bottom: '(\d+)px'/)
  assert.ok(m, `${id} 没有 bottom（判据没落点）`)
  return Number(m[1])
}

// 从 content_creds.js 解析凭据气泡的 bottom（BUBBLE_SLOT 那个元素）。
function bottomOfCredBubble() {
  const at = creds.indexOf('box.id = BUBBLE_SLOT')
  assert.ok(at > 0, '找不到凭据气泡元素')
  const m = creds.slice(at, at + 400).match(/bottom: '(\d+)px'/)
  assert.ok(m, '凭据气泡没有 bottom（判据没落点）')
  return Number(m[1])
}