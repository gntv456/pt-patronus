// 登录 setup：产出 storageState 供后续测试项目复用（每个测试免重复登录）。
// 走后端 HXPT_EMERGENCY_ADMIN 应急通道——完全离线，不触真实好学站。
import { expect, test } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const stateFile = join(here, '.auth', 'state.json')

test('登录并保存会话', async ({ page }) => {
  await page.goto('/login')
  await page.getByPlaceholder('好学用户名').fill('e2e-admin')
  await page.getByPlaceholder('好学密码', { exact: false }).fill('e2e-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  // 登录成功 → 跳出 /login，且 JWT 落 localStorage（storageState 会捕获）
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 15_000 })
  const token = await page.evaluate(() => localStorage.getItem('token') || '')
  expect(token).not.toBe('')
  mkdirSync(dirname(stateFile), { recursive: true })
  await page.context().storageState({ path: stateFile })
})
