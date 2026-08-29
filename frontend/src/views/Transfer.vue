<script setup lang="ts">
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

    <!-- 配置抽屉 -->
    <el-dialog v-model="showConfig" :title="t('transfer.config')" width="760px">
      <el-form :model="cfg" label-width="120px" size="small">
        <el-form-item :label="t('rss.f.enabled')"><el-switch v-model="cfg.enabled" /></el-form-item>
        <el-form-item :label="t('transfer.f.mode')">
          <el-select v-model="cfg.mode" style="width: 100%">
            <el-option :label="t('transfer.mode.link')" value="link" />
            <el-option :label="t('transfer.mode.softlink')" value="softlink" />
            <el-option :label="t('transfer.mode.move')" value="move" />
            <el-option :label="t('transfer.mode.copy')" value="copy" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('transfer.f.movieDir')"
          ><el-input v-model="cfg.movie_dir" placeholder="/media/movies"
        /></el-form-item>
        <el-form-item :label="t('transfer.f.tvDir')"
          ><el-input v-model="cfg.tv_dir" placeholder="/media/tv"
        /></el-form-item>
        <el-form-item :label="t('transfer.f.cats')"
          ><el-input v-model="catsStr" placeholder="subscribe,workflow"
        /></el-form-item>
        <el-form-item :label="t('transfer.f.skipTags')"
          ><el-input v-model="skipStr" placeholder="brushflow:,keepseed,reseed"
        /></el-form-item>
        <el-form-item :label="t('transfer.f.organizeTag')"><el-input v-model="cfg.organize_tag" /></el-form-item>
        <el-form-item :label="t('transfer.f.retryMinutes')">
          <el-input-number
            v-model="cfg.failed_retry_minutes"
            :min="10"
            :max="10080"
            :step="60"
            controls-position="right"
          />
          <span class="unit">{{ t('transfer.minutes') }}</span>
        </el-form-item>
        <el-form-item :label="t('transfer.f.pathMappings')">
          <div class="path-mappings">
            <div v-for="(mapping, index) in cfg.path_mappings" :key="index" class="path-mapping-row">
              <el-select v-model="mapping.downloader_id" class="mapping-downloader">
                <el-option :label="t('transfer.f.allDownloaders')" :value="0" />
                <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
              </el-select>
              <el-input v-model="mapping.remote_path" :placeholder="t('transfer.f.remotePath')" />
              <span class="mapping-arrow">→</span>
              <el-input v-model="mapping.local_path" :placeholder="t('transfer.f.localPath')" />
              <el-button
                circle
                :icon="Delete"
                :title="t('transfer.f.removeMapping')"
                @click="removePathMapping(index)"
              />
            </div>
            <el-button :icon="Plus" @click="addPathMapping">{{ t('transfer.f.addMapping') }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('transfer.f.typeFolder')"><el-switch v-model="cfg.type_folder" /></el-form-item>
        <el-form-item :label="t('transfer.f.scrape')">
          <el-select v-model="cfg.scrape" style="width: 100%">
            <el-option :label="t('transfer.scrape.missingOnly')" value="missing_only" />
            <el-option :label="t('transfer.scrape.overwrite')" value="overwrite" />
            <el-option :label="t('transfer.scrape.skip')" value="skip" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('transfer.f.refreshLibrary')"><el-switch v-model="cfg.refresh_library" /></el-form-item>
        <el-divider content-position="left">{{ t('transfer.f.cloudUpload') }}</el-divider>
        <el-form-item :label="t('transfer.f.enableUpload')">
          <el-switch v-model="cfg.cloud_upload.enabled" />
          <span class="form-hint">{{ t('transfer.f.uploadHint') }}</span>
        </el-form-item>
        <template v-if="cfg.cloud_upload.enabled">
          <el-form-item :label="t('transfer.f.targetDrive')">
            <el-select v-model="cfg.cloud_upload.drive_id" style="width: 100%">
              <el-option v-for="d in cloudDrives" :key="d.id" :label="`${d.name}（${d.type}）`" :value="d.id" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('transfer.f.cloudBase')">
            <el-input v-model="cfg.cloud_upload.cloud_base" :placeholder="t('transfer.f.cloudBaseHint')" />
          </el-form-item>
          <el-form-item :label="t('transfer.f.uploadVideos')"
            ><el-switch v-model="cfg.cloud_upload.upload_videos"
          /></el-form-item>
          <el-form-item :label="t('transfer.f.strmDir')">
            <el-input v-model="cfg.cloud_upload.strm_output_dir" :placeholder="t('transfer.f.strmDirHint')" />
          </el-form-item>
          <el-form-item :label="t('transfer.f.strmTemplate')">
            <el-input v-model="cfg.cloud_upload.strm_template" placeholder="{direct}{cloudpath}" />
            <span class="form-hint">{{ t('transfer.f.strmTemplateHint') }}</span>
          </el-form-item>
          <el-form-item :label="t('transfer.f.deleteLocal')">
            <el-switch v-model="cfg.cloud_upload.delete_local" />
            <span class="form-hint">{{ t('transfer.f.deleteLocalHint') }}</span>
          </el-form-item>
        </template>
        <el-form-item :label="t('transfer.f.overwrite')"><el-switch v-model="cfg.overwrite_files" /></el-form-item>
        <el-form-item :label="t('transfer.f.bestVersion')">
          <el-switch v-model="cfg.best_version" />
          <span class="form-hint">{{ t('transfer.f.bestVersionHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('transfer.f.categoryFolder')">
          <el-switch v-model="cfg.category_folder" />
          <el-button link type="primary" style="margin-left: 12px" @click="openCategories">{{
            t('transfer.editCategories')
          }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.identifyWords')">
          <el-button link type="primary" @click="openIdentify">{{ t('transfer.editIdentify') }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.dirTags')">
          <el-button link type="primary" @click="openTags">{{ t('transfer.editTags') }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.dirSync')">
          <el-button link type="primary" @click="openDirSync">{{ t('transfer.editDirSync') }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.movieRename')">
          <div class="rename-editor">
            <div class="var-chips">
              <el-button v-for="v in renameVars" :key="v.key" size="small" @click="insertVar('movie_rename', v.key)">{{
                v.label
              }}</el-button>
            </div>
            <el-input v-model="cfg.movie_rename" type="textarea" :rows="2" />
            <div class="rename-preview">
              {{ t('transfer.preview') }}<code>{{ previewRename(cfg.movie_rename, false) }}</code>
            </div>
          </div>
        </el-form-item>
        <el-form-item :label="t('transfer.f.tvRename')">
          <div class="rename-editor">
            <div class="var-chips">
              <el-button v-for="v in renameVars" :key="v.key" size="small" @click="insertVar('tv_rename', v.key)">{{
                v.label
              }}</el-button>
            </div>
            <el-input v-model="cfg.tv_rename" type="textarea" :rows="2" />
            <div class="rename-preview">
              {{ t('transfer.preview') }}<code>{{ previewRename(cfg.tv_rename, true) }}</code>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConfig = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="savingCfg" @click="saveConfig">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 目录同步任务 -->
    <el-dialog v-model="showDirSync" :title="t('transfer.dirSyncTitle')" width="760px" top="6vh">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        {{ t('transfer.dirSyncDesc') }}
      </el-alert>
      <el-table :data="syncTasks" size="small" :empty-text="t('transfer.noSyncTasks')">
        <el-table-column :label="t('rss.f.name')" width="110">
          <template #default="{ row }"><el-input v-model="row.name" size="small" /></template>
        </el-table-column>
        <el-table-column :label="t('transfer.col.srcDir')">
          <template #default="{ row }"
            ><el-input v-model="row.src_dir" size="small" placeholder="/downloads/media"
          /></template>
        </el-table-column>
        <el-table-column :label="t('transfer.col.dstDir')">
          <template #default="{ row }"
            ><el-input v-model="row.dst_dir" size="small" placeholder="/media/movies"
          /></template>
        </el-table-column>
        <el-table-column :label="t('transfer.col.method')" width="110">
          <template #default="{ row }">
            <el-select v-model="row.mode" size="small">
              <el-option :label="t('transfer.mode.shortLink')" value="link" /><el-option
                :label="t('transfer.mode.shortSoftlink')"
                value="softlink"
              />
              <el-option :label="t('transfer.mode.shortCopy')" value="copy" /><el-option
                :label="t('transfer.mode.shortMove')"
                value="move"
              />
              <el-option label="STRM" value="strm" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column :label="t('transfer.col.recursive')" width="60">
          <template #default="{ row }"><el-switch v-model="row.recursive" size="small" /></template>
        </el-table-column>
        <el-table-column :label="t('reseed.col.actions')" width="130">
          <template #default="{ row, $index }">
            <el-button
              size="small"
              link
              type="primary"
              :loading="syncingName === row.name"
              @click="runSync(row as DirSyncTask)"
              >{{ t('transfer.run') }}</el-button
            >
            <el-button size="small" link type="danger" @click="syncTasks.splice($index, 1)">{{
              t('transfer.del')
            }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div style="margin: 10px 0">
        <el-button size="small" @click="addSyncTask">{{ t('transfer.addTask') }}</el-button>
      </div>
      <template #footer>
        <el-button @click="showDirSync = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="syncSaving" @click="saveSyncTasks">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 目录标签 & 排序规则 -->
    <el-dialog v-model="showTags" :title="t('transfer.tagsTitle')" width="720px" top="6vh">
      <el-divider content-position="left">{{ t('transfer.dirTagsDivider') }}</el-divider>
      <el-table :data="dirTags" size="small" :empty-text="t('transfer.noTags')">
        <el-table-column :label="t('transfer.col.tagName')" width="120">
          <template #default="{ row }"><el-input v-model="row.name" size="small" placeholder="movies" /></template>
        </el-table-column>
        <el-table-column :label="t('mediaserver.col.type')" width="100">
          <template #default="{ row }">
            <el-select v-model="row.kind" size="small">
              <el-option :label="t('transfer.kind.local')" value="local" /><el-option
                :label="t('transfer.kind.cloud')"
                value="cloud"
              />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column :label="t('mediaserver.col.path')">
          <template #default="{ row }"
            ><el-input v-model="row.path" size="small" :placeholder="t('transfer.f.tagPathHint')"
          /></template>
        </el-table-column>
        <el-table-column :label="t('reseed.col.actions')" width="60">
          <template #default="{ $index }">
            <el-button size="small" link type="danger" @click="dirTags.splice($index, 1)">{{
              t('transfer.del')
            }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div style="margin: 8px 0">
        <el-button size="small" @click="dirTags.push({ name: '', kind: 'local', path: '' })">{{
          t('transfer.addTag')
        }}</el-button>
      </div>
      <el-divider content-position="left">{{ t('transfer.sortRulesDivider') }}</el-divider>
      <el-form label-width="130px" size="small">
        <el-form-item :label="t('transfer.f.resScore')">
          <el-input-number v-model="sortRules.resolution['2160p']" size="small" /> /
          <el-input-number v-model="sortRules.resolution['1080p']" size="small" />
        </el-form-item>
        <el-form-item :label="t('transfer.f.targetSize')">
          <el-input-number v-model="sortRules.target_size_gb" size="small" :min="0" />
          <span class="form-hint">{{ t('transfer.f.targetSizeHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('transfer.f.seedersWeight')">
          <el-input-number v-model="sortRules.seeders_weight" size="small" />
          <span class="form-hint">{{ t('transfer.f.seedersWeightHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('transfer.f.freeBonus')">
          <el-input-number v-model="sortRules.free_bonus" size="small" />
        </el-form-item>
        <el-form-item :label="t('transfer.f.recentBonus')">
          <el-input-number v-model="sortRules.recent_days" size="small" /> {{ t('transfer.daysPlus') }}
          <el-input-number v-model="sortRules.recent_bonus" size="small" /> {{ t('transfer.points') }}
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showTags = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="tagsSaving" @click="saveTagsAndRules">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 识别词 -->
    <el-dialog v-model="showIdentify" :title="t('transfer.identifyTitle')" width="720px" top="6vh">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        {{ t('transfer.identifyDesc') }}
      </el-alert>
      <el-table :data="identifyWords" size="small" :empty-text="t('transfer.noIdentifyWords')">
        <el-table-column :label="t('mediaserver.col.type')" width="110">
          <template #default="{ row }">
            <el-select v-model="row.kind" size="small">
              <el-option :label="t('transfer.identify.replace')" value="replace" />
              <el-option :label="t('transfer.identify.trim')" value="trim" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column :label="t('transfer.col.match')">
          <template #default="{ row }"
            ><el-input v-model="row.from" size="small" :placeholder="t('transfer.f.matchHint')"
          /></template>
        </el-table-column>
        <el-table-column :label="t('transfer.col.replaceWith')">
          <template #default="{ row }"
            ><el-input v-model="row.to" size="small" :placeholder="t('transfer.f.emptyDelete')"
          /></template>
        </el-table-column>
        <el-table-column :label="t('transfer.col.note')" width="140">
          <template #default="{ row }"><el-input v-model="row.note" size="small" /></template>
        </el-table-column>
        <el-table-column :label="t('reseed.col.actions')" width="70">
          <template #default="{ $index }">
            <el-button size="small" link type="danger" @click="removeIdentifyWord($index)">{{
              t('transfer.del')
            }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div style="margin: 10px 0">
        <el-button size="small" @click="addIdentifyWord">{{ t('common.add') }}</el-button>
      </div>
      <el-divider content-position="left">{{ t('transfer.testDivider') }}</el-divider>
      <div style="display: flex; gap: 8px; align-items: center">
        <el-input v-model="identifyTestName" size="small" :placeholder="t('transfer.f.testNameHint')" />
        <el-button size="small" @click="testIdentify">{{ t('transfer.normalizeTest') }}</el-button>
        <el-button size="small" type="primary" :loading="recognizeTesting" @click="testRecognize">{{
          t('transfer.recognizeTest')
        }}</el-button>
      </div>
      <div v-if="identifyTestResult" class="rename-preview" style="margin-top: 8px">{{ identifyTestResult }}</div>

      <el-divider content-position="left">{{ t('transfer.sharedRepoDivider') }}</el-divider>
      <div class="small text-muted" style="margin-bottom: 6px">
        {{ t('transfer.sharedRepoDesc') }}
      </div>
      <div
        v-for="(r, i) in sharedRepos"
        :key="i"
        style="display: flex; gap: 6px; margin-bottom: 6px; align-items: center"
      >
        <el-input v-model="r.name" size="small" style="width: 130px" :placeholder="t('rss.f.name')" />
        <el-input v-model="r.url" size="small" placeholder="https://.../identify-words.json" />
        <el-button size="small" :loading="shareImporting" @click="importSharedRepo(r.url)">{{
          t('transfer.previewImport')
        }}</el-button>
        <el-button size="small" link type="danger" @click="removeSharedRepo(i)">{{ t('transfer.del') }}</el-button>
      </div>
      <div style="display: flex; gap: 8px">
        <el-button size="small" @click="addSharedRepo">{{ t('transfer.addRepo') }}</el-button>
        <el-button size="small" @click="saveSharedRepos">{{ t('transfer.saveRepos') }}</el-button>
        <span v-if="sharePreview" class="small text-muted">{{
          t('transfer.repoStats', { total: sharePreview.total, new: sharePreview.new })
        }}</span>
      </div>
      <template #footer>
        <el-button @click="showIdentify = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="identifySaving" @click="saveIdentify">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 自动分类规则 -->
    <el-dialog v-model="showCategories" :title="t('transfer.categoriesTitle')" width="640px">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        {{ t('transfer.categoriesDesc') }}
      </el-alert>
      <el-form label-width="70px">
        <el-form-item :label="t('search.catMovie')">
          <el-input v-model="catMovieText" type="textarea" :rows="8" :placeholder="t('transfer.f.catMovieHint')" />
        </el-form-item>
        <el-form-item :label="t('search.catTv')">
          <el-input v-model="catTVText" type="textarea" :rows="8" :placeholder="t('transfer.f.catTVHint')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCategories = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="catSaving" @click="saveCategories">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 目录监视 -->
    <el-dialog v-model="showDirwatch" :title="t('transfer.dirwatch')" width="520px">
      <el-form label-width="110px">
        <el-form-item :label="t('rss.f.enabled')"><el-switch v-model="dwForm.enabled" /></el-form-item>
        <el-form-item :label="t('transfer.f.watchDirs')">
          <el-input v-model="dwForm.dirsText" type="textarea" :rows="4" :placeholder="t('transfer.f.watchDirsHint')" />
        </el-form-item>
        <el-form-item :label="t('mediaserver.col.type')">
          <el-select v-model="dwForm.monitor_mode">
            <el-option :label="t('transfer.watchMode.auto')" value="auto" />
            <el-option :label="t('transfer.watchMode.polling')" value="compatibility" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('transfer.f.debounce')"
          ><el-input-number v-model="dwForm.debounce_ms" :min="0" :step="500"
        /></el-form-item>
        <el-form-item :label="t('transfer.f.pollInterval')"
          ><el-input-number v-model="dwForm.poll_interval_sec" :min="0"
        /></el-form-item>
        <p class="text-muted" style="font-size: var(--fs-xs)">{{ t('transfer.dirwatchNote') }}</p>
      </el-form>
      <template #footer>
        <el-button @click="showDirwatch = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="dwSaving" @click="saveDirwatch">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 手动整理 -->
    <el-dialog v-model="showOrganize" :title="t('transfer.organize')" width="640px">
      <div class="organize-bar">
        <el-select v-model="selDl" size="small" style="width: 200px" @change="loadTorrents">
          <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
        <el-button size="small" @click="loadTorrents" :loading="loadingTorrents">{{
          t('transfer.refreshTorrents')
        }}</el-button>
      </div>
      <el-radio-group v-model="selHash" v-loading="loadingTorrents" class="torrent-list">
        <el-radio v-for="t in completedTorrents" :key="t.hash" :value="t.hash" class="torrent-item">
          <div class="torrent-name">{{ t.name }}</div>
          <div class="torrent-sub">{{ t.category }} · {{ t.hash.slice(0, 12) }}</div>
        </el-radio>
        <EmptyState
          v-if="!loadingTorrents && !completedTorrents.length"
          icon="files"
          :title="t('transfer.noCompleted')"
          :sub="t('transfer.noCompletedSub')"
        />
      </el-radio-group>
      <template #footer>
        <el-button @click="showOrganize = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="organizing" @click="doOrganize">{{
          t('transfer.organizeAction')
        }}</el-button>
      </template>
    </el-dialog>
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
