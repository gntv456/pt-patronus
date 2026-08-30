<script setup lang="ts">
import TransferConfigDialog from '@/components/transfer/TransferConfigDialog.vue'
import TransferMoreDialogs from '@/components/transfer/TransferMoreDialogs.vue'
import { useI18n } from 'vue-i18n'
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import { Delete, Plus } from '@element-plus/icons-vue'
import { useTransfer } from './useTransfer'
import type { TransferHistory, TransferPendingItem, DirSyncTask } from '@/api/transfer'

const { t } = useI18n()

const {
  list,
  loading,
  statusFilter,
  viewMode,
  pendingItems,
  pendingTotal,
  pendingEnabled,
  organizing,
  redoing,
  downloaders,
  filtered,
  load,
  setView,
  organizePending,
  sweep,
  redoRow,
  deleteRow,
  selectedRows,
  handleSelectionChange,
  batchDeleteRows,
  clearFailed,
  showConfig,
  cfg,
  cloudDrives,
  showDirwatch,
  dwSaving,
  dwForm,
  openDirwatch,
  saveDirwatch,
  catsStr,
  skipStr,
  savingCfg,
  openConfig,
  saveConfig,
  addPathMapping,
  removePathMapping,
  showCategories,
  catMovieText,
  catTVText,
  catSaving,
  openCategories,
  saveCategories,
  showIdentify,
  identifyWords,
  identifySaving,
  identifyTestName,
  identifyTestResult,
  openIdentify,
  saveIdentify,
  addIdentifyWord,
  removeIdentifyWord,
  testIdentify,
  testRecognize,
  recognizeTesting,
  sharedRepos,
  shareImporting,
  sharePreview,
  loadSharedRepos,
  addSharedRepo,
  removeSharedRepo,
  saveSharedRepos,
  importSharedRepo,
  showTags,
  dirTags,
  sortRules,
  tagsSaving,
  openTags,
  saveTagsAndRules,
  showDirSync,
  syncTasks,
  syncSaving,
  syncingName,
  openDirSync,
  saveSyncTasks,
  addSyncTask,
  runSync,
  showOrganize,
  selDl,
  torrents,
  selHash,
  loadingTorrents,
  openOrganize,
  loadTorrents,
  completedTorrents,
  doOrganize,
  modeLabel,
  renameVars,
  insertVar,
  previewRename,
} = useTransfer()
</script>

<template>
  <div class="page">
    <header class="header">
      <div>
        <h1>{{ t('transfer.title') }}</h1>
        <p class="sub">{{ t('transfer.subtitle') }}</p>
      </div>
      <div class="actions">
        <el-radio-group v-model="viewMode" size="small" @change="load">
          <el-radio-button value="history">{{ t('transfer.view.history') }}</el-radio-button>
          <el-radio-button value="pending">{{ t('transfer.view.pending') }}</el-radio-button>
        </el-radio-group>
        <el-select
          v-model="statusFilter"
          size="small"
          style="width: 110px"
          :disabled="viewMode === 'pending'"
          @change="load"
        >
          <el-option :label="t('assessment.filter.all')" value="" />
          <el-option :label="t('transfer.st.success')" value="success" />
          <el-option :label="t('transfer.st.failed')" value="failed" />
          <el-option :label="t('transfer.st.skipped')" value="skipped" />
        </el-select>
        <el-button size="small" @click="load" :loading="loading">{{ t('common.refresh') }}</el-button>
        <el-button size="small" @click="sweep">{{ t('transfer.sweep') }}</el-button>
        <el-button size="small" type="primary" @click="openOrganize">{{ t('transfer.organize') }}</el-button>
        <el-button size="small" @click="openConfig">{{ t('transfer.config') }}</el-button>
        <el-button size="small" @click="openDirwatch">{{ t('transfer.dirwatch') }}</el-button>
        <el-button size="small" type="danger" plain :disabled="!selectedRows.length" @click="batchDeleteRows">
          {{ t('transfer.batchDelete') }}{{ selectedRows.length ? `(${selectedRows.length})` : '' }}
        </el-button>
        <el-button size="small" type="danger" plain @click="clearFailed">{{ t('transfer.clearFailed') }}</el-button>
      </div>
    </header>

    <div class="glass card">
      <div v-if="viewMode === 'pending'" v-loading="loading" class="pending-panel">
        <el-alert
          v-if="!pendingEnabled"
          type="warning"
          :closable="false"
          show-icon
          :title="t('transfer.notEnabledTitle')"
          :description="t('transfer.notEnabledDesc')"
          style="margin-bottom: 12px"
        />
        <p class="text-muted" style="margin: 0 0 10px">
          {{ t('transfer.pendingCount', { n: pendingTotal }) }}
        </p>
        <el-table :data="pendingItems" stripe :empty-text="t('transfer.noPending')">
          <el-table-column prop="name" :label="t('transfer.col.torrent')" min-width="220" show-overflow-tooltip />
          <el-table-column prop="downloader_name" :label="t('downloaders.title')" width="120" />
          <el-table-column prop="category" :label="t('rss.f.category')" width="100" />
          <el-table-column prop="save_path" :label="t('mediaserver.col.path')" min-width="160" show-overflow-tooltip />
          <el-table-column :label="t('reseed.col.actions')" width="100" fixed="right">
            <template #default="{ row }">
              <el-button
                size="small"
                type="primary"
                link
                :loading="organizing"
                @click="organizePending(row as TransferPendingItem)"
                >{{ t('transfer.organizeAction') }}</el-button
              >
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div v-show="viewMode === 'history'">
        <el-table
          :data="filtered"
          v-loading="loading"
          size="small"
          :empty-text="t('transfer.noRecords')"
          row-key="id"
          @selection-change="handleSelectionChange"
        >
          <el-table-column type="selection" width="36" :reserve-selection="true" fixed="left" />
          <el-table-column :label="t('transfer.col.media')" min-width="220">
            <template #default="{ row }">
              <div class="media-cell">
                <div class="poster"><MediaPoster :url="row.poster_url" /></div>
                <div>
                  <div class="title">
                    {{ row.title }}<span v-if="row.year" class="year"> ({{ row.year }})</span>
                  </div>
                  <div class="meta">
                    <el-tag size="small" :type="row.type === 'tv' ? 'warning' : 'info'" effect="plain">
                      {{ row.type === 'tv' ? t('search.catTv') : t('search.catMovie') }}
                    </el-tag>
                    <span v-if="row.episodes" class="ep">{{ row.episodes }}</span>
                    <span class="src">{{ row.source }}:{{ row.source_id }}</span>
                  </div>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('transfer.col.mode')" width="80">
            <template #default="{ row }">{{ modeLabel[row.mode] || row.mode }}</template>
          </el-table-column>
          <el-table-column :label="t('transfer.col.dest')" prop="dest" min-width="200" show-overflow-tooltip />
          <el-table-column :label="t('transfer.col.error')" min-width="280" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="row.status === 'failed'" class="error-msg">{{
                row.error_msg || t('transfer.unknownError')
              }}</span>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('reseed.col.status')" width="90">
            <template #default="{ row }">
              <el-tag
                size="small"
                :type="row.status === 'success' ? 'success' : row.status === 'skipped' ? 'info' : 'danger'"
              >
                {{
                  row.status === 'success'
                    ? t('transfer.st.success')
                    : row.status === 'skipped'
                      ? t('transfer.st.skipped')
                      : t('transfer.st.failed')
                }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('transfer.col.time')" width="150">
            <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
          </el-table-column>
          <el-table-column :label="t('reseed.col.actions')" width="130" fixed="right">
            <template #default="{ row }">
              <el-button size="small" link @click="redoRow(row as TransferHistory)" :loading="redoing === row.id">{{
                t('transfer.redo')
              }}</el-button>
              <el-button size="small" link type="danger" @click="deleteRow(row as TransferHistory)">{{
                t('common.delete')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <EmptyState
          v-if="!loading && !filtered.length"
          icon="files"
          :title="t('transfer.noRecords')"
          :sub="t('transfer.noRecordsSub')"
        />
      </div>
    </div>

    <TransferConfigDialog
      v-model:show-config="showConfig"
      v-model:cfg="cfg"
      v-model:cats-str="catsStr"
      v-model:skip-str="skipStr"
      :cloud-drives="cloudDrives"
      :downloaders="downloaders"
      :saving-cfg="savingCfg"
      :rename-vars="renameVars"
      :preview-rename="previewRename"
      @open-categories="openCategories"
      @open-identify="openIdentify"
      @open-tags="openTags"
      @open-dir-sync="openDirSync"
      @insert-var="(f: string, k: string) => insertVar(f as any, k)"
      @add-path-mapping="addPathMapping"
      @remove-path-mapping="removePathMapping"
      @save-config="saveConfig"
    />

    <TransferMoreDialogs
      v-model:show-dir-sync="showDirSync"
      v-model:show-tags="showTags"
      v-model:show-identify="showIdentify"
      v-model:show-categories="showCategories"
      v-model:show-dirwatch="showDirwatch"
      v-model:show-organize="showOrganize"
      v-model:sync-tasks="syncTasks"
      v-model:dir-tags="dirTags"
      v-model:sort-rules="sortRules"
      v-model:identify-words="identifyWords"
      v-model:dw-form="dwForm"
      v-model:shared-repos="sharedRepos"
      v-model:cat-movie-text="catMovieText"
      v-model:cat-tv-text="catTVText"
      v-model:sel-dl="selDl"
      v-model:sel-hash="selHash"
      :completed-torrents="completedTorrents"
      :downloaders="downloaders"
      :loading="loading"
      :syncing-name="syncingName"
      :sync-saving="syncSaving"
      :tags-saving="tagsSaving"
      :identify-saving="identifySaving"
      :recognize-testing="recognizeTesting"
      :identify-test-name="identifyTestName"
      :identify-test-result="identifyTestResult"
      :cat-saving="catSaving"
      :dw-saving="dwSaving"
      :organizing="organizing"
      :loading-torrents="loadingTorrents"
      :share-importing="shareImporting"
      :share-preview="sharePreview"
      :list="list"
      @run-sync="(r: any) => runSync(r)"
      @add-sync-task="addSyncTask"
      @save-sync-tasks="saveSyncTasks"
      @save-tags-and-rules="saveTagsAndRules"
      @add-identify-word="addIdentifyWord"
      @remove-identify-word="removeIdentifyWord"
      @test-identify="testIdentify"
      @test-recognize="testRecognize"
      @save-identify="saveIdentify"
      @save-categories="saveCategories"
      @save-dirwatch="saveDirwatch"
      @add-shared-repo="addSharedRepo"
      @remove-shared-repo="removeSharedRepo"
      @import-shared-repo="(...a: unknown[]) => importSharedRepo(a[0] as string)"
      @save-shared-repos="saveSharedRepos"
      @do-organize="doOrganize"
      @load-torrents="loadTorrents"
    />
  </div>
</template>

<style scoped>
.page {
  padding: 16px;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
  gap: var(--sp-3);
}
.header h1 {
  font-size: var(--fs-title);
  margin: 0;
}
.sub {
  font-size: var(--fs-xs);
  opacity: 0.7;
  margin: 4px 0 0;
}
.actions {
  display: flex;
  gap: var(--sp-2);
  align-items: center;
}
.glass.card {
  padding: 12px;
  border-radius: var(--radius-md);
}
.media-cell {
  display: flex;
  gap: var(--sp-2);
  align-items: center;
}
.poster {
  width: 38px;
  height: 54px;
  object-fit: cover;
  border-radius: var(--radius-2xs);
  flex-shrink: 0;
}
.title {
  font-weight: 600;
}
.year {
  font-weight: 400;
  opacity: 0.7;
}
.meta {
  display: flex;
  gap: var(--sp-2);
  align-items: center;
  margin-top: 3px;
  font-size: var(--fs-2xs);
  opacity: 0.7;
}
.organize-bar {
  display: flex;
  gap: var(--sp-2);
  margin-bottom: 10px;
}
.torrent-list {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  width: 100%;
}
.torrent-item {
  width: 100%;
  padding: 6px 8px;
  border-radius: var(--radius-xs);
  transition:
    background var(--dur-fast) var(--ease-out-smooth),
    color var(--dur-fast) var(--ease-out-smooth),
    border-color var(--dur-fast) var(--ease-out-smooth);
}
.torrent-item:hover {
  background: var(--glass-hover, rgba(255, 255, 255, 0.06));
}
.torrent-name {
  font-size: var(--fs-body);
}
.torrent-sub {
  font-size: var(--fs-2xs);
  opacity: 0.6;
}
.rename-editor {
  width: 100%;
}
.var-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
  margin-bottom: 6px;
}
.rename-preview {
  font-size: var(--fs-xs);
  opacity: 0.8;
  margin-top: 4px;
}
.rename-preview code {
  background: var(--glass-hover, rgba(127, 127, 127, 0.15));
  padding: 1px 6px;
  border-radius: var(--radius-2xs);
}
.form-hint {
  margin-left: 12px;
  font-size: var(--fs-xs);
  opacity: 0.7;
}
.unit {
  margin-left: 8px;
  font-size: var(--fs-xs);
  opacity: 0.7;
}
.path-mappings {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  width: 100%;
}
.path-mapping-row {
  display: grid;
  grid-template-columns: 130px minmax(140px, 1fr) 18px minmax(160px, 1fr) 32px;
  gap: var(--sp-1);
  align-items: center;
}
.mapping-arrow {
  text-align: center;
  opacity: 0.6;
}
.error-msg {
  color: var(--el-color-danger);
}
</style>
