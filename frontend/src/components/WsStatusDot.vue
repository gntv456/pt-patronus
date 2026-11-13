<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

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
    aria-live="polite"
    :class="{ off: !connected }"
    :title="label"
    :aria-label="label"
  >
    <span class="ws-dot" :class="{ off: !connected }"></span>
    <!-- 数字与文字兜底：状态不只靠颜色表达；静态点，不用无限 pulse -->
    <span class="ws-num" aria-hidden="true">{{ connected ? '1' : '0' }}</span>
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
  background: var(--color-success);
  flex: none;
  transition: background var(--dur-fast) var(--ease);
}
.ws-dot.off {
  background: var(--color-warning);
}
.ws-num {
  font-size: var(--fs-xs);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
  line-height: 1;
}
.ws-text {
  font-size: var(--fs-xs);
  color: var(--pt-warn-fg);
  white-space: nowrap;
}
</style>
