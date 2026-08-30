// vue-i18n 接入（组合式 API 模式）。
// 约定（见 README/前端i18n接入评估.md）：
//   - locale 持久化在 localStorage('locale')，默认 zh-CN
//   - key 命名 `页.区块.元素`（如 login.form.username / nav.items.dashboard）
//   - 渲染侧用 t(key, 原中文兜底)——未抽取的 key 走数据里的原始文案，防漏译白屏
//   - element-plus 内置文案经 App.vue 的 ElConfigProvider 联动同一 locale
//
// 懒加载：两张翻译表各 ~145KB（gzip ~50KB），静态 import 会全部进首屏 chunk。
// 改为按需动态 import——只载当前语言，切换时再载另一张。载入前 messages 为空
// 对象，t() 走兜底文案不白屏；main.ts 在 mount 前 await i18nBoot 保证首帧有文案。
import { createI18n } from 'vue-i18n'

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
  messages: { 'zh-CN': {}, 'en-US': {} },
  missingWarn: false,
  fallbackWarn: false,
})

// 已载入标记：避免重复动态 import（测试/热重载多次调用）。
const loaded = new Set<AppLocale>()

/** 动态载入某语言的翻译表。幂等；失败时保留空表（t() 回落兜底文案，不白屏）。 */
export async function loadLocale(l: AppLocale) {
  if (loaded.has(l)) return
  const mod = l === 'en-US' ? await import('./locales/en-US') : await import('./locales/zh-CN')
  i18n.global.setLocaleMessage(l, mod.default)
  loaded.add(l)
}

/** 切换语言：先确保目标表已载入再切，避免切换瞬间整页回落兜底中文闪烁。 */
export async function setLocale(l: AppLocale) {
  await loadLocale(l)
  ;(i18n.global.locale as unknown as { value: AppLocale }).value = l
  try {
    localStorage.setItem(LOCALE_KEY, l)
  } catch {
    /* 隐身模式等存储不可用：仅当前会话生效 */
  }
}

// 启动即载当前语言；main.ts 在 app.mount 前 await 它，保证首帧渲染就有完整文案。
export const i18nBoot: Promise<void> = loadLocale(initialLocale())
