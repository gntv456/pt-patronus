<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { safeUrl } from '@/utils/safeUrl'
import type { ReseedTask } from '@/api/reseed'
import ReseedMissingDialog from '@/components/ReseedMissingDialog.vue'
import { useReseed } from '@/views/useReseed'

const { t } = useI18n()

const {
  config,
  tokenInput,
  savingToken,
  torznabUrl,
  torznabKey,
  savingTorznab,
  downloaders,
  selectedDl,
  reseeding,
  tasks,
  taskTotal,
  tasksTruncated,
  loadingTasks,
  focusSiteId,
  focusSiteName,
  statusFilter,
  resolving,
  setting,
  savingSetting,
  showMissing,
  missingTargets,
  missingResult,
  checking,
  sites,
  loadTasks,
  saveToken,
  saveTorznab,
  saveSettingField,
  startReseed,
  openMissing,
  checkMissing,
  resolve,
  statusType,
  statusLabel,
} = useReseed()

// 候选表头的条数：后端单次最多返回 500 条，被截断时必须写清「最近 500 条 · 共 N 条」，
// 否则表头会把部分渲染成全部，用户清完这 500 条还以为是空的。
const taskCntLabel = computed(() => {
  const shown = tasks.value.length
  if (!tasksTruncated.value) return `${shown}`
  return taskTotal.value === null
    ? t('reseed.tasksTruncatedUnknown', { n: shown })
    : t('reseed.tasksTruncated', { n: shown, total: taskTotal.value })
})
</script>

<template>
  <div class="reseed">
    <!-- 标题行 -->
    <div class="page-head">
      <h2 class="page-title">{{ t('reseed.title') }}</h2>
      <span class="subtitle text-muted">{{ t('reseed.subtitle') }}</span>
      <span class="cfg-chip" :class="config.configured ? 'ok' : 'warn'"
        ><span class="dot"></span>{{ config.configured ? t('reseed.configured') : t('reseed.notConfigured') }}</span
      >
      <span class="cfg-chip" :class="config.torznab_configured ? 'ok' : 'warn'"
        ><span class="dot"></span
        >{{ config.torznab_configured ? t('reseed.configured') : t('reseed.notConfigured') }}</span
      >
    </div>

    <!-- 工具栏 -->
    <div class="toolbar card solid-card">
      <el-select v-model="selectedDl" :placeholder="t('reseed.selectDownloader')" class="dl-select">
        <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
      </el-select>
      <el-alert
        v-if="!config.configured"
        type="info"
        :closable="false"
        show-icon
        :title="t('reseed.noIyuuHint')"
        style="margin-bottom: 12px"
      />
      <el-button type="primary" :loading="reseeding" @click="startReseed">
        <SptIcon name="seed" :size="16" class="btn-ico" aria-hidden="true" />
        <span>{{ t('reseed.start') }}</span>
      </el-button>
      <el-button text @click="openMissing">
        <SptIcon name="clock" :size="16" class="btn-ico" aria-hidden="true" />
        <span>{{ t('reseed.missingCheck') }}</span>
      </el-button>
      <div class="spacer"></div>
      <el-popover trigger="click" placement="bottom-end" :width="380" popper-class="reseed-cfg-pop">
        <template #reference>
          <el-button text class="cfg-btn">
            <SptIcon name="gear" :size="16" class="btn-ico" aria-hidden="true" />
            <span>{{ t('settings.tab') }}</span>
          </el-button>
        </template>
        <div class="cfg-pop">
          <div class="cfg-sec">
            <div class="cfg-sec-head">
              <span class="cfg-sec-title">IYUU Token</span>
              <span class="cfg-state" :class="config.configured ? 'ok' : 'warn'">{{
                config.configured ? t('reseed.configuredAs', { v: config.masked }) : t('reseed.notConfigured')
              }}</span>
            </div>
            <div class="token-row">
              <el-input
                v-model="tokenInput"
                :placeholder="config.configured ? t('reseed.tokenOverwrite') : t('reseed.tokenPaste')"
                type="password"
                show-password
                clearable
              />
              <el-button type="primary" :loading="savingToken" @click="saveToken">{{ t('common.save') }}</el-button>
            </div>
            <a class="cfg-link" href="https://iyuu.cn" target="_blank" rel="noopener">{{ t('reseed.iyuuLink') }}</a>
          </div>
          <div class="cfg-divider"></div>
          <div class="cfg-sec">
            <div class="cfg-sec-head">
              <span class="cfg-sec-title">Torznab</span>
              <span class="cfg-state" :class="config.torznab_configured ? 'ok' : 'warn'">{{
                config.torznab_configured ? t('reseed.configured') : t('reseed.notConfigured')
              }}</span>
            </div>
            <p class="cfg-hint text-muted">{{ t('reseed.torznabHint') }}</p>
            <div class="token-row">
              <el-input
                v-model="torznabUrl"
                placeholder="https://jackett/api/v2.0/indexers/all/results/torznab"
                clearable
                size="small"
              />
            </div>
            <div class="token-row" style="margin-top: 6px">
              <el-input
                v-model="torznabKey"
                :placeholder="
                  config.torznab_key_masked
                    ? t('reseed.apiKeySet', { v: config.torznab_key_masked })
                    : t('reseed.apiKeyOptional')
                "
                show-password
                clearable
                size="small"
                @keyup.enter="saveTorznab"
              />
              <el-button type="primary" :loading="savingTorznab" @click="saveTorznab">{{ t('common.save') }}</el-button>
            </div>
          </div>
          <div class="cfg-divider"></div>
          <div class="cfg-sec" v-if="selectedDl !== null">
            <div class="cfg-sec-title">{{ t('reseed.autoReseed') }}</div>
            <div class="switch-row">
              <span>{{ t('reseed.scheduledReseed') }}</span>
              <el-switch v-model="setting.auto_reseed" :loading="savingSetting" @change="saveSettingField" />
            </div>
            <div class="switch-row">
              <span class="sub-label">{{ t('reseed.autoAddCandidates') }}</span>
              <el-switch
                v-model="setting.auto_reseed_add"
                :disabled="!setting.auto_reseed"
                :loading="savingSetting"
                @change="saveSettingField"
              />
            </div>
            <p class="cfg-cron text-muted">{{ t('reseed.cronHint') }}</p>
          </div>
          <p v-else class="cfg-empty text-muted">{{ t('reseed.selectDlFirst') }}</p>
        </div>
      </el-popover>
    </div>

    <el-alert
      v-if="focusSiteId"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      :title="t('reseed.focusTitle', { site: focusSiteName, n: tasks.length })"
    />

    <!-- 候选任务表 -->
    <div class="card solid-card table-card">
      <div class="card-head">
        <span
          >{{ t('reseed.tasks') }}<span class="cnt text-muted">（{{ taskCntLabel }}）</span></span
        >
        <el-radio-group v-model="statusFilter" size="small" @change="loadTasks">
          <el-radio-button label="pending">{{ t('reseed.st.pending') }}</el-radio-button>
          <el-radio-button label="added">{{ t('reseed.st.added') }}</el-radio-button>
          <el-radio-button label="skipped">{{ t('reseed.st.skipped') }}</el-radio-button>
          <el-radio-button label="failed">{{ t('reseed.st.failed') }}</el-radio-button>
          <el-radio-button label="">{{ t('assessment.filter.all') }}</el-radio-button>
        </el-radio-group>
      </div>
      <div class="table-wrap">
        <el-table :data="tasks" v-loading="loadingTasks" height="100%" :empty-text="t('reseed.emptyTasks')">
          <el-table-column :label="t('reseed.col.source')" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="src-name">{{ row.source_name || row.info_hash }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="candidate_site" :label="t('reseed.col.candidateSite')" width="110" />
          <el-table-column :label="t('reseed.col.candidateLink')" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              <a :href="safeUrl(row.candidate_url)" target="_blank" rel="noopener" class="dl-link">{{
                t('reseed.col.downloadLink')
              }}</a>
            </template>
          </el-table-column>
          <el-table-column :label="t('reseed.col.savePath')" min-width="160" show-overflow-tooltip>
            <template #default="{ row }"
              ><span class="text-muted">{{ row.save_path || '-' }}</span></template
            >
          </el-table-column>
          <el-table-column :label="t('reseed.col.status')" width="90">
            <template #default="{ row }"
              ><el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag></template
            >
          </el-table-column>
          <el-table-column :label="t('reseed.col.actions')" width="150" fixed="right">
            <template #default="{ row }">
              <div v-if="row.status === 'pending'" class="row-actions">
                <el-button
                  size="small"
                  type="primary"
                  :loading="resolving.has(row.id)"
                  @click="resolve(row as ReseedTask, 'add')"
                  >{{ t('reseed.addReseed') }}</el-button
                >
                <el-button size="small" :loading="resolving.has(row.id)" @click="resolve(row as ReseedTask, 'skip')">{{
                  t('reseed.skip')
                }}</el-button>
              </div>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 未辅种检查 -->
    <ReseedMissingDialog
      v-model:visible="showMissing"
      v-model:targets="missingTargets"
      v-model:result="missingResult"
      v-model:checking="checking"
      :sites="sites"
      @check="checkMissing"
    />
  </div>
</template>

<style scoped src="./Reseed.css"></style>

<!-- 弹层（el-popover teleport 到 body，scoped 选不中，单独用 popper-class 命名空间） -->
<style>
.reseed-cfg-pop {
  --el-popover-padding: 16px;
}
.reseed-cfg-pop .cfg-sec-title {
  font-weight: 600;
  font-size: var(--fs-body);
}
.reseed-cfg-pop .cfg-sec-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.reseed-cfg-pop .cfg-state {
  font-size: var(--fs-xs);
  font-weight: 600;
}
.reseed-cfg-pop .cfg-state.ok {
  color: var(--pt-success);
}
.reseed-cfg-pop .cfg-hint {
  margin: 0 0 8px;
  font-size: var(--fs-xs);
  line-height: 1.4;
}
.reseed-cfg-pop .cfg-state.warn {
  color: var(--pt-warn);
}
.reseed-cfg-pop .token-row {
  display: flex;
  gap: var(--sp-2);
}
.reseed-cfg-pop .cfg-link {
  display: inline-block;
  margin-top: 8px;
  font-size: var(--fs-xs);
  color: var(--pt-primary);
  text-decoration: none;
}
.reseed-cfg-pop .cfg-divider {
  height: 1px;
  background: rgba(128, 128, 128, 0.18);
  margin: 14px 0;
}
.reseed-cfg-pop .switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  font-size: var(--fs-body);
}
.reseed-cfg-pop .switch-row .sub-label {
  color: var(--text-muted);
}
.reseed-cfg-pop .cfg-cron {
  font-size: var(--fs-xs);
  margin: 6px 0 0;
}
.reseed-cfg-pop .cfg-empty {
  font-size: var(--fs-body);
  margin: 0;
}
</style>
