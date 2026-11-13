<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
  activeWorkspace: string
  hostCount: number
  monitorCount: number
  sshTargetCount: number
  credentialCount: number
  configCount: number
  activeAlertCount: number
}>()

const emit = defineEmits<{
  (e: 'workspace', v: string): void
  (e: 'execute'): void
  (e: 'go', name: string): void
  (e: 'deploy'): void
  (e: 'credentials'): void
  (e: 'configs'): void
  (e: 'monitor'): void
}>()
</script>

<template>
  <nav class="ops-shortcuts">
    <button class="sc-tab" :class="{ active: activeWorkspace === 'asset' }" @click="emit('workspace', 'asset')">
      <svg viewBox="0 0 24 24" class="sc-ico"><path d="M4 5h16v14H4V5zm2 2v10h12V7H6zm2 2h8v2H8V9z" /></svg>
      <span
        >{{ t('nog.assets') }} <em>{{ hostCount }}</em></span
      >
    </button>
    <button class="sc-tab" :class="{ active: activeWorkspace === 'execute' }" @click="emit('execute')">
      <svg viewBox="0 0 24 24" class="sc-ico">
        <path d="M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h2l2 3 2-3h2l-3 4 3 4h-2l-2-3-2 3H8l3-4-3-4z" />
      </svg>
      <span
        >{{ t('nog.batchExec') }} <em>{{ sshTargetCount }}</em></span
      >
    </button>
    <button class="sc-tab" :class="{ active: activeWorkspace === 'deploy' }" @click="emit('deploy')">
      <svg viewBox="0 0 24 24" class="sc-ico"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" /></svg>
      <span>{{ t('nog.release') }}</span>
    </button>
    <button class="sc-tab" :class="{ active: activeWorkspace === 'monitor' }" @click="emit('monitor')">
      <svg viewBox="0 0 24 24" class="sc-ico">
        <path d="M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z" />
      </svg>
      <span
        >{{ t('nog.monitor') }} <em>{{ monitorCount }}</em></span
      >
    </button>
    <span class="sc-divider"></span>
    <button class="sc-tab" @click="emit('go', 'files')">
      <svg viewBox="0 0 24 24" class="sc-ico">
        <path d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z" />
      </svg>
      <span>{{ t('nog.files') }}</span>
    </button>
    <button class="sc-tab" @click="emit('credentials')">
      <svg viewBox="0 0 24 24" class="sc-ico">
        <path
          d="M12 1L3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5l-9-4zm0 10.99h7c-.53 4.12-3.28 7.79-7 8.94V12H5V6.3l7-3.11v8.8z"
        />
      </svg>
      <span
        >{{ t('nog.credentials') }} <em>{{ credentialCount }}</em></span
      >
    </button>
    <button class="sc-tab" @click="emit('configs')">
      <svg viewBox="0 0 24 24" class="sc-ico">
        <path
          d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"
        />
      </svg>
      <span
        >{{ t('nog.configs') }} <em>{{ configCount }}</em></span
      >
    </button>
    <button class="sc-tab" @click="emit('go', 'ops-center')">
      <svg viewBox="0 0 24 24" class="sc-ico">
        <path d="M21 5H3v14h18V5zm-9 2c2.2 0 4 1.8 4 4s-1.8 4-4 4-4-1.8-4-4 1.8-4 4-4z" />
      </svg>
      <span>{{ t('nog.opsCenter') }}</span>
    </button>
    <button class="sc-tab sc-alert" @click="emit('go', 'settings')">
      <svg viewBox="0 0 24 24" class="sc-ico">
        <path
          d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2zm6-6v-5c0-3.07-1.64-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5S10.5 3.17 10.5 4v.68C7.63 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z"
        />
      </svg>
      <span
        >{{ t('nog.alerts') }} <em v-if="activeAlertCount">{{ activeAlertCount }}</em></span
      >
    </button>
  </nav>
</template>

<style scoped>
.ops-shortcuts {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  overflow-x: auto;
  scrollbar-width: thin;
  padding: 4px 0;
}

.sc-tab {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  padding: 5px 12px;
  border: 0;
  border-radius: var(--radius-pill);
  background: none;
  cursor: pointer;
  font-size: var(--fs-sm);
  color: var(--text-muted);
  white-space: nowrap;
  transition: background-color var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast) var(--ease-out-smooth), box-shadow var(--dur-fast) var(--ease-out-smooth), transform var(--dur-fast) var(--ease-out-smooth), opacity var(--dur-fast) var(--ease-out-smooth);
  flex-shrink: 0;
}
.sc-tab:hover {
  background: var(--surface-hover);
  color: var(--text-strong);
}
.sc-tab.active {
  background: var(--pt-primary);
  color: #fff;
  font-weight: 600;
}
.sc-tab.sc-alert em {
  background: var(--pt-danger);
  color: #fff;
}
.sc-tab em {
  font-style: normal;
  font-size: var(--fs-2xs);
  padding: 0 5px;
  border-radius: var(--radius-xs);
  background: var(--surface-muted);
  color: var(--text-muted);
  line-height: 1.4;
}
.sc-tab.active em {
  background: rgba(255, 255, 255, 0.25);
  color: #fff;
}

.sc-ico {
  width: 15px;
  height: 15px;
  fill: currentColor;
  flex-shrink: 0;
}
.sc-tab.active .sc-ico {
  fill: #fff;
}

.sc-divider {
  width: 1px;
  height: 20px;
  background: var(--divider);
  margin: 0 4px;
  flex-shrink: 0;
}

@media (max-width: 760px) {
  .sc-tab span {
    font-size: 0;
  }
  .sc-tab em {
    font-size: var(--fs-2xs);
  }
  .sc-tab {
    padding: 6px 8px;
  }
  .sc-ico {
    width: 18px;
    height: 18px;
  }
  .sc-divider {
    display: none;
  }
}
</style>
