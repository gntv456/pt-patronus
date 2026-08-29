import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { i18n } from './i18n'
import { showErrorOverlay } from './utils/errorOverlay'
import { useUiPrefs } from './composables/useUiPrefs'
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

// element-plus 不再全量注册：组件 / 指令 / 服务式组件 + 样式
// 由 vite 的 unplugin-vue-components + ElementPlusResolver 按需自动导入（见 vite.config.ts）。

const app = createApp(App)
app.use(createPinia())
app.use(i18n)
app.use(router)
// 可点击 div 的键盘可达（a11y）：v-a11y-button = role=button + tabindex + Enter/Space
app.directive('a11y-button', a11yButton)

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

// 玻璃性能降级：低端设备（CPU/RAM 低）或手动开启（localStorage['perf-glass-flat']='1'）时，
// 给 body 加 glass-reduced 类，关闭全站 backdrop-filter（GPU 合成性能杀手，187 处卡片层叠掉帧）。
// 对齐移动端 glassReduced；系统「减少透明度」偏好由 glass.css @media 自动处理。
{
  const nav = navigator as Navigator & { deviceMemory?: number }
  const lowEnd =
    (navigator.hardwareConcurrency > 0 && navigator.hardwareConcurrency <= 4) ||
    (typeof nav.deviceMemory === 'number' && nav.deviceMemory <= 4)
  if (lowEnd || localStorage.getItem('perf-glass-flat') === '1') {
    document.body.classList.add('glass-reduced')
  }
}

// 界面偏好（主题色/风格/壁纸/遮罩/卡片样式）：挂载前应用到 body，避免首帧闪烁。
// useUiPrefs 内部 watch immediate 应用 + 经用户偏好服务端同步跨浏览器保留。
{
  const { apply } = useUiPrefs()
  apply()
}

app.mount('#app')
