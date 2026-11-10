// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 扩展链路 E2E（核心三链路之外的第二批）：订阅读删、i18n 切换持久化、登录失败/登出。
// 全链路离线（与 core-flow 同口径）：登录走本地密码（/auth/local/login）；订阅数据经应用 API 造数。
//
// 顺序约定：登录态用例（storageState）在前，登出用例最后——/auth/logout 会 bump
// TokenVersion 使用户的全部 token 失效，若先执行会让 storageState 里的会话作废。
import { expect, test } from '@playwright/test'
import { E2E_LOCAL_PASSWORD, E2E_LOCAL_USER } from '../playwright.config'

// 从已登录页读 localStorage token（core-flow 同款；本登录链路不种 cookie，
// API 调用需显式 Authorization）。
async function pageToken(page: import('@playwright/test').Page): Promise<string> {
  return page.evaluate(() => localStorage.getItem('token') || '')
}

// 登录态上下文（默认 storageState）：界面偏好与订阅管理。
test.describe('界面偏好与订阅', () => {
  test('i18n：切换 en-US 后界面变英文且刷新后持久', async ({ page }) => {
    await page.goto('/sites')
    await expect(page.getByRole('button', { name: '+ 添加站点' })).toBeVisible()
    // 切英文（locale 持久化在 localStorage['locale']，i18n/index.ts 启动时读取）
    await page.evaluate(() => localStorage.setItem('locale', 'en-US'))
    await page.reload()
    await expect(page.getByRole('button', { name: '+ Add site' })).toBeVisible({ timeout: 15_000 })
    // 刷新后仍英文（持久化验证点）
    await page.reload()
    await expect(page.getByRole('button', { name: '+ Add site' })).toBeVisible({ timeout: 15_000 })
    // 还原中文，避免影响同 storageState 的其他用例
    await page.evaluate(() => localStorage.setItem('locale', 'zh-CN'))
  })

  test('订阅：API 建档 → 列表可见 → UI 删除（含确认框）→ 列表消失', async ({ page, request }) => {
    await page.goto('/subscriptions')
    const auth = { headers: { Authorization: `Bearer ${await pageToken(page)}` } }
    const title = `E2E订阅${Date.now() % 100000}`
    // 清理历史残留（保证幂等）
    const existing = await (await request.get('/api/v1/subscriptions', auth)).json()
    for (const s of existing.subscriptions ?? []) {
      if (s.title?.startsWith('E2E订阅')) await request.delete(`/api/v1/subscriptions/${s.id}`, auth)
    }
    // 建档走应用自身 API（与媒体页一键订阅同一条后端链路）
    const created = await request.post('/api/v1/subscriptions', {
      headers: { Authorization: auth.headers.Authorization },
      data: { title, type: 'movie', source: 'tmdb', source_id: `e2e-${Date.now()}`, year: 2026 },
    })
    expect(created.status()).toBe(200)
    // UI 列表可见
    await page.reload()
    await expect(page.getByText(title).first()).toBeVisible({ timeout: 15_000 })
    // UI 删除：卡片内「删除」按钮 → 确认框 → 确认
    const card = page.locator('.sub', { hasText: title })
    await card.getByRole('button', { name: '删除' }).click()
    await page.getByRole('dialog').getByRole('button', { name: '删除' }).click()
    await expect(page.locator('.sub', { hasText: title })).toHaveCount(0, { timeout: 15_000 })
  })
})

// 未登录上下文（覆盖 storageState）：登录失败 / 登出流程（放最后，见文件头说明）。
test.describe('登录与会话', () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test('登录失败：错误口令出现可见错误提示且不跳转', async ({ page }) => {
    await page.goto('/login')
    await page.getByPlaceholder('用户名').fill('e2e-admin')
    await page.getByPlaceholder('密码', { exact: true }).fill('wrong-pass-000')
    await page.getByRole('button', { name: '登录', exact: true }).click()
    // 密码校验失败 → 登录页内联错误（.err），仍停留在 /login
    await expect(page.locator('.err').first()).toBeVisible({ timeout: 15_000 })
    await expect(page).toHaveURL(/\/login/)
  })

  test('登出：下拉菜单退出后回登录页且本地会话清除', async ({ page }) => {
    // 正常登录（本地密码，与 auth.setup 同源）
    await page.goto('/login')
    await page.getByPlaceholder('用户名').fill(E2E_LOCAL_USER)
    await page.getByPlaceholder('密码', { exact: true }).fill(E2E_LOCAL_PASSWORD)
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 15_000 })
    // 右上角用户菜单 → 退出登录
    await page.locator('.user-name').click()
    await page.getByText('退出登录').click()
    await page.waitForURL(/\/login/, { timeout: 15_000 })
    // 登录页同源下读 localStorage（此时会话必已清）
    await expect.poll(() => page.evaluate(() => localStorage.getItem('token') || '')).toBe('')
  })
})
