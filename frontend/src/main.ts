import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { showErrorOverlay } from './utils/errorOverlay'
import './styles/glass.css'

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
app.use(router)

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

app.mount('#app')
