// 可点击 div 的无障碍键盘可达指令（a11y 审计 P2：27 处 <div @click> 无法被键盘
// 操作——无 tab 焦点、无 Enter/Space 触发）。
// 用法：在带 @click 的 div 上加 v-a11y-button，即可获得：
//   role="button" + tabindex="0"（进入 Tab 序）+ Enter/Space 触发 click()
//   （Space 阻止默认滚动；click() 复用元素已有 @click 处理器，无需改业务代码）。
// 幂等：重复挂载不重复设 role/tabindex。
import type { Directive } from 'vue'

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Enter' && e.key !== ' ') return
  e.preventDefault() // Space 默认滚动页面 / Enter 默认提交——按钮语义下都不应触发
  ;(e.currentTarget as HTMLElement).click()
}

export const a11yButton: Directive<HTMLElement> = {
  mounted(el) {
    if (!el.hasAttribute('role')) el.setAttribute('role', 'button')
    if (!el.hasAttribute('tabindex')) el.setAttribute('tabindex', '0')
    el.addEventListener('keydown', onKeydown)
  },
  unmounted(el) {
    el.removeEventListener('keydown', onKeydown)
  },
}
