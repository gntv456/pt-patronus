// 种子页三个注入 UI 的槽位不重叠（node:test）。
// 跑法：node --test extension/test/torrent_ui_slot.test.mjs
//
// #359 content_torrent.js 自己的三层也会互相压住（#358 的同族）。
//
// ensureAll() 里 ensureDetailButton() 与 ensureListBar() 是**相邻无条件调用**，
// 两者判据不同（findDownloadURL / findListURLs）但可以同时成立——PT 站详情页
// 本来就既有主下载链接、又有侧栏推荐种子。于是三层同屏：
//
//	批量栏  bottom:18px  高约 35px  ⇒ 18–53px
//	菜单    bottom:64px  3 个按钮 + gap + padding ≈ 118px ⇒ 64–182px
//	浮钮    bottom:76px  高约 35px  ⇒ 76–111px   ← 被菜单整个盖住
//
// 三者 zIndex 全是 2147483646，叠放顺序取决于创建顺序（浮钮先建），
// 于是菜单在后 → 菜单压住浮钮。而菜单是要靠点浮钮之外的地方才收起的，
// 浮钮被盖住就点不到了。
//
// 修法：与 #358 同一套槽位带，把浮钮排到菜单之上。

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const src = readFileSync(join(here, '..', 'content_torrent.js'), 'utf8')

// 从 CSS 字面量里解析某个 id 的 bottom
function bottomOf(id) {
  const at = src.indexOf(`id = '${id}'`)
  assert.ok(at > 0, `找不到 ${id}`)
  const tail = src.slice(at, at + 400)
  const m = tail.match(/bottom: '(\d+)px'/)
  assert.ok(m, `${id} 没有 bottom（判据没落点）`)
  return Number(m[1])
}

test('三个 UI 同屏是常态（两个 ensure 相邻无条件调用）', () => {
  const at = src.indexOf('function ensureAll(')
  const tail = src.slice(at, at + 300)
  assert.match(tail, /ensureDetailButton\(\)/, 'ensureAll 里应有浮钮')
  assert.match(tail, /ensureListBar\(\)/, 'ensureAll 里应有批量栏')
  // 相邻、无互斥条件 ⇒ 同屏
  const iBtn = tail.indexOf('ensureDetailButton()')
  const iBar = tail.indexOf('ensureListBar()')
  assert.ok(iBtn >= 0 && iBar > iBtn, '两者应当相邻调用（不同判据可同时成立）')
})

test('浮钮必须排在菜单之上（菜单展开时不该盖住它）', () => {
  const menuBottom = bottomOf('ptp-listmenu')
  const floatBottom = bottomOf('ptp-float')
  // 菜单高约 118px；浮钮底边必须高于菜单顶边。
  const menuTop = menuBottom + 118
  assert.ok(
    floatBottom >= menuTop,
    `浮钮 bottom=${floatBottom}px 未高于菜单顶边（约 ${menuTop}px）：菜单展开会盖住浮钮`,
  )
})

test('浮钮的 zIndex 要严格高于批量栏与菜单（层级明确，不靠创建顺序）', () => {
  const at = src.indexOf(`id = 'ptp-float'`)
  const tail = src.slice(at, at + 300)
  const m = tail.match(/zIndex: (\d+)/)
  assert.ok(m, '浮钮缺 zIndex（判据没落点）')
  const floatZ = Number(m[1])
  const others = ['ptp-listfab', 'ptp-listmenu'].map((id) => {
    const a = src.indexOf(`id = '${id}'`)
    const z = src.slice(a, a + 300).match(/zIndex: (\d+)/)
    return z ? Number(z[1]) : 0
  })
  for (const z of others) {
    assert.ok(floatZ > z, `浮钮 zIndex（${floatZ}）须严格高于其它层（${z}）`)
  }
})
