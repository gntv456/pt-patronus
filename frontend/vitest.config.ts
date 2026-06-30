import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// 独立测试配置：与 vite.config.ts（构建用）解耦，不引入 proxy/manualChunks。
// jsdom 提供 localStorage/location，便于未来写组件测试；纯函数测试在 node 下亦可用。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.ts'],
  },
})
