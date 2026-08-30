<script setup lang="ts">
// Transfer.vue 的设置类对话框组：目录同步/标签排序/识别词/分类规则/目录监视/手动整理。
// （从 Transfer.vue 拆出；弹窗开关与表单状态经 defineModel 双向绑定，动作经 emit 上抛。）
import { useI18n } from 'vue-i18n'
import type { DirSyncTask, SortRules, SharedRepo } from '@/api/transfer'

defineModel<boolean>('showDirSync', { default: false })
defineModel<boolean>('showTags', { default: false })
defineModel<boolean>('showIdentify', { default: false })
defineModel<boolean>('showCategories', { default: false })
defineModel<boolean>('showDirwatch', { default: false })
defineModel<boolean>('showOrganize', { default: false })

defineModel<DirSyncTask[]>('syncTasks', { required: true })
defineModel<object[]>('dirTags', { required: true })
defineModel<SortRules>('sortRules', { required: true })
defineModel<object[]>('identifyWords', { required: true })
defineModel<{ enabled: boolean; dirsText: string; monitor_mode: string; debounce_ms: number; poll_interval_sec: number }>('dwForm', { required: true })
defineModel<SharedRepo[]>('sharedRepos', { required: true })
defineModel<string>('catMovieText', { default: '' })
defineModel<string>('catTVText', { default: '' })
defineModel<number>('selDl', { default: 0 })
defineModel<string>('selHash', { default: '' })

withDefaults(
  defineProps<{
    completedTorrents: { hash: string; name: string; category?: string }[]
    downloaders: { id: number; name: string }[]
    loading: boolean
    syncingName: string
    syncSaving: boolean
    tagsSaving: boolean
    identifySaving: boolean
    recognizeTesting: boolean
    identifyTestName: string
    identifyTestResult: string
    catSaving: boolean
    dwSaving: boolean
    organizing: boolean
    loadingTorrents: boolean
    shareImporting: boolean
    sharePreview: { total: number; new: number } | null
    list: unknown[]
  }>(),
  { completedTorrents: () => [], downloaders: () => [], list: () => [] },
)

const emit = defineEmits<{
  'update:identifyTestName': [v: string]
  'update:catMovieText': [v: string]
  'update:catTVText': [v: string]
  'update:selDl': [v: number]
  'update:selHash': [v: string]
  'update:showDirSync': [v: boolean]
  'update:showTags': [v: boolean]
  'update:showIdentify': [v: boolean]
  'update:showCategories': [v: boolean]
  'update:showDirwatch': [v: boolean]
  'update:showOrganize': [v: boolean]
  runSync: [...args: unknown[]]
  addSyncTask: []
  saveSyncTasks: []
  saveTagsAndRules: []
  addIdentifyWord: []
  removeIdentifyWord: [i: number]
  testIdentify: []
  testRecognize: []
  saveIdentify: []
  saveCategories: []
  saveDirwatch: []
  addSharedRepo: []
  removeSharedRepo: [i: number]
  importSharedRepo: [...args: unknown[]]
  saveSharedRepos: []
  sharePreview: []
  doOrganize: []
  loadTorrents: []
}>()

const { t } = useI18n()
</script>

<template>
    <!-- 目录同步任务 -->
    <el-dialog :model-value="showDirSync"
      @update:model-value="emit('update:showDirSync', $event)" :title="t('transfer.dirSyncTitle')" width="760px" top="6vh">
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
              @click="emit('runSync', row as DirSyncTask)"
              >{{ t('transfer.run') }}</el-button
            >
            <el-button size="small" link type="danger" @click="syncTasks.splice($index, 1)">{{
              t('transfer.del')
            }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div style="margin: 10px 0">
        <el-button size="small" @click="emit('addSyncTask')">{{ t('transfer.addTask') }}</el-button>
      </div>
      <template #footer>
        <el-button @click="emit('update:showDirSync', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="syncSaving" @click="emit('saveSyncTasks')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 目录标签 & 排序规则 -->
    <el-dialog :model-value="showTags"
      @update:model-value="emit('update:showTags', $event)" :title="t('transfer.tagsTitle')" width="720px" top="6vh">
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
        <el-button @click="emit('update:showTags', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="tagsSaving" @click="emit('saveTagsAndRules')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 识别词 -->
    <el-dialog :model-value="showIdentify"
      @update:model-value="emit('update:showIdentify', $event)" :title="t('transfer.identifyTitle')" width="720px" top="6vh">
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
            <el-button size="small" link type="danger" @click="emit('removeIdentifyWord', $index)">{{
              t('transfer.del')
            }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div style="margin: 10px 0">
        <el-button size="small" @click="emit('addIdentifyWord')">{{ t('common.add') }}</el-button>
      </div>
      <el-divider content-position="left">{{ t('transfer.testDivider') }}</el-divider>
      <div style="display: flex; gap: 8px; align-items: center">
        <el-input :model-value="identifyTestName" @update:model-value="emit('update:identifyTestName', $event)" size="small" :placeholder="t('transfer.f.testNameHint')" />
        <el-button size="small" @click="emit('testIdentify')">{{ t('transfer.normalizeTest') }}</el-button>
        <el-button size="small" type="primary" :loading="recognizeTesting" @click="emit('testRecognize')">{{
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
        <el-button size="small" :loading="shareImporting" @click="emit('importSharedRepo')">{{
          t('transfer.previewImport')
        }}</el-button>
        <el-button size="small" link type="danger" @click="emit('removeSharedRepo', i)">{{ t('transfer.del') }}</el-button>
      </div>
      <div style="display: flex; gap: 8px">
        <el-button size="small" @click="emit('addSharedRepo')">{{ t('transfer.addRepo') }}</el-button>
        <el-button size="small" @click="emit('saveSharedRepos')">{{ t('transfer.saveRepos') }}</el-button>
        <span v-if="sharePreview" class="small text-muted">{{
          t('transfer.repoStats', { total: sharePreview.total, new: sharePreview.new })
        }}</span>
      </div>
      <template #footer>
        <el-button @click="emit('update:showIdentify', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="identifySaving" @click="emit('saveIdentify')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 自动分类规则 -->
    <el-dialog :model-value="showCategories"
      @update:model-value="emit('update:showCategories', $event)" :title="t('transfer.categoriesTitle')" width="640px">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        {{ t('transfer.categoriesDesc') }}
      </el-alert>
      <el-form label-width="70px">
        <el-form-item :label="t('search.catMovie')">
          <el-input :model-value="catMovieText" @update:model-value="emit('update:catMovieText', $event)" type="textarea" :rows="8" :placeholder="t('transfer.f.catMovieHint')" />
        </el-form-item>
        <el-form-item :label="t('search.catTv')">
          <el-input :model-value="catTVText" @update:model-value="emit('update:catTVText', $event)" type="textarea" :rows="8" :placeholder="t('transfer.f.catTVHint')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="emit('update:showCategories', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="catSaving" @click="emit('saveCategories')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 目录监视 -->
    <el-dialog :model-value="showDirwatch"
      @update:model-value="emit('update:showDirwatch', $event)" :title="t('transfer.dirwatch')" width="520px">
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
        <el-button @click="emit('update:showDirwatch', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="dwSaving" @click="emit('saveDirwatch')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 手动整理 -->
    <el-dialog :model-value="showOrganize"
      @update:model-value="emit('update:showOrganize', $event)" :title="t('transfer.organize')" width="640px">
      <div class="organize-bar">
        <el-select :model-value="selDl" @update:model-value="emit('update:selDl', Number($event))" size="small" style="width: 200px" @change="emit('loadTorrents')">
          <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
        <el-button size="small" @click="emit('loadTorrents')" :loading="loadingTorrents">{{
          t('transfer.refreshTorrents')
        }}</el-button>
      </div>
      <el-radio-group :model-value="selHash" @update:model-value="emit('update:selHash', String($event))" v-loading="loadingTorrents" class="torrent-list">
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
        <el-button @click="emit('update:showOrganize', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="organizing" @click="emit('doOrganize')">{{
          t('transfer.organizeAction')
        }}</el-button>
      </template>
    </el-dialog>
</template>
