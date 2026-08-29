// Playwright E2E 配置：三条 webServer（后端 Go / mock qBittorrent / 前端 Vite）。
// 环境约定：
//   - 后端 HXPT_EMERGENCY_ADMIN=e2e-admin:<passcode>：应急登录通道让核心链路测试
//     完全离线（不触真实好学站）；仅测试进程注入，生产不受影响。口令段为
//     HMAC-SHA256(SECRET_KEY, username) 前 8 字节 hex（debug 默认 SECRET_KEY 下
//     由 E2E_EMERGENCY_CODE 常量给出，与后端 auth_test.go 共同锁定该语义）。
//   - mock qB 模拟 qBittorrent WebUI（创建下载器/测连/下发种子全链路可断言）。
//   - DATA_DIR 指向临时目录，每次全量跑空库起步（可重复、无外部状态依赖）。
import { defineConfig } from '@playwright/test'
import { createHmac } from 'node:crypto'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

const BACKEND_PORT = Number(process.env.E2E_BACKEND_PORT || 8089)
const FRONT_PORT = 5173
const MOCK_QB_PORT = 8099
const baseURL = `http://127.0.0.1:${FRONT_PORT}`

// E2E 后端固定用 debug 默认 SECRET_KEY，口令可确定性推导；auth.setup.ts 用同一常量登录。
const E2E_SECRET = 'dev-insecure-secret-change-me'
const E2E_EMERGENCY_USER = 'e2e-admin'
const E2E_EMERGENCY_CODE = createHmac('sha256', E2E_SECRET).update(E2E_EMERGENCY_USER).digest('hex').slice(0, 16)
export const E2E_EMERGENCY_PASSCODE = E2E_EMERGENCY_CODE

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
        HXPT_EMERGENCY_ADMIN: `${E2E_EMERGENCY_USER}:${E2E_EMERGENCY_CODE}`,
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
