<script setup lang="ts">
import { ref, watch } from 'vue'
import { Top, Folder } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { fileApi } from '@/api/filemanager'
import type { Agent } from '@/api/monitor'
import type { FileEntry } from '@/api/filemanager'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const storageDialog = defineModel<boolean>('storageDialog', { required: true })
const editingId = defineModel<number | null>('editingId', { required: true })
const form = defineModel<{
  name: string
  type: string
  url: string
  username: string
  password: string
  base_path: string
  agent_id: number
}>('form', { required: true })
const agents = defineModel<Agent[]>('agents', { required: true })
const agentsError = defineModel<string>('agentsError', { required: true })
const submitting = defineModel<boolean>('submitting', { required: true })
const mkdirVisible = defineModel<boolean>('mkdirVisible', { required: true })
const mkdirName = defineModel<string>('mkdirName', { required: true })
const renameVisible = defineModel<boolean>('renameVisible', { required: true })
const renameNewName = defineModel<string>('renameNewName', { required: true })
const moveCopyVisible = defineModel<boolean>('moveCopyVisible', { required: true })
const moveCopyMode = defineModel<'move' | 'copy'>('moveCopyMode', { required: true })
const moveCopyPath = defineModel<string>('moveCopyPath', { required: true })
const moveCopyLoading = defineModel<boolean>('moveCopyLoading', { required: true })
const moveCopyEntries = defineModel<FileEntry[]>('moveCopyEntries', { required: true })
const strmVisible = defineModel<boolean>('strmVisible', { required: true })
const strmDir = defineModel<FileEntry | null>('strmDir', { required: true })
const strmForm = defineModel<{
  output_dir: string
  mount_root: string
  template: string
  recursive: boolean
  keep_ext: boolean
}>('strmForm', { required: true })
const strmLoading = defineModel<boolean>('strmLoading', { required: true })

const emit = defineEmits<{
  (e: 'submit'): void
  (e: 'delEditingStorage'): void
  (e: 'doMkdir'): void
  (e: 'doRename'): void
  (e: 'moveCopyUp'): void
  (e: 'moveCopyEnter', d: FileEntry): void
  (e: 'doMoveCopy'): void
  (e: 'doStrm'): void
}>()

// 切换存储类型时重置类型相关字段（避免残留无效值）
watch(
  () => form.value.type,
  (newType, oldType) => {
    if (oldType && newType !== oldType && !editingId.value) {
      form.value.url = ''
      form.value.username = ''
      form.value.password = ''
      form.value.agent_id = 0
      form.value.base_path = '/'
    }
  },
)

// 测试连接（仅编辑已有存储时可用）
const testingConn = ref(false)
async function testConnection() {
  if (!editingId.value) {
    ElMessage.info(t('fmd.saveFirst'))
    return
  }
  testingConn.value = true
  try {
    const r = await fileApi.test(editingId.value)
    if (r.ok) ElMessage.success(t('mediaserver.connOk2'))
    else ElMessage.error(t('mediaserver.connFailed'))
  } catch (e) {
    ElMessage.error((e as Error).message || t('mediaserver.connFailed'))
  } finally {
    testingConn.value = false
  }
}
</script>

<template>
  <!-- 存储添加/编辑 -->
  <el-dialog
    class="pt-dialog"
    append-to-body
    align-center
    v-model="storageDialog"
    :title="editingId ? t('fmd.editStorage') : t('fmd.addStorage')"
    width="460"
  >
    <el-form label-width="80">
      <el-form-item :label="t('common.name')"><el-input v-model="form.name" /></el-form-item>
      <el-form-item :label="t('cloud.type')">
        <el-radio-group v-model="form.type">
          <el-radio value="local">{{ t('fmd.local') }}</el-radio>
          <el-radio value="webdav">WebDAV</el-radio>
          <el-radio value="alist">Alist</el-radio>
          <el-radio value="smb">SMB</el-radio>
          <el-radio value="rclone">Rclone</el-radio>
          <el-radio value="agent">Agent</el-radio>
        </el-radio-group>
        <div v-if="form.type === 'local'" class="hint">{{ t('fmd.localHint') }}</div>
        <div v-else-if="form.type === 'webdav'" class="hint">{{ t('fmd.webdavHint') }}</div>
        <div v-else-if="form.type === 'alist'" class="hint">{{ t('fmd.alistHint') }}</div>
        <div v-else-if="form.type === 'rclone'" class="hint">{{ t('fmd.rcloneHint') }}</div>
      </el-form-item>
      <template v-if="form.type === 'agent'">
        <el-form-item label="Agent">
          <el-select v-model="form.agent_id" :placeholder="t('fmd.pickAgent')" style="width: 100%">
            <el-option
              v-for="a in agents"
              :key="a.id"
              :label="`${a.name}${a.online ? '' : t('fmd.offline')}`"
              :value="a.id"
            />
          </el-select>
        </el-form-item>
        <div v-if="agentsError" class="hint" style="color: var(--el-color-danger)">
          {{ t('fmd.agentsReadFail', { reason: agentsError }) }}
        </div>
        <div class="hint">{{ t('fmd.agentHint') }}</div>
      </template>
      <template v-else-if="form.type === 'rclone'">
        <el-form-item :label="t('fmd.binary')"
          ><el-input v-model="form.url" :placeholder="t('fmd.binaryPh')"
        /></el-form-item>
        <el-form-item label="remote"
          ><el-input v-model="form.username" :placeholder="t('fmd.remotePh')"
        /></el-form-item>
        <el-form-item label="conf"
          ><el-input
            v-model="form.password"
            type="textarea"
            :rows="5"
            :placeholder="editingId ? t('fmd.keepPh') : t('fmd.pasteConf')"
        /></el-form-item>
      </template>
      <template v-else-if="form.type !== 'local'">
        <el-form-item :label="t('deviceform.address')"><el-input v-model="form.url" /></el-form-item>
        <el-form-item :label="t('deviceform.username')"><el-input v-model="form.username" /></el-form-item>
        <el-form-item :label="t('deviceform.password')"
          ><el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="editingId ? t('fmd.keepPh') : ''"
        /></el-form-item>
        <div v-if="form.type === 'smb'" class="hint">{{ t('fmd.smbHint') }}</div>
      </template>
      <el-form-item :label="t('fmd.rootPath')"><el-input v-model="form.base_path" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button v-if="editingId" type="danger" plain @click="emit('delEditingStorage')">{{
        t('fmd.deleteStorage')
      }}</el-button>
      <el-button @click="storageDialog = false">{{ t('common.cancel') }}</el-button>
      <el-button v-if="editingId" :loading="testingConn" @click="testConnection">{{ t('fmd.testConn') }}</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('submit')">{{
        editingId ? t('common.save') : t('common.add')
      }}</el-button>
    </template>
  </el-dialog>

  <!-- 新建文件夹 -->
  <el-dialog
    class="pt-dialog"
    append-to-body
    align-center
    v-model="mkdirVisible"
    :title="t('filemanager.newFolder')"
    width="360"
  >
    <el-input v-model="mkdirName" :placeholder="t('fmd.folderNamePh')" @keyup.enter="emit('doMkdir')" />
    <template #footer>
      <el-button @click="mkdirVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('doMkdir')">{{ t('fmd.create') }}</el-button>
    </template>
  </el-dialog>

  <!-- 重命名 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="renameVisible" :title="t('fmd.rename')" width="360">
    <el-input v-model="renameNewName" @keyup.enter="emit('doRename')" />
    <template #footer>
      <el-button @click="renameVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('doRename')">{{ t('common.confirm') }}</el-button>
    </template>
  </el-dialog>

  <!-- 移动/复制 -->
  <el-dialog
    class="pt-dialog"
    append-to-body
    align-center
    v-model="moveCopyVisible"
    :title="moveCopyMode === 'move' ? t('fmd.moveTo') : t('fmd.copyTo')"
    width="560"
  >
    <div class="mc-bar">
      <el-button size="small" :icon="Top" :disabled="moveCopyPath === '/'" @click="emit('moveCopyUp')">{{
        t('fmd.up')
      }}</el-button>
      <span class="mc-path">{{ moveCopyPath }}</span>
    </div>
    <div v-loading="moveCopyLoading" class="mc-list">
      <div v-a11y-button v-for="d in moveCopyEntries" :key="d.path" class="mc-item" @click="emit('moveCopyEnter', d)">
        <el-icon><Folder /></el-icon>&nbsp;{{ d.name }}
      </div>
      <div v-if="!moveCopyEntries.length && !moveCopyLoading" class="mc-empty">{{ t('fmd.noSubfolders') }}</div>
    </div>
    <template #footer>
      <el-button @click="moveCopyVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('doMoveCopy')">{{
        moveCopyMode === 'move' ? t('fmd.moveHere') : t('fmd.copyHere')
      }}</el-button>
    </template>
  </el-dialog>

  <!-- STRM 即时生成 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="strmVisible" :title="t('fmd.genStrm')" width="480">
    <div class="hint">{{ t('fmd.strmHint', { name: strmDir?.name }) }}</div>
    <el-form label-width="88" style="margin-top: 8px">
      <el-form-item :label="t('cloud.outputDir')"
        ><el-input v-model="strmForm.output_dir" :placeholder="t('fmd.strmOutPh')"
      /></el-form-item>
      <el-form-item :label="t('cloud.mountRoot')"
        ><el-input v-model="strmForm.mount_root" :placeholder="t('cloud.mountRootPh')"
      /></el-form-item>
      <el-form-item :label="t('cloud.template')"
        ><el-input v-model="strmForm.template" :placeholder="t('fmd.tplPh')"
      /></el-form-item>
      <el-form-item :label="t('cloud.recursive')"><el-switch v-model="strmForm.recursive" /></el-form-item>
      <el-form-item :label="t('cloud.keepExt')"><el-switch v-model="strmForm.keep_ext" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="strmVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="strmLoading" @click="emit('doStrm')">{{ t('cloud.generate') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint {
  margin: -4px 0 8px;
  padding: 8px 12px;
  font-size: var(--fs-xs);
  color: var(--text-muted, #909399);
  background: var(--pt-bg-soft, rgba(255, 255, 255, 0.04));
  border-radius: var(--radius-xs);
  line-height: 1.6;
}
.mc-bar {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: 10px;
}
.mc-path {
  flex: 1;
  font-family: monospace;
  font-size: var(--fs-body);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mc-list {
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-xs);
  padding: 6px;
}
.mc-item {
  display: flex;
  align-items: center;
  padding: 7px 10px;
  border-radius: var(--radius-xs);
  cursor: pointer;
  font-size: var(--fs-body);
  transition:
    background var(--dur-fast) var(--ease-out-smooth),
    color var(--dur-fast) var(--ease-out-smooth),
    border-color var(--dur-fast) var(--ease-out-smooth);
}
.mc-item:hover {
  background: color-mix(in srgb, var(--pt-primary, #4361ee) 10%, transparent);
}
.mc-empty {
  text-align: center;
  color: var(--text-muted, #909399);
  padding: 20px;
}
</style>
