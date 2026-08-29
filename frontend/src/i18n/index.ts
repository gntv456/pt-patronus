// vue-i18n 接入（组合式 API 模式）。
// 约定（见 README/前端i18n接入评估.md）：
//   - locale 持久化在 localStorage('locale')，默认 zh-CN
//   - key 命名 `页.区块.元素`（如 login.form.username / nav.items.dashboard）
//   - 渲染侧用 t(key, 原中文兜底)——未抽取的 key 走数据里的原始文案，防漏译白屏
//   - element-plus 内置文案经 App.vue 的 ElConfigProvider 联动同一 locale
import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'

export const LOCALE_KEY = 'locale'
export type AppLocale = 'zh-CN' | 'en-US'

function initialLocale(): AppLocale {
  try {
    return localStorage.getItem(LOCALE_KEY) === 'en-US' ? 'en-US' : 'zh-CN'
  } catch {
    return 'zh-CN'
  }
}

export const i18n = createI18n({
  legacy: false,
  locale: initialLocale(),
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
  missingWarn: false,
  fallbackWarn: false,
})

export function setLocale(l: AppLocale) {
  ;(i18n.global.locale as unknown as { value: AppLocale }).value = l
  try {
    localStorage.setItem(LOCALE_KEY, l)
  } catch {
    /* 隐身模式等存储不可用：仅当前会话生效 */
  }
}
