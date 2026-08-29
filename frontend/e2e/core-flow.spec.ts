// 核心链路 E2E：登录 → 站点列表 → 跨站搜索 → 下发下载器。
// 全链路离线：
//   - 登录走后端 HXPT_EMERGENCY_ADMIN 应急通道（不触真实好学站）
//   - qBittorrent 由 e2e/mock-qb.mjs 模拟（创建/测连/下发/任务列表全真实路径）
//   - 搜索对无 cookie 站点走后端聚合容错路径（SSE 完成 + 失败站提示 = 链路通）
import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const dataDir = process.env.E2E_DATA_DIR || join(here, '.data')

// 后端起在同一进程外的临时库；从库里拿不到 token，改为 UI 登录后从 localStorage 读。
async function pageToken(page: import('@playwright/test').Page): Promise<string> {
  return page.evaluate(() => localStorage.getItem('token') || '')
}

test.describe.serial('核心链路', () => {
  let siteName = ''

  test('站点列表：添加站点', async ({ page }) => {
    await page.goto('/sites')
    await expect(page.getByRole('button', { name: '+ 添加站点' })).toBeVisible()
    await page.getByRole('button', { name: '+ 添加站点' }).click()
    siteName = 'E2E测试站'
    await page.getByPlaceholder('如：好学').fill(siteName)
    // 地址输入：选填，直接填 mock 域（真实连通性由搜索聚合容错路径验证）
    await page.getByPlaceholder(/http/).first().fill('https://e2e-mock.example.org')
    await page.getByRole('button', { name: '添加', exact: true }).click()
    // 列表应出现新站点（等待创建请求完成）
    await expect(page.getByText(siteName).first()).toBeVisible({ timeout: 15_000 })
  })

  test('跨站搜索：发起搜索并完成 SSE 轮次', async ({ page }) => {
    await page.goto('/search')
    await page.getByPlaceholder(/跨启用站点搜索/).fill('ubuntu')
    // 有 1 个启用站点（无有效 cookie）→ 后端聚合返回该站失败组，SSE 正常 done。
    // 断言链路打通：要么结果空态，要么失败站提示，绝不悬挂 loading。
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(
      page.getByText(/没有找到相关种子|个站点搜索失败|筛选后无结果/).first(),
    ).toBeVisible({ timeout: 30_000 })
  })

  test('下发下载器：创建 qB → 测连 → 推送种子 → 任务出现', async ({ page, request }) => {
    // 清理历史运行残留的同名下载器（保证卡片/推送 id 一一对应）
    await page.goto('/downloaders')
    const auth = { headers: { Authorization: `Bearer ${await pageToken(page)}` } }
    const existing = await (await request.get('/api/v1/downloaders', auth)).json()
    for (const d of existing.downloaders ?? []) {
      if (d.name === 'E2E qB') await request.delete(`/api/v1/downloaders/${d.id}`, auth)
    }
    await page.reload()
    // 1) UI 创建下载器（指向 mock qB）
    await page.getByRole('button', { name: '添加下载器' }).first().click()
    const dlg = page.getByRole('dialog')
    await dlg.getByPlaceholder('例如：家庭 qB').fill('E2E qB')
    await dlg.getByPlaceholder('http://127.0.0.1:8080').fill('http://127.0.0.1:8099')
    await dlg.getByRole('button', { name: '添加下载器' }).click()
    await expect(page.getByText('E2E qB').first()).toBeVisible({ timeout: 15_000 })

    // 2) UI 测试连接（真实走后端 → mock qB 认证链）
    await page.getByRole('button', { name: '测试连接' }).first().click()
    await expect(page.locator('.connection-status.connected').first()).toBeVisible({ timeout: 15_000 })

    // 3) 下发种子：取新建下载器的 id，走应用自身 API（与扩展/搜索页推送同一条后端链路）
    const list2 = await (await request.get('/api/v1/downloaders', auth)).json()
    const dlId = (list2.downloaders ?? []).find((d: { name: string }) => d.name === 'E2E qB')?.id
    expect(dlId).toBeTruthy()
    const add = await request.post(`/api/v1/downloaders/${dlId}/torrents`, {
      headers: { Authorization: `Bearer ${await pageToken(page)}` },
      data: { url: 'magnet:?xt=urn:btih:e2e0000000000000000000000000000000000000', tags: ['e2e'] },
    })
    expect(add.status()).toBe(200)
    expect((await add.json()).ok).toBe(true)

    // 4) 打开任务抽屉（WS 实时同步 mock qB 快照，广播间隔 2s），断言种子出现
    await page.getByRole('button', { name: '打开任务' }).first().click()
    // mock qB 跨运行累积种子，任务名带自增序号 → 用正则匹配最新一条，避免子串歧义
    await expect(page.getByText(/^E2E Mock Torrent #\d+/).first()).toBeVisible({ timeout: 30_000 })
  })
})
