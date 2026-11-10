// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import { fileURLToPath, URL } from 'node:url'

// 独立测试配置：与 vite.config.ts（构建用）解耦，不引入 proxy/manualChunks。
// jsdom 提供 localStorage/location，便于未来写组件测试；纯函数测试在 node 下亦可用。
// AutoImport 必须与构建配置一致，否则 ElMessage 等自动导入在测试里是未定义的全局标识符。
export default defineConfig({
  plugins: [vue(), AutoImport({ resolvers: [ElementPlusResolver({ importStyle: false })] })],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.ts'],
  },
})
