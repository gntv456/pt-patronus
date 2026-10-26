// 批量推送失败必须带出原因（node:test）。
// 跑法：node --test extension/test/push_batch_reason.test.mjs
//
// #60 待扫第③条（collection.js/批量匹配）的后半。#362 修了**批量收藏**：
// 失败单独计数、把后台给的真实原因带出来（`✅ 已收藏 N / ⚠ 失败 M：原因`）。
// 但同一份菜单里 25 行之下的**批量推送**还是旧形状：
//
//	  for (const u of listUrls) {
//	    const r = await chrome.runtime.sendMessage({ type: 'ptp-push-download', url: u })
//	    if (r && r.ok) ok++
//	    else fail++
//	  }
//	  b.textContent = `✅ ${ok}${fail ? ` / ⚠ ${fail}` : ''}`
//
// pushDownload 出口是好的：它内部 try/catch 兜住一切并回 {ok:false, error:原因}，
// 原因来自后端且相当具体（「这台设备还没有凭据库」「下载器暂时连不上」「请先登录」
// 这一类）。**页面拿到了却丢掉** —— 用户只看到「⚠ 3」，无从判断该重试、该登录，
// 还是该去设置页，于是只会反复点。
//
// 与 #362 是同一处 UI 的两个出口：一个披露原因、一个不披露。这不是两种设计取舍，
// 是 #362 修漏了隔壁那一个。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const ct = readFileSync(join(here, '..', 'content_torrent.js'), 'utf8')

// menuItem 取某个 mk(...) 菜单项的函数体（到下一个 mk( 为止）。
function menuItem(marker) {
  const at = ct.indexOf(marker)
  assert.ok(at > 0, `找不到菜单项 ${marker}（判据没有落点）`)
  const end = ct.indexOf('mk(', at + 5)
  return ct.slice(at, end > 0 ? end : at + 1500)
}

test('批量推送失败必须带出原因，不许只报计数', () => {
  const tail = menuItem("mk('⬇ 全部推送下载'")
  // 失败要计数（这条本来就有，防回归）。
  assert.ok(/fail/.test(tail), '批量推送必须统计失败数')
  // 关键缺口：原因必须被留下来并显示出来。
  assert.ok(
    /\.error|err\.message|原因|reason/.test(tail),
    '批量推送必须带出失败原因，不许只报计数（用户无从判断该不该重试、该登录还是该去设置页）',
  )
})

test('两个批量出口的原因披露标准必须一致（不许一个报、一个不报）', () => {
  const push = menuItem("mk('⬇ 全部推送下载'")
  const fav = menuItem("mk('⭐ 收藏本页种子'")
  const hasReason = (s) => /\.error|err\.message|原因|reason/.test(s)
  assert.ok(
    hasReason(push) === hasReason(fav),
    '批量推送与批量收藏的原因披露不一致：' +
    `push=${hasReason(push)} fav=${hasReason(fav)}（#362 修了收藏漏了推送，同一份菜单不该两种标准）`,
  )
})

test('后台的推送出口确实会回真实原因（这条判据才有意义）', () => {
  const bg = readFileSync(join(here, '..', 'background.js'), 'utf8')
  const at = bg.indexOf('async function pushDownload')
  assert.ok(at > 0, '找不到 pushDownload')
  const tail = bg.slice(at, at + 1800)
  // 失败分支必须带 error 出去，否则页面无从披露。
  assert.ok(
    /error:\s*(e\.message|String\(e\)|'[^']+')/.test(tail),
    'pushDownload 的失败出口必须回真实原因（页面侧才有东西可显示）',
  )
  // 且必须被 .then(sendResponse) 送出，而不是只挂在成功分支。
  const routeAt = bg.indexOf("case 'ptp-push-download'")
  assert.ok(routeAt > 0, '找不到 ptp-push-download 路由')
  const route = bg.slice(routeAt, routeAt + 120)
  assert.match(route, /pushDownload\(msg\.url\)\s*\n?\s*\.then\(sendResponse\)/, '推送出口必须把结果送回页面')
})
