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
      '/api': { target: 'http://localhost:8088', changeOrigin: true, ws: true },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // 分离大块 vendor：element-plus（全量 UI 库）/ vue 核心 / 其余 vendor。
        // 避免单一 index chunk 过大，且各 vendor 独立缓存——app 代码变更不会 invalidate 稳定的 vendor chunk。
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('@element-plus/icons-vue')) return 'element-icons';
            if (id.includes('element-plus')) return 'element-plus';
            if (id.includes('vue') || id.includes('pinia')) return 'vue';
            return 'vendor';
          }
        },
      },
    },
    // Element Plus 是内部工具的稳定 UI 基座，独立缓存后可接受略大于默认阈值的 vendor chunk。
    chunkSizeWarningLimit: 850,
  },
})
