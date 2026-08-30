<script setup lang="ts">
// 表单对象经 props 传入并双向编辑(宿主 ref 直改深层字段),整文件豁免。
/* eslint-disable vue/no-mutating-props */
// MediaServer 的健康检查 + 添加/编辑服务器两个对话框（从 MediaServer.vue 拆出；行为不变）。
import { useI18n } from 'vue-i18n'

defineModel<boolean>('showHealth', { default: false })
defineModel<boolean>('healthSubtitles', { default: false })
defineModel<boolean>('healthUpgradeable', { default: false })
defineModel<boolean>('showForm', { default: false })

defineProps<{
  healthReport: any
  healthLoading: boolean
  fixingSubtitles: boolean
  fixSubtitles: (r: any) => void
  editing: boolean
  form: Record<string, any>
  loading: boolean
}>()

const emit = defineEmits<{
  runHealth: []
  fix: [r: any]
  save: []
  'update:showHealth': [v: boolean]
  'update:healthSubtitles': [v: boolean]
  'update:healthUpgradeable': [v: boolean]
  'update:showForm': [v: boolean]
}>()

const { t } = useI18n()
</script>

<template>
    <el-dialog :model-value="showHealth" @update:model-value="emit('update:showHealth', $event)" :title="t('mediaserver.healthTitle')" width="860px" top="6vh">
      <div class="libs-bar">
        <el-checkbox :model-value="healthSubtitles" @change="(v: any) => emit('update:healthSubtitles', v)" size="small">{{ t('mediaserver.scanSubtitles') }}</el-checkbox>
        <el-checkbox :model-value="healthUpgradeable" @change="(v: any) => emit('update:healthUpgradeable', v)" size="small">{{ t('mediaserver.scanUpgradeable') }}</el-checkbox>
        <el-button size="small" :loading="healthLoading" @click="emit('runHealth')">{{ t('mediaserver.recheck') }}</el-button>
        <el-button
          size="small"
          type="primary"
          :loading="fixingSubtitles"
          :disabled="!healthReport?.missing_subtitle"
          @click="fixSubtitles"
        >
          {{ t('mediaserver.fixMissing', { n: healthReport?.missing_subtitle ?? 0 }) }}
        </el-button>
      </div>
      <div v-loading="healthLoading">
        <div class="hint" style="margin-bottom: 8px" v-if="healthReport">
          {{ t('mediaserver.unmatched') }} {{ healthReport.unmatched }} · {{ t('mediaserver.missingEps') }}
          {{ healthReport.missing_episodes }} · {{ t('mediaserver.missingSubs') }} {{ healthReport.missing_subtitle }} ·
          {{ t('mediaserver.upgradeable') }} {{ healthReport.upgradeable ?? 0 }}
        </div>
        <el-table :data="healthReport?.issues ?? []" size="small" :empty-text="t('mediaserver.healthyEmpty')">
          <el-table-column :label="t('mediaserver.col.type')" width="100">
            <template #default="{ row }">
              <el-tag
                size="small"
                :type="
                  row.type === 'unmatched'
                    ? 'danger'
                    : row.type === 'missing_episodes'
                      ? 'warning'
                      : row.type === 'upgradeable'
                        ? 'success'
                        : 'info'
                "
              >
                {{
                  row.type === 'unmatched'
                    ? t('mediaserver.unmatched')
                    : row.type === 'missing_episodes'
                      ? t('mediaserver.missingEps')
                      : row.type === 'upgradeable'
                        ? t('mediaserver.upgradeable')
                        : t('mediaserver.missingSubs')
                }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="title" :label="t('rss.col.title')" min-width="150" show-overflow-tooltip />
          <el-table-column prop="library" :label="t('mediaserver.col.library')" width="110" show-overflow-tooltip />
          <el-table-column prop="detail" :label="t('mediaserver.col.detail')" min-width="160" show-overflow-tooltip />
          <el-table-column prop="path" :label="t('mediaserver.col.path')" min-width="200" show-overflow-tooltip />
        </el-table>
      </div>
    </el-dialog>


    <el-dialog
      :model-value="showForm" @update:model-value="emit('update:showForm', $event)"
      :title="editing ? t('mediaserver.editTitle') : t('mediaserver.addTitle')"
      width="560px"
      top="8vh"
    >
      <el-form :model="form" label-width="120px" label-position="right">
        <el-form-item :label="t('rss.f.name')"
          ><el-input v-model="form.name" :placeholder="t('mediaserver.f.nameHint')"
        /></el-form-item>
        <el-form-item :label="t('mediaserver.col.type')">
          <el-select v-model="form.type">
            <el-option label="Emby" value="emby" />
            <el-option label="Jellyfin" value="jellyfin" />
            <el-option label="Plex" value="plex" />
            <el-option :label="t('mediaserver.type.ugosFull')" value="ugos" />
            <el-option :label="t('mediaserver.type.fnosFull')" value="fnos" />
            <el-option :label="t('mediaserver.type.zspaceFull')" value="zspace" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('mediaserver.f.url')">
          <el-input v-model="form.url" placeholder="http://192.168.1.10:8096" />
        </el-form-item>
        <el-form-item label="API Key / Token">
          <el-input
            v-model="form.token"
            type="password"
            show-password
            :placeholder="editing ? t('mediaserver.f.tokenKeep') : t('mediaserver.f.tokenNew')"
          />
        </el-form-item>
        <el-form-item :label="t('mediaserver.f.usernameOptional')">
          <el-input v-model="form.username" :placeholder="t('mediaserver.f.usernameHint')" />
        </el-form-item>
        <el-form-item :label="t('mediaserver.f.setDefault')"><el-switch v-model="form.is_default" /></el-form-item>
        <el-form-item :label="t('mediaserver.refreshOnDownload')">
          <el-switch v-model="form.refresh_on_download" />
          <span class="hint">{{ t('mediaserver.f.refreshHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('mediaserver.notifyOnPlay')">
          <el-switch v-model="form.notify_on_play" />
          <span class="hint">{{ t('mediaserver.f.notifyHint') }}</span>
        </el-form-item>
        <el-divider content-position="left">{{ t('mediaserver.f.directLink') }}</el-divider>
        <el-form-item :label="t('mediaserver.f.enableDirect')">
          <el-switch v-model="form.direct_link_enabled" />
          <span class="hint">{{ t('mediaserver.f.directHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('mediaserver.f.directBase')">
          <el-input v-model="form.direct_link_base" placeholder="https://cdn.example.com/library" />
          <span class="hint">{{ t('mediaserver.f.directBaseHint') }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="emit('update:showForm', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="emit('save')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

</template>
