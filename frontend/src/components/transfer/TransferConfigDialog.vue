<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// Transfer.vue 的整理配置对话框：模式/目录/云上传/路径映射/重命名模板（从 Transfer.vue 拆出）。
// cfg/catsStr/skipStr 经 defineModel 双向绑定，动作经 emit 上抛。
import { Delete, Plus } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import type { TransferConfig } from '@/api/transfer'

defineModel<boolean>('showConfig', { default: false })
defineModel<TransferConfig>('cfg', { required: true })
defineModel<string>('catsStr', { default: '' })
defineModel<string>('skipStr', { default: '' })

withDefaults(
  defineProps<{
    cloudDrives: { id: number; name: string; type?: string }[]
    downloaders: { id: number; name: string }[]
    loadFailed?: string[]
    savingCfg: boolean
    renameVars: { key: string; label: string }[]
    previewRename: (tpl: string, isTv: boolean) => string
  }>(),
  { cloudDrives: () => [], downloaders: () => [], loadFailed: () => [], renameVars: () => [] },
)

const emit = defineEmits<{
  'update:showConfig': [v: boolean]
  'update:catsStr': [v: string]
  'update:skipStr': [v: string]
  addPathMapping: []
  removePathMapping: [i: number]
  saveConfig: []
  openCategories: []
  openIdentify: []
  openTags: []
  openDirSync: []
  insertVar: [field: string, key: string]
}>()

const { t } = useI18n()
</script>

<template>
    <!-- 配置抽屉 -->
    <el-dialog :model-value="showConfig"
      @update:model-value="emit('update:showConfig', $event)" :title="t('transfer.config')" width="760px">
      <el-form :model="cfg" label-width="120px" size="small">
        <el-form-item v-if="loadFailed.length" :label="t('common.loadFailed')">
          <span class="load-failed">{{
            loadFailed
              .map((f) => (f === 'downloader' ? t('downloaders.title') : f === 'cloud' ? t('transfer.f.targetDrive') : t('transfer.config')))
              .join('、')
          }}</span>
        </el-form-item>
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
          ><el-input :model-value="catsStr" @update:model-value="emit('update:catsStr', $event)" placeholder="subscribe,workflow"
        /></el-form-item>
        <el-form-item :label="t('transfer.f.skipTags')"
          ><el-input :model-value="skipStr" @update:model-value="emit('update:skipStr', $event)" placeholder="brushflow:,keepseed,reseed"
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
                @click="emit('removePathMapping', index)"
              />
            </div>
            <el-button :icon="Plus" @click="emit('addPathMapping')">{{ t('transfer.f.addMapping') }}</el-button>
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
          <el-button link type="primary" style="margin-left: 12px" @click="emit('openCategories')">{{
            t('transfer.editCategories')
          }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.identifyWords')">
          <el-button link type="primary" @click="emit('openIdentify')">{{ t('transfer.editIdentify') }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.dirTags')">
          <el-button link type="primary" @click="emit('openTags')">{{ t('transfer.editTags') }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.dirSync')">
          <el-button link type="primary" @click="emit('openDirSync')">{{ t('transfer.editDirSync') }}</el-button>
        </el-form-item>
        <el-form-item :label="t('transfer.f.movieRename')">
          <div class="rename-editor">
            <div class="var-chips">
              <el-button v-for="v in renameVars" :key="v.key" size="small" @click="emit('insertVar', 'movie_rename', v.key)">{{
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
              <el-button v-for="v in renameVars" :key="v.key" size="small" @click="emit('insertVar', 'tv_rename', v.key)">{{
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
        <el-button @click="emit('update:showConfig', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="savingCfg" @click="emit('saveConfig')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
</template>
