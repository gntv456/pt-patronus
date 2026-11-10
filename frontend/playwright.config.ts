// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Playwright E2E 配置：三条 webServer（后端 Go / mock qBittorrent / 前端 Vite）。
// 环境约定：
//   - 登录走本地密码（/auth/local/login，好学站点关闭后的主路径，#<issue>）：
//     空库首登自动建号（e2e-admin 即 admin），完全离线不触真实好学站。
//   - mock qB 模拟 qBittorrent WebUI（创建下载器/测连/下发种子全链路可断言）。
//   - DATA_DIR 指向临时目录，每次全量跑空库起步（可重复、无外部状态依赖）。
import { defineConfig } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

const BACKEND_PORT = Number(process.env.E2E_BACKEND_PORT || 8089)
const FRONT_PORT = 5173
const MOCK_QB_PORT = 8099
const baseURL = `http://127.0.0.1:${FRONT_PORT}`

// E2E 本地登录账号：空库首登自动建号即 admin（密码满足「8 位+字母+数字」闸门）。
export const E2E_LOCAL_USER = 'e2e-admin'
export const E2E_LOCAL_PASSWORD = 'e2e-pass-2024'

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false, // 共享一个后端实例与空库状态，串行保证确定性
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: [
    {
      command: `node e2e/mock-qb.mjs`,
      url: `http://127.0.0.1:${MOCK_QB_PORT}/api/v2/app/version`,
      reuseExistingServer: !process.env.CI,
      timeout: 15_000,
    },
    {
      command:
        `cd ../backend && go run ./cmd/ptpatronus`,
      url: `http://127.0.0.1:${BACKEND_PORT}/api/v1/system/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        APP_ENV: 'debug',
        HTTP_ADDR: `:${BACKEND_PORT}`,
        DATA_DIR: process.env.E2E_DATA_DIR || '../frontend/e2e/.data',
        JWT_EXPIRE_MIN: '120',
        GLOBAL_RATE_PER_SEC: '1000', // 测试不发限流（UI 并发请求不受 429 干扰）
        LOGIN_RATE_PER_MIN: '1000',
      },
    },
    {
      command: `npm run dev -- --host 127.0.0.1 --port ${FRONT_PORT} --strictPort`,
      url: baseURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: { VITE_DEV_PROXY_TARGET: `http://127.0.0.1:${BACKEND_PORT}` },
    },
  ],
  projects: [
    // setup 项目：执行一次登录，产出 storageState（JWT 进 localStorage 快照）
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    {
      name: 'chromium',
      use: { browserName: 'chromium', storageState: join(here, 'e2e', '.auth', 'state.json') },
      dependencies: ['setup'],
    },
  ],
})
