<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 下一最佳行动卡：按工作台数据派生「当前最该做的一件事」，补齐中段引导断层
// （空库 3 步向导完成即消失，之后没有「配下载器 → 配订阅」类引导）。
// 单条展示 + 可关闭（localStorage 记忆，dismissed 不再打扰）；完成后自动切下一项。
// 判据只用 Workbench 已有数据（零新接口）；「下载器在跑」以今日整理成功为信号。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import type { Workbench } from '@/api/dashboard'
import SptIcon from '@/components/SptIcon.vue'

const props = defineProps<{ wb: Workbench | null }>()
const { t } = useI18n()
const router = useRouter()

const DISMISS_KEY = 'pt.nextAction.dismissed'

interface Action {
  key: 'sites' | 'downloaders' | 'subscriptions'
  route: string
  done: (wb: Workbench) => boolean
}

// 优先级从上到下：第一个未完成的就是「下一行动」。三步走完卡片自动消失。
const ACTIONS: Action[] = [
  { key: 'sites', route: '/sites', done: (w) => w.summary.sites_active > 0 },
  {
    key: 'downloaders',
    route: '/downloaders',
    done: (w) => w.summary.downloads_done > 0,
  },
  {
    key: 'subscriptions',
    route: '/subscriptions',
    done: (w) => w.summary.subscriptions_new + w.summary.subscription_hits > 0,
  },
]

const dismissed = ref(localStorage.getItem(DISMISS_KEY) === '1')

const current = computed(() => {
  const wb = props.wb
  if (!wb || dismissed.value) return null
  return ACTIONS.find((x) => !x.done(wb)) ?? null
})

function dismiss() {
  dismissed.value = true
  localStorage.setItem(DISMISS_KEY, '1')
}
</script>

<template>
  <div
    v-if="current"
    class="next-action"
    role="region"
    :aria-label="t('wb.next.title')"
  >
    <SptIcon name="arrow-right" :size="16" class="na-ico" aria-hidden="true" />
    <div class="na-body">
      <span class="na-title">{{ t('wb.next.title') }}</span>
      <span class="na-desc">{{ t('wb.next.' + current.key) }}</span>
    </div>
    <el-button size="small" type="primary" @click="router.push(current.route)">
      {{ t('wb.next.go') }}
    </el-button>
    <button
      type="button"
      class="na-close"
      :aria-label="t('wb.next.dismiss')"
      @click="dismiss"
    >
      <SptIcon name="x" :size="14" aria-hidden="true" />
    </button>
  </div>
</template>

<style scoped>
.next-action {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  margin-bottom: var(--space-3);
  background: var(--color-primary-soft);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-card);
}
.na-ico {
  color: var(--color-primary);
  flex: none;
}
.na-body {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  min-width: 0;
  flex: 1;
}
.na-title {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--color-text);
  flex: none;
}
.na-desc {
  font-size: var(--fs-xs);
  color: var(--color-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.na-close {
  border: 0;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-control);
  font-size: var(--fs-xs);
}
.na-close:hover {
  background: var(--color-surface-hover);
}
</style>
