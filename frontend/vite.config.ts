// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import { fileURLToPath, URL } from 'node:url'
import { readFileSync } from 'node:fs'

// 应用版本号：构建期从 package.json 注入为全局常量 __APP_VERSION__，
// 作为前端兜底（运行时优先读后端 /system/info 的版本）。
const appVersion = JSON.parse(readFileSync(fileURLToPath(new URL('./package.json', import.meta.url)), 'utf8')).version

// 后端在 :8088；开发期把 /api 代理到后端，规避浏览器 CORS。
export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  plugins: [
    vue(),
    // element-plus 按需：自动 import 服务式组件（ElMessage/ElMessageBox 等）
    // + 模板组件 + 指令（v-loading）+ 各自样式，避免全量打包。
    AutoImport({ resolvers: [ElementPlusResolver()] }),
    Components({ directives: true, resolvers: [ElementPlusResolver()] }),
  ],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    proxy: {
      // ws:true：开发期转发 /api/v1/ws 的 WebSocket 升级（生产同源 go:embed 不受影响）。
      // timeout 拉长：跨站搜索 SSE 最长可达多站×15s，默认代理超时会掐流。
      // 目标可用 VITE_DEV_PROXY_TARGET 覆盖（8088 被宿主其他服务占用时本地起 8098 验证）。
      '/api': {
        target: process.env.VITE_DEV_PROXY_TARGET || 'http://localhost:8088',
        changeOrigin: true,
        ws: true,
        timeout: 120_000,
        proxyTimeout: 120_000,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // 阶段 1：CSS 按路由分包、去掉 modulepreload polyfill、构建期输出 gzip 体积
    cssCodeSplit: true,
    modulePreload: { polyfill: false },
    reportCompressedSize: true,
    rollupOptions: {
      output: {
        // 分离稳定的第三方基座（xterm / 流程图 / i18n / vue 核心）为独立 chunk 便于长期缓存。
        // element-plus 不在此强制合并：见下方说明，交由 Rollup 按路由懒加载自动分块。
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('@element-plus/icons-vue')) return 'element-icons';
            // 阶段 1 深挖：不再把全部 element-plus 模块合并成单一 241KB chunk。
            // 原来该 chunk 因 App.vue 根组件静态用了 ElConfigProvider 而被首屏
            // modulepreload 整块拉走；改为交回 Rollup，让各 EP 组件随引用它的
            // 懒加载路由 chunk 走，首屏只留根组件真正需要的那一小块。
            if (id.includes('element-plus')) return undefined
            if (id.includes('@xterm')) return 'xterm';
            if (id.includes('@vue-flow') || id.includes('/d3-')) return 'flow';
            if (id.includes('vue-i18n') || id.includes('@intlify')) return 'i18n';
            if (id.includes('vue') || id.includes('pinia')) return 'vue';
            return 'vendor';
          }
        },
      },
    },
    // EP 拆成按路由的细粒度 chunk 后单块已明显变小；收紧告警阈值，让体积回归更早暴露。
    chunkSizeWarningLimit: 500,
  },
})
