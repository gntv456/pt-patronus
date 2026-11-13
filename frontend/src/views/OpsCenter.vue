<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { useQueryState } from '@/composables/useUrlState'
import { useI18n } from 'vue-i18n'
import OpsMonitor from '@/components/ops/OpsMonitor.vue'
import OpsDocker from '@/components/ops/OpsDocker.vue'
import OpsDatabase from '@/components/ops/OpsDatabase.vue'
import OpsDisk from '@/components/ops/OpsDisk.vue'
import OpsProcess from '@/components/ops/OpsProcess.vue'
import OpsSsh from '@/components/ops/OpsSsh.vue'
import OpsTerminal from '@/components/ops/OpsTerminal.vue'
import FileManager from '@/views/FileManager.vue'

type Tab = 'monitor' | 'files' | 'docker' | 'database' | 'disk' | 'process' | 'ssh' | 'terminal'

const { t } = useI18n()
const tabQuery = useQueryState('tab', 'monitor')
const VALID_TABS: Tab[] = ['monitor', 'files', 'docker', 'database', 'disk', 'process', 'ssh', 'terminal']
// activeTab 由 URL query 读写，刷新/分享可恢复；非法值回退 monitor。
const activeTab = computed<Tab>({
  get: () => (VALID_TABS.includes(tabQuery.value as Tab) ? (tabQuery.value as Tab) : 'monitor'),
  set: (v: Tab) => {
    tabQuery.value = v
  },
})
// 方向键在标签间移动焦点（tablist 键盘可达）。
function stepTab(dir: -1 | 1) {
  const i = VALID_TABS.indexOf(activeTab.value)
  const next = VALID_TABS[(i + dir + VALID_TABS.length) % VALID_TABS.length]
  activeTab.value = next
  void nextTick(() => {
    document.querySelector<HTMLButtonElement>(`.ops-tab[data-key="${next}"]`)?.focus()
  })
}

const tabs = computed(() => [
  {
    key: 'monitor' as Tab,
    label: t('ops.tab.monitor'),
    icon: 'M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z',
  },
  {
    key: 'files' as Tab,
    label: t('ops.tab.files'),
    icon: 'M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z',
  },
  {
    key: 'docker' as Tab,
    label: t('ops.tab.docker'),
    icon: 'M21 5H3v14h18V5zm-9 2c2.2 0 4 1.8 4 4s-1.8 4-4 4-4-1.8-4-4 1.8-4 4-4z',
  },
  {
    key: 'database' as Tab,
    label: t('ops.tab.database'),
    icon: 'M12 3C7.58 3 4 4.79 4 7v10c0 2.21 3.59 4 8 4s8-1.79 8-4V7c0-2.21-3.58-4-8-4zm0 2c3.87 0 6 1.5 6 2s-2.13 2-6 2-6-1.5-6-2 2.13-2 6-2z',
  },
  { key: 'disk' as Tab, label: t('ops.tab.disk'), icon: 'M4 6h16l-1 14H5L4 6zm5 0V4h6v2' },
  {
    key: 'process' as Tab,
    label: t('ops.tab.process'),
    icon: 'M3.5 18.49l6-6.01 4 4L22 6.92l-1.41-1.41-7.09 7.97-4-4L2 16.99z',
  },
  {
    key: 'ssh' as Tab,
    label: t('ops.tab.ssh'),
    icon: 'M20 2H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h4v3l4-3h8c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z',
  },
  {
    key: 'terminal' as Tab,
    label: t('ops.tab.terminal'),
    icon: 'M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h2l2 3 2-3h2l-3 4 3 4h-2l-2-3-2 3H8l3-4-3-4z',
  },
])
</script>

<template>
  <div class="ops-center">
    <nav class="ops-nav" role="tablist" :aria-label="t('nog.opsCenter')">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        class="ops-tab"
        :data-key="tab.key"
        role="tab"
        :aria-selected="activeTab === tab.key"
        :aria-label="tab.label"
        :tabindex="activeTab === tab.key ? 0 : -1"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
        @keydown.left.prevent="stepTab(-1)"
        @keydown.right.prevent="stepTab(1)"
      >
        <svg viewBox="0 0 24 24" class="ops-tab-ico"><path :d="tab.icon" /></svg>
        <span class="ops-tab-label">{{ tab.label }}</span>
      </button>
    </nav>

    <main class="ops-content">
      <KeepAlive>
        <OpsMonitor v-if="activeTab === 'monitor'" />
        <FileManager v-else-if="activeTab === 'files'" :embedded="true" />
        <OpsDocker v-else-if="activeTab === 'docker'" />
        <OpsDatabase v-else-if="activeTab === 'database'" />
        <OpsDisk v-else-if="activeTab === 'disk'" />
        <OpsProcess v-else-if="activeTab === 'process'" />
        <OpsSsh v-else-if="activeTab === 'ssh'" />
        <OpsTerminal v-else-if="activeTab === 'terminal'" />
      </KeepAlive>
    </main>
  </div>
</template>

<style scoped>
.ops-center {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  max-width: 1400px;
  margin: 0 auto;
  padding: var(--sp-3) var(--sp-4);
}

.ops-nav {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  overflow-x: auto;
  scrollbar-width: thin;
  padding-bottom: 2px;
}

.ops-tab {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  min-height: 24px;
  padding: 6px 14px;
  border: 0;
  border-radius: var(--radius-control);
  background: none;
  cursor: pointer;
  font-size: var(--fs-sm);
  color: var(--color-text-muted);
  white-space: nowrap;
  transition:
    background-color var(--dur-fast) var(--ease),
    color var(--dur-fast) var(--ease);
  flex-shrink: 0;
}
.ops-tab:hover {
  background: var(--surface-hover);
  color: var(--text-strong);
}
.ops-tab.active {
  background: var(--color-primary);
  color: var(--color-text-inverse);
  font-weight: 600;
  box-shadow: none;
}

.ops-tab-ico {
  width: 16px;
  height: 16px;
  fill: currentColor;
  flex-shrink: 0;
}
.ops-tab.active .ops-tab-ico {
  fill: var(--color-text-inverse);
}
.ops-tab-label {
  line-height: 1;
}

.ops-content {
  min-height: 400px;
  flex: 1;
}

@media (max-width: 760px) {
  .ops-tab-label {
    display: none;
  }
  .ops-tab {
    padding: 8px 10px;
    min-height: 44px;
  }
  .ops-tab-ico {
    width: 20px;
    height: 20px;
  }
}
</style>
