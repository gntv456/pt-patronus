<script setup lang="ts">
// Cloud.vue 的对话框组：115 扫码授权/目录树导入/网盘表单/规则表单/浏览 picker。
// （从 Cloud.vue 拆出；仅展示与转发，全部状态经 defineModel 双向绑定、动作经 emit 上抛。）
import { useI18n } from 'vue-i18n'
import { TYPE_LABEL, type DriveInput, type RuleInput } from '@/api/cloud'

defineModel<boolean>('showAuth115', { default: false })
defineModel<boolean>('showImportTree', { default: false })
defineModel<boolean>('showDriveForm', { default: false })
defineModel<boolean>('showRuleForm', { default: false })
defineModel<boolean>('showBrowse', { default: false })
defineModel<string>('authAppId', { default: '' })
defineModel<{ drive_id: number; base: string; output_dir: string; tree: string }>('treeForm', { required: true })
defineModel<DriveInput>('driveForm', { required: true })
defineModel<RuleInput>('ruleForm', { required: true })

withDefaults(
  defineProps<{
    authState: unknown
    authQR: string
    authStatusText: string
    authStarting: boolean
    authPolling: boolean
    drives: { id: number; name: string; type: string }[]
    browseFiles: { path: string; is_dir: boolean; name: string; size?: number }[]
    browsePath: string
    browseLoading: boolean
    importing: boolean
    qrImage: (qr: string) => string
    editingDrive: unknown
    editingRule: unknown
    browseDrive: { name?: string } | null
    tokenLabel: string
    loading: boolean
    fmtSize: (...args: any[]) => string
  }>(),
  { drives: () => [], browseFiles: () => [] },
)

const emit = defineEmits<{
  'update:authAppId': [v: string]
  'update:showAuth115': [v: boolean]
  'update:showImportTree': [v: boolean]
  'update:showDriveForm': [v: boolean]
  'update:showRuleForm': [v: boolean]
  'update:showBrowse': [v: boolean]
  startAuth: []
  doImportTree: []
  saveDrive: []
  saveRule: []
  startBrowse: [fn: () => void]
  browseUp: []
  enterDir: [f: any]
  pickDir: []
  ruleBrowse: []
  doMkdir: []
  doOffline: []
  doShareSave: []
}>()

const { t } = useI18n()
</script>

<template>
    <!-- 115 开放平台扫码授权 -->
    <el-dialog
      class="pt-dialog"
      append-to-body
      align-center
      :model-value="showAuth115"
      @update:model-value="emit('update:showAuth115', $event)"
      :title="t('cloud.authTitle')"
      width="480px"
    >
      <el-form label-width="90px">
        <el-form-item label="AppID">
          <el-input :model-value="authAppId" @update:model-value="emit('update:authAppId', $event)" :placeholder="t('cloud.authAppIdPh')" :disabled="!!authState" />
        </el-form-item>
      </el-form>
      <div v-if="authQR" class="auth-qr">
        <img :src="qrImage(authQR)" :alt="t('cloud.qrAuth')" style="width: 220px; height: 220px" />
        <p class="hint">{{ authStatusText }}</p>
      </div>
      <template #footer>
        <el-button @click="emit('update:showAuth115', false)">{{ t('common.close') }}</el-button>
        <el-button type="primary" :loading="authStarting || authPolling" :disabled="!!authState" @click="emit('startAuth')">
          {{ authState ? t('cloud.polling') : t('cloud.genQr') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 115 目录树批量导入 -->
    <el-dialog
      class="pt-dialog"
      append-to-body
      align-center
      :model-value="showImportTree"
      @update:model-value="emit('update:showImportTree', $event)"
      :title="t('cloud.importTitle')"
      width="640px"
    >
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        {{ t('cloud.importTip') }}
      </el-alert>
      <el-form label-width="110px">
        <el-form-item :label="t('cloud.targetDrive')">
          <el-select v-model="treeForm.drive_id" style="width: 100%">
            <el-option
              v-for="d in drives"
              :key="d.id"
              :label="`${d.name}（${(TYPE_LABEL as Record<string, string>)[d.type] || d.type}）`"
              :value="d.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('cloud.treeRoot')">
          <el-input v-model="treeForm.base" :placeholder="t('cloud.treeRootPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.strmOutputDir')">
          <el-input v-model="treeForm.output_dir" placeholder="/media/strm" />
        </el-form-item>
        <el-form-item :label="t('cloud.treeText')">
          <el-input v-model="treeForm.tree" type="textarea" :rows="10" :placeholder="t('cloud.treePh')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="emit('update:showImportTree', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="importing" @click="emit('doImportTree')">{{ t('cloud.importGo') }}</el-button>
      </template>
    </el-dialog>

    <!-- 网盘表单 -->
    <el-dialog
      class="pt-dialog"
      append-to-body
      align-center
      :model-value="showDriveForm"
      @update:model-value="emit('update:showDriveForm', $event)"
      :title="editingDrive ? t('cloud.editDrive') : t('cloud.addDrive')"
      width="560px"
    >
      <el-form :model="driveForm" label-width="120px" label-position="right">
        <el-form-item :label="t('common.name')">
          <el-input v-model="driveForm.name" :placeholder="t('cloud.namePh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.type')">
          <el-select v-model="driveForm.type">
            <el-option :label="t('cloud.typeName.115Open')" value="115_open" />
            <el-option :label="t('cloud.typeName.115')" value="115" />
            <el-option :label="t('cloud.typeName.aliyunOpen')" value="aliyun_open" />
            <el-option :label="t('cloud.typeName.quark')" value="quark" />
            <el-option :label="t('cloud.typeName.123')" value="123" />
            <el-option label="OneDrive" value="onedrive" />
            <el-option :label="t('cloud.typeName.baidu')" value="baidu" />
            <el-option :label="t('cloud.typeName.tianyi')" value="tianyi" />
            <el-option :label="t('cloud.typeName.mobile')" value="mobile" />
          </el-select>
        </el-form-item>
        <el-form-item label="Cookie / Token">
          <el-input
            v-model="driveForm.token"
            type="textarea"
            :rows="3"
            :placeholder="editingDrive ? t('cloud.tokenKeepPh') : tokenLabel"
          />
          <span class="hint">{{ t('cloud.tokenHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('cloud.rootDir')">
          <el-input v-model="driveForm.root_path" :placeholder="t('cloud.rootPh')" />
          <span class="hint">{{ t('cloud.rootHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('cloud.setDefault')"><el-switch v-model="driveForm.is_default" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="emit('update:showDriveForm', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="emit('saveDrive')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 规则表单 -->
    <el-dialog
      class="pt-dialog"
      append-to-body
      align-center
      :model-value="showRuleForm"
      @update:model-value="emit('update:showRuleForm', $event)"
      :title="editingRule ? t('cloud.editRule') : t('cloud.addRule')"
      width="600px"
    >
      <el-form :model="ruleForm" label-width="110px" label-position="right">
        <el-form-item :label="t('common.name')">
          <el-input v-model="ruleForm.name" :placeholder="t('cloud.ruleNamePh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.drive')">
          <el-select v-model="ruleForm.drive_id" :disabled="!drives.length">
            <el-option v-for="d in drives" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('cloud.cloudDir')">
          <div style="display: flex; gap: var(--sp-1); width: 100%">
            <el-input v-model="ruleForm.cloud_path" :placeholder="t('cloud.cloudPathPh')" style="flex: 1" />
            <el-button @click="emit('ruleBrowse')">{{ t('cloud.browse') }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('cloud.outputDir')">
          <el-input v-model="ruleForm.output_dir" :placeholder="t('cloud.outputDirPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.outputTag')">
          <el-input v-model="ruleForm.output_tag" :placeholder="t('cloud.outputTagPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.mountRoot')">
          <el-input v-model="ruleForm.mount_root" :placeholder="t('cloud.mountRootPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.template')">
          <el-input v-model="ruleForm.template" :placeholder="t('cloud.templatePh')" />
          <span class="hint">{{ t('cloud.templateHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('cloud.localUrl')">
          <el-input v-model="ruleForm.local_base_url" :placeholder="t('cloud.localUrlPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.publicUrl')">
          <el-input v-model="ruleForm.public_base_url" :placeholder="t('cloud.publicUrlPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.extWhitelist')">
          <el-input v-model="ruleForm.include_exts" :placeholder="t('cloud.extWhitelistPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.excludeKw')">
          <el-input v-model="ruleForm.exclude_keywords" :placeholder="t('cloud.excludeKwPh')" />
        </el-form-item>
        <el-form-item :label="t('cloud.recursiveScan')"><el-switch v-model="ruleForm.recursive" /></el-form-item>
        <el-form-item :label="t('cloud.incremental')">
          <el-switch v-model="ruleForm.incremental" />
          <span class="hint">{{ t('cloud.incrementalHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('cloud.keepExt')">
          <el-switch v-model="ruleForm.keep_ext" />
          <span class="hint">{{ t('cloud.keepExtHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('cloud.enabled')"><el-switch v-model="ruleForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="emit('update:showRuleForm', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="emit('saveRule')">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 浏览 -->
    <el-dialog
      class="pt-dialog"
      append-to-body
      align-center
      :model-value="showBrowse"
      @update:model-value="emit('update:showBrowse', $event)"
      :title="t('cloud.browseTitle', { name: browseDrive?.name ?? '' })"
      width="640px"
    >
      <div class="tab-toolbar" style="margin-bottom: 8px">
        <el-button size="small" @click="emit('doMkdir')">{{ t('filemanager.newFolder') }}</el-button>
        <el-button size="small" @click="emit('doOffline')">{{ t('cloud.offline') }}</el-button>
        <el-button size="small" @click="emit('doShareSave')">{{ t('cloud.shareSave') }}</el-button>
      </div>
      <div class="browse-bar">
        <el-button size="small" :disabled="browsePath === '/'" @click="emit('browseUp')">{{ t('cloud.up') }}</el-button>
        <span class="browse-path">{{ browsePath }}</span>
      </div>
      <div v-loading="browseLoading" class="browse-list">
        <div
          v-a11y-button
          v-for="f in browseFiles.filter((x) => x.is_dir)"
          :key="f.path"
          class="browse-item"
          @click="emit('enterDir', f)"
        >
          📁 {{ f.name }}
        </div>
        <div v-for="f in browseFiles.filter((x) => !x.is_dir)" :key="f.path" class="browse-item file">
          🎬 {{ f.name }}
          <span class="meta">{{ fmtSize(f.size) }}</span>
        </div>
        <div v-if="!browseFiles.length && !browseLoading" class="meta" style="padding: 20px; text-align: center">
          {{ t('cloud.emptyDir') }}
        </div>
      </div>
      <template #footer>
        <el-button @click="emit('update:showBrowse', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="emit('pickDir')">{{ t('cloud.useThisDir') }}</el-button>
      </template>
    </el-dialog>
</template>
