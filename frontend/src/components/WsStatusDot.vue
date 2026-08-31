<script setup lang="ts">
// WsStatusDot — 顶栏实时连接状态小点。
// useWs 的模块级单例状态在这里落地成可见反馈：连上=绿点（无打扰），断开=琥珀点 +
// 「重连中」提示。此前 WS 静默断线时用户毫无感知（通知/H&R/实时速率全部停摆，
// 只会觉得「界面不更新了」），这个小点是最低成本的断线可见性。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useWs } from '@/composables/useWs'

const { t } = useI18n()
const { connected } = useWs()

const label = computed(() => (connected.value ? t('wsDot.online') : t('wsDot.offline')))
</script>

<template>
  <span
    class="ws-dot-wrap"
    role="status"
    :class="{ off: !connected }"
    :title="label"
    :aria-label="label"
  >
    <span class="ws-dot" :class="{ off: !connected }"></span>
    <span v-if="!connected" class="ws-text">{{ t('wsDot.offline') }}</span>
  </span>
</template>

<style scoped>
.ws-dot-wrap {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px;
  align-self: center;
}
.ws-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--pt-success, #2ecc71);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--pt-success, #2ecc71) 18%, transparent);
  flex: none;
  transition: background var(--dur-base) var(--ease-out-smooth);
}
.ws-dot.off {
  background: var(--pt-warn, #f39c12);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--pt-warn, #f39c12) 20%, transparent);
  animation: ws-pulse 1.6s var(--ease-out-smooth) infinite;
}
.ws-text {
  font-size: var(--fs-2xs);
  color: var(--pt-warn, #f39c12);
  white-space: nowrap;
}
@keyframes ws-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}
@media (prefers-reduced-motion: reduce) {
  .ws-dot.off {
    animation: none;
  }
}
</style>
