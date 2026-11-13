<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// GlobalNoticeBar — 全局消息提醒横幅（所有页面 topbar 下方）。
// 聚合 useWs 实时告警（Cookie 失效站点 + H&R 危险种子），红色告警色醒目提示。
// 有告警才显示，可展开详情、可关闭；新告警到达自动重显。无告警时不占位。
import { useI18n } from 'vue-i18n'
import { computed, ref, watch } from 'vue'
import { useWs } from '@/composables/useWs'
import SptIcon from '@/components/SptIcon.vue'
import { useFeedback } from '@/composables/useFeedback'

const { t } = useI18n()
const feedback = useFeedback()
const { hrAlerts, cookieAlerts } = useWs()

const expanded = ref(false)
// 已忽略的告警条数；=== 当前条数时视为已关闭。新告警到达（条数增加）自动重显。
const dismissedCount = ref(0)

interface Notice {
  key: string
  icon: string
  text: string
}

const notices = computed<Notice[]>(() => {
  const items: Notice[] = []
  for (const site of cookieAlerts.value) {
    items.push({ key: `cookie-${site}`, icon: 'alert-triangle', text: t('gnb.cookieExpired', { site }) })
  }
  for (const hr of hrAlerts.value) {
    items.push({
      key: `hr-${hr.site}-${hr.title}`,
      icon: 'clock',
      text: t('gnb.hrDanger', {
        site: hr.site,
        title: hr.title,
        dl: hr.deadline ? t('gnb.deadline', { d: hr.deadline }) : '',
      }),
    })
  }
  return items
})

watch(
  () => notices.value.length,
  (n, old) => {
    if (n > (old ?? 0)) dismissedCount.value = 0 // 新告警到达，自动重显
  },
)

const visible = computed(() => notices.value.length > 0 && dismissedCount.value === 0)

function dismiss() {
  const prev = dismissedCount.value
  dismissedCount.value = notices.value.length
  feedback.undo(t('gnb.dismiss'), () => {
    dismissedCount.value = prev
  })
}
</script>

<template>
  <Transition name="gnb-slide">
    <div v-if="visible" class="gnb" role="status">
      <SptIcon name="alert-triangle" :size="18" class="gnb-icon" />
      <button
        type="button"
        class="gnb-body"
        :aria-expanded="notices.length > 1 ? expanded : undefined"
        :disabled="notices.length <= 1"
        @click="expanded = !expanded"
      >
        <span class="gnb-summary">
          <span class="gnb-tag">{{ t('gnb.tag') }}</span>
          <span class="gnb-count">{{ notices.length }}</span>
          <span class="gnb-text">
            {{ notices[0].text
            }}<template v-if="notices.length > 1"> {{ t('gnb.moreN', { n: notices.length }) }}</template>
          </span>
        </span>
        <span v-if="expanded" class="gnb-detail">
          <span v-for="n in notices" :key="n.key" class="gnb-item">
            <SptIcon :name="n.icon" :size="14" />
            <span>{{ n.text }}</span>
          </span>
        </span>
      </button>
      <button
        v-if="notices.length > 1"
        class="gnb-btn"
        :aria-label="expanded ? t('trt.collapse') : t('trt.expand')"
        @click.stop="expanded = !expanded"
      >
        <SptIcon name="chevron-down" :size="16" class="gnb-chevron" :class="{ 'gnb-chevron--up': expanded }" />
      </button>
      <button class="gnb-btn" :aria-label="t('gnb.dismiss')" @click.stop="dismiss">
        <SptIcon name="x" :size="16" />
      </button>
    </div>
  </Transition>
</template>

<style scoped>
.gnb {
  display: flex;
  align-items: flex-start;
  gap: var(--sp-2);
  padding: 9px 14px;
  border-radius: var(--radius-card);
  /* 浅底深字：不用红橙渐变（白字在橙端仅 2.19:1），语义色文字在明暗主题都 >= 4.5:1 */
  background: var(--color-danger-soft);
  border: 1px solid var(--color-danger);
  color: var(--pt-danger-fg);
  font-size: var(--fs-body);
  line-height: 1.5;
}
.gnb-icon {
  flex: none;
  margin-top: 1px;
}
.gnb-body {
  flex: 1;
  min-width: 0;
  padding: 0;
  background: transparent;
  border: none;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.gnb-body:disabled {
  cursor: default;
}
.gnb-summary {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
.gnb-tag {
  flex: none;
  padding: 1px 8px;
  border-radius: var(--radius-pill);
  background: var(--color-surface);
  border: 1px solid var(--color-danger);
  color: var(--pt-danger-fg);
  font-size: var(--fs-xs);
  font-weight: 700;
}
.gnb-count {
  flex: none;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: var(--radius-pill);
  background: var(--color-surface);
  border: 1px solid var(--color-danger);
  color: var(--pt-danger-fg);
  font-size: var(--fs-xs);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.gnb-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gnb-detail {
  margin-top: 6px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  padding-top: 6px;
  border-top: 1px solid var(--color-border);
}
.gnb-item {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  font-size: var(--fs-xs);
}
.gnb-btn {
  flex: none;
  min-width: 24px;
  min-height: 24px;
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--pt-danger-fg);
  padding: 4px;
  border-radius: var(--radius-control);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background var(--dur-fast) var(--ease);
}
.gnb-btn:hover {
  background: var(--color-surface);
}
.gnb-chevron {
  transition: transform var(--dur-base) var(--ease);
}
.gnb-chevron--up {
  transform: rotate(180deg);
}
.gnb-slide-enter-active,
.gnb-slide-leave-active {
  transition: opacity var(--dur-fast) var(--ease);
}
.gnb-slide-enter-from,
.gnb-slide-leave-to {
  opacity: 0;
}

@media (max-width: 720px) {
  .gnb-btn {
    min-width: 44px;
    min-height: 44px;
  }
}
</style>
