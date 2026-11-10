// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { i18n, i18nBoot } from './i18n'
import { showErrorOverlay } from './utils/errorOverlay'
import { startTokenRefresher } from './api/http'
import { useUiPrefs } from './composables/useUiPrefs'
import { usePerfMode } from './composables/usePerfMode'
import { a11yButton } from './directives/a11yButton'
import './styles/glass.css'
import './styles/dialog.css'

// 服务式组件 ElMessageBox 的样式在 unplugin AutoImport + ElementPlusResolver 下注入不可靠
// （JS 调用的服务式组件 CSS 常被漏掉 → 弹窗跑到左上角、半透明）。手动确保加载。
// message/notification/loading 同理一并引入，规避同类问题。
import 'element-plus/es/components/message-box/style/css'
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/notification/style/css'
import 'element-plus/es/components/loading/style/css'

// EP 官方暗色主题变量（html.dark 命名空间）：未加载时所有未被 :deep 覆盖的 EP 组件
// （select 下拉/message-box/日期弹层）在暗色下保持白底——W1 修复项。
import 'element-plus/theme-chalk/dark/css-vars.css'

// element-plus 不再全量注册：组件 / 指令 / 服务式组件 + 样式
// 由 vite 的 unplugin-vue-components + ElementPlusResolver 按需自动导入（见 vite.config.ts）。

const app = createApp(App)
app.use(createPinia())
app.use(i18n)
app.use(router)
// 可点击 div 的键盘可达（a11y）：v-a11y-button = role=button + tabindex + Enter/Space
app.directive('a11y-button', a11yButton)

// JWT 滑动续期定时器（30 分钟一轮 + 401 被动续期见 api/http.ts）：活跃用户不再
// 「用着用着被踢回登录页」（后端 /auth/refresh 此前无前端调用方，端点闲置）。
startTokenRefresher()

// 全局错误兜底：Vue 渲染错误默认会卸载出错组件树 → 整页白屏（终端用户零信息）。
// errorHandler + window error/unhandledrejection 三路捕获后弹可见错误卡片
// （错误名 + 堆栈 + 刷新/回首页），把白屏变成可反馈的错误。详见 utils/errorOverlay.ts。
app.config.errorHandler = (err, _instance, info) => {
  showErrorOverlay(err, `页面渲染出错（${info}）`)
}
window.addEventListener('error', (e) => {
  // 只处理真正的 JS 运行时异常（e.error 非空）；资源加载失败的 error 事件 error 字段为 null。
  if (e.error) showErrorOverlay(e.error, '脚本运行时错误')
})
window.addEventListener('unhandledrejection', (e) => {
  showErrorOverlay(e.reason, '未处理的 Promise 错误')
})

// 玻璃性能降级（阶段 1）：默认关闭全站 backdrop-filter（187 处 blur 是 GPU 合成杀手）。
// 低端机 / perf-glass-flat=1 强制降级；用户可置 perf-glass-full=1 或调 setGlassBlur(true) 恢复。
// 系统「减少透明度」偏好仍由 glass.css @media 自动处理。
{
  const { applyGlassMode } = usePerfMode()
  applyGlassMode()
}

// 界面偏好（主题色/风格/壁纸/遮罩/卡片样式）：挂载前应用到 body，避免首帧闪烁。
// useUiPrefs 内部 watch immediate 应用 + 经用户偏好服务端同步跨浏览器保留。
{
  const { apply } = useUiPrefs()
  apply()
}

// 当前语言翻译表就绪后再挂载（懒加载：只载用户语言那张，另一张切换时再取）。
// 用 then 而非顶层 await——构建目标浏览器(chrome87 等)不支持顶层 await。
i18nBoot.then(() => app.mount('#app'))
