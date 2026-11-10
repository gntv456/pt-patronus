// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 登录 setup：产出 storageState 供后续测试项目复用（每个测试免重复登录）。
// 走本地密码登录（/auth/local/login，好学站点关闭后的主路径，#<issue>）——
// 空库首登自动建号即 admin，完全离线不触真实好学站。
import { expect, test } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { E2E_LOCAL_PASSWORD, E2E_LOCAL_USER } from '../playwright.config'

const here = dirname(fileURLToPath(import.meta.url))
const stateFile = join(here, '.auth', 'state.json')

test('登录并保存会话', async ({ page }) => {
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill(E2E_LOCAL_USER)
  await page.getByPlaceholder('密码', { exact: true }).fill(E2E_LOCAL_PASSWORD)
  await page.getByRole('button', { name: '登录', exact: true }).click()
  // 登录成功 → 跳出 /login，且 JWT 落 localStorage（storageState 会捕获）
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 15_000 })
  const token = await page.evaluate(() => localStorage.getItem('token') || '')
  expect(token).not.toBe('')
  mkdirSync(dirname(stateFile), { recursive: true })
  await page.context().storageState({ path: stateFile })
})
