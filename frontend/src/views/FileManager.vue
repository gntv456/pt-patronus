<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { fileApi, type FileEntry } from '@/api/filemanager'
import {
  Folder,
  Document,
  Top,
  Refresh,
  Upload,
  FolderAdd,
  Download,
  Edit,
  Rank as MoveIcon,
  CopyDocument,
  Delete,
  Link,
  ArrowDown,
} from '@element-plus/icons-vue'
import EmptyState from '@/components/EmptyState.vue'
import FileManagerSidebar from '@/components/FileManagerSidebar.vue'
import FileManagerDialogs from '@/components/FileManagerDialogs.vue'
import FileManagerExtraDialogs from '@/components/FileManagerExtraDialogs.vue'
import { useFileManager, type StorageItem } from '@/composables/useFileManager'
import { useFileOps } from '@/composables/useFileOps'
import { useFileExtraOps } from '@/composables/useFileExtraOps'

const { t } = useI18n()
const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const fm = useFileManager()
const ops = useFileOps(fm)

// Re-expose for template
const {
  list,
  loading,
  current,
  statusMap,
  testing,
  path,
  entries,
  bloading,
  viewMode,
  selected,
  uploadProgress,
  storageDialog,
  editingId,
  form,
  agents,
  mkdirVisible,
  mkdirName,
  renameVisible,
  renameNewName,
  moveCopyVisible,
  moveCopyMode,
  moveCopyPath,
  moveCopyEntries,
  moveCopyLoading,
  strmVisible,
  strmDir,
  strmForm,
  strmLoading,
  submitting,
  editVisible,
  editTarget,
  editContent,
  editLoading,
  archiveVisible,
  archiveTarget,
  archiveName,
  archiveFmt,
  archiveLoading,
  chmodVisible,
  chmodTarget,
  chmodMode,
  chmodRecursive,
  chmodLoading,
  dirSizeMap,
  dirSizeLoading,
  previewVisible,
  previewTarget,
  previewContent,
  previewLoading,
  curFs,
  curReadOnly,
  crumbs,
  batchDelProgress,
  itemKey,
  loadStorages,
  testOne,
  statusOf,
  selectStorage,
  refresh,
  onSelectionChange,
  enter,
  jump,
  up,
  dlUrl,
} = fm
const extra = useFileExtraOps(fm, refresh)
const {
  onStorageMenu,
  openCreate,
  openEdit,
  submit,
  delEditingStorage,
  doMkdir,
  handleUpload,
  openRename,
  doRename,
  delOne,
  delBatch,
  cancelBatchDel,
  openMoveCopy,
  moveCopyUp,
  moveCopyEnter,
  doMoveCopy,
  openStrm,
  doStrm,
  toggleView,
  toggleSelect,
  isSelected,
  fmtBytes,
  itemIcon,
  itemLabel,
} = ops
const { openEditFile, doSaveFile, openArchive, doArchive, doExtract, openChmod, doChmod, calcDirSize, openPreview } =
  extra

function onFileExtra(cmd: string, e: FileEntry) {
  if (cmd === 'edit') openEditFile(e)
  else if (cmd === 'archive') openArchive(e)
  else if (cmd === 'extract') doExtract(e)
  else if (cmd === 'chmod') openChmod(e)
  else if (cmd === 'dirsize') calcDirSize(e)
  else if (cmd === 'preview') openPreview(e)
}
function supportsFsCmds(): boolean {
  const fs = curFs.value
  return !!fs && (fs.type === 'local' || fs.type === 'agent')
}
function isArchive(e: FileEntry): boolean {
  return /\.(zip|tar\.gz|tgz)$/i.test(e.name)
}

onMounted(async () => {
  await Promise.all([fm.loadStorages(), fm.loadAgents()])
})
watch(
  () => storageDialog.value,
  (v) => {
    if (v) fm.loadAgents()
  },
)
</script>

<template>
  <div class="fm-page" :class="{ embedded: props.embedded }">
    <!-- 左栏 -->
    <FileManagerSidebar
      :list="list"
      :loading="loading"
      :current="current"
      :item-key="itemKey"
      :item-icon="itemIcon"
      :item-label="itemLabel"
      :status-of="statusOf"
      @select="selectStorage"
      @test="testOne"
      @menu="onStorageMenu"
      @refresh="loadStorages"
      @create="openCreate"
    />

    <!-- 右栏 -->
    <section class="fm-main">
      <template v-if="current">
        <div class="fm-toolbar">
          <el-button size="small" :icon="Top" :disabled="path === '/'" @click="up">{{ t('filemanager.up') }}</el-button>
          <div class="crumbs">
            <template v-for="(c, i) in crumbs" :key="i">
              <span class="crumb" @click="jump(c.path)">{{ c.name }}</span>
              <span v-if="i < crumbs.length - 1" class="crumb-sep">/</span>
            </template>
          </div>
          <div class="tb-actions">
            <el-button v-if="!curReadOnly" size="small" :icon="FolderAdd" @click="mkdirVisible = true">{{
              t('filemanager.newFolder')
            }}</el-button>
            <el-upload v-if="!curReadOnly" :show-file-list="false" :http-request="handleUpload">
              <el-button size="small" type="primary" :icon="Upload" :disabled="uploadProgress >= 0">{{
                t('filemanager.upload')
              }}</el-button>
            </el-upload>
            <el-button
              size="small"
              @click="toggleView"
              :title="viewMode === 'list' ? t('filemanager.grid') : t('filemanager.list')"
              >{{ viewMode === 'list' ? '▤' : '☰' }}</el-button
            >
            <el-button size="small" :icon="Refresh" circle @click="refresh" />
          </div>
        </div>
        <el-progress
          v-if="uploadProgress >= 0"
          :percentage="uploadProgress"
          :stroke-width="6"
          style="margin: 0 12px 8px"
        />

        <transition name="fade">
          <div v-if="selected.length && !curReadOnly" class="batch-bar glass">
            <span>{{ t('subscriptions.batch.selected', { n: selected.length }) }}</span>
            <el-button size="small" :icon="MoveIcon" @click="openMoveCopy('move', selected)">{{
              t('filemanager.move')
            }}</el-button>
            <el-button size="small" :icon="CopyDocument" @click="openMoveCopy('copy', selected)">{{
              t('filemanager.copy')
            }}</el-button>
            <el-button size="small" type="danger" plain :icon="Delete" @click="delBatch">{{
              t('common.delete')
            }}</el-button>
            <el-button size="small" text @click="selected = []">{{ t('common.cancel') }}</el-button>
          </div>
        </transition>

        <!-- 批量删除进度 -->
        <transition name="fade">
          <div v-if="batchDelProgress" class="batch-del-progress glass">
            <span>{{ t('filemanager.deleting') }} {{ batchDelProgress.done }}/{{ batchDelProgress.total }}</span>
            <el-progress
              :percentage="Math.round((batchDelProgress.done / batchDelProgress.total) * 100)"
              :stroke-width="14"
              style="flex: 1; margin: 0 12px"
            />
            <el-button size="small" type="warning" plain @click="cancelBatchDel">{{ t('assistant.stop') }}</el-button>
          </div>
        </transition>

        <div v-loading="bloading" class="fm-files">
          <el-table
            v-if="viewMode === 'list'"
            :data="entries"
            :empty-text="t('filemanager.dirEmpty')"
            @selection-change="onSelectionChange"
            style="width: 100%"
          >
            <el-table-column v-if="!curReadOnly" type="selection" width="40" />
            <el-table-column :label="t('filemanager.col.name')" min-width="220">
              <template #default="{ row }">
                <a v-if="row.is_dir" class="link" @click="enter(row as FileEntry)"
                  ><el-icon><Folder /></el-icon>&nbsp;{{ row.name }}</a
                >
                <span v-else
                  ><el-icon><Document /></el-icon>&nbsp;{{ row.name }}</span
                >
              </template>
            </el-table-column>
            <el-table-column :label="t('rss.col.size')" width="120"
              ><template #default="{ row }"
                ><span v-if="row.is_dir">{{ dirSizeMap[row.path] || '-' }}</span
                ><span v-else>{{ fmtBytes(row.size) }}</span></template
              ></el-table-column
            >
            <el-table-column :label="curReadOnly ? t('reseed.col.actions') : ''" :width="curReadOnly ? 130 : 280">
              <template #default="{ row }">
                <el-button
                  v-if="row.is_dir && curReadOnly"
                  size="small"
                  link
                  :icon="Link"
                  @click="openStrm(row as FileEntry)"
                  >{{ t('filemanager.genStrm') }}</el-button
                >
                <template v-else-if="!curReadOnly">
                  <el-button
                    v-if="!row.is_dir"
                    size="small"
                    link
                    :icon="Download"
                    tag="a"
                    :href="dlUrl(row as FileEntry)"
                    target="_blank"
                    >{{ t('harem.down') }}</el-button
                  >
                  <el-button size="small" link :icon="Edit" @click="openRename(row as FileEntry)">{{
                    t('filemanager.rename')
                  }}</el-button>
                  <el-button size="small" link :icon="MoveIcon" @click="openMoveCopy('move', [row as FileEntry])">{{
                    t('filemanager.move')
                  }}</el-button>
                  <el-button size="small" link :icon="CopyDocument" @click="openMoveCopy('copy', [row as FileEntry])">{{
                    t('filemanager.copy')
                  }}</el-button>
                  <el-button size="small" link type="danger" :icon="Delete" @click="delOne(row as FileEntry)">{{
                    t('common.delete')
                  }}</el-button>
                  <el-dropdown v-if="curFs" trigger="click" @command="(cmd) => onFileExtra(cmd, row as FileEntry)">
                    <el-button size="small" link
                      >{{ t('filemanager.more') }}<el-icon class="el-icon--right"><ArrowDown /></el-icon
                    ></el-button>
                    <template #dropdown
                      ><el-dropdown-menu>
                        <el-dropdown-item v-if="!row.is_dir" command="preview">{{
                          t('filemanager.preview')
                        }}</el-dropdown-item>
                        <el-dropdown-item v-if="!row.is_dir" command="edit">{{ t('common.edit') }}</el-dropdown-item>
                        <el-dropdown-item v-if="supportsFsCmds()" command="archive">{{
                          t('filemanager.archive')
                        }}</el-dropdown-item>
                        <el-dropdown-item v-if="isArchive(row as FileEntry)" command="extract">{{
                          t('filemanager.extract')
                        }}</el-dropdown-item>
                        <el-dropdown-item v-if="supportsFsCmds()" command="chmod">{{
                          t('harem.permission')
                        }}</el-dropdown-item>
                        <el-dropdown-item v-if="row.is_dir" command="dirsize">{{
                          t('filemanager.calcSize')
                        }}</el-dropdown-item>
                      </el-dropdown-menu></template
                    >
                  </el-dropdown>
                </template>
              </template>
            </el-table-column>
          </el-table>

          <div v-else class="grid-view">
            <div
              v-a11y-button
              v-for="e in entries"
              :key="e.path"
              class="grid-card glass glass-hover"
              :class="{ sel: !curReadOnly && !e.is_dir && isSelected(e) }"
              @click="e.is_dir ? enter(e) : !curReadOnly && toggleSelect(e)"
            >
              <div v-if="!curReadOnly && !e.is_dir" class="grid-check" :class="{ on: isSelected(e) }">
                {{ isSelected(e) ? '✓' : '' }}
              </div>
              <div class="grid-icon">{{ e.is_dir ? '📁' : '🎬' }}</div>
              <div class="grid-name" :title="e.name">{{ e.name }}</div>
              <div class="grid-meta">{{ e.is_dir ? t('filemanager.folder') : fmtBytes(e.size) }}</div>
              <el-button
                v-if="e.is_dir && curReadOnly"
                size="small"
                link
                class="grid-strm"
                :icon="Link"
                @click.stop="openStrm(e)"
                >STRM</el-button
              >
            </div>
            <div v-if="!entries.length" class="grid-empty">{{ t('filemanager.dirEmpty') }}</div>
          </div>
        </div>
      </template>

      <div v-else class="fm-empty">
        <EmptyState
          icon="📂"
          :title="list.length ? t('filemanager.selectStorageTitle') : t('filemanager.noStorageTitle2')"
          :description="list.length ? t('filemanager.selectStorageDesc') : t('filemanager.noStorageDesc2')"
        />
      </div>
    </section>

    <!-- 对话框组 -->
    <FileManagerDialogs
      v-model:storage-dialog="storageDialog"
      v-model:editing-id="editingId"
      v-model:form="form"
      v-model:agents="agents"
      v-model:submitting="submitting"
      v-model:mkdir-visible="mkdirVisible"
      v-model:mkdir-name="mkdirName"
      v-model:rename-visible="renameVisible"
      v-model:rename-new-name="renameNewName"
      v-model:move-copy-visible="moveCopyVisible"
      v-model:move-copy-mode="moveCopyMode"
      v-model:move-copy-path="moveCopyPath"
      v-model:move-copy-loading="moveCopyLoading"
      v-model:move-copy-entries="moveCopyEntries"
      v-model:strm-visible="strmVisible"
      v-model:strm-dir="strmDir"
      v-model:strm-form="strmForm"
      v-model:strm-loading="strmLoading"
      @submit="submit"
      @del-editing-storage="delEditingStorage"
      @do-mkdir="doMkdir"
      @do-rename="doRename"
      @move-copy-up="moveCopyUp"
      @move-copy-enter="moveCopyEnter"
      @do-move-copy="doMoveCopy"
      @do-strm="doStrm"
    />
    <FileManagerExtraDialogs
      v-model:edit-visible="editVisible"
      v-model:edit-content="editContent"
      :edit-target="editTarget"
      :edit-loading="editLoading"
      v-model:archive-visible="archiveVisible"
      v-model:archive-name="archiveName"
      v-model:archive-fmt="archiveFmt"
      :archive-target="archiveTarget"
      :archive-loading="archiveLoading"
      v-model:chmod-visible="chmodVisible"
      v-model:chmod-mode="chmodMode"
      v-model:chmod-recursive="chmodRecursive"
      :chmod-target="chmodTarget"
      :chmod-loading="chmodLoading"
      v-model:preview-visible="previewVisible"
      :preview-target="previewTarget"
      :preview-content="previewContent"
      :preview-loading="previewLoading"
      :storage-id="curFs?.id ?? 0"
      @save="doSaveFile"
      @archive="doArchive"
      @chmod="doChmod"
    />
  </div>
</template>

<style scoped src="./filemanager.css"></style>
