<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { fileApi, type FileEntry } from '@/api/filemanager'
import {
  Folder, Document, Top, Plus, Refresh, Upload, FolderAdd,
  Download, Edit, Rank as MoveIcon, CopyDocument, Delete, Link, ArrowDown,
} from '@element-plus/icons-vue'
import EmptyState from '@/components/EmptyState.vue'
import FileManagerDialogs from '@/components/FileManagerDialogs.vue'
import FileManagerExtraDialogs from '@/components/FileManagerExtraDialogs.vue'
import { useFileManager, type StorageItem } from '@/composables/useFileManager'
import { useFileOps } from '@/composables/useFileOps'
import { useFileExtraOps } from '@/composables/useFileExtraOps'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const fm = useFileManager()
const ops = useFileOps(fm)

// Re-expose for template
const {
  list, loading, current, statusMap, testing, path, entries, bloading, viewMode, selected, uploadProgress,
  storageDialog, editingId, form, agents,
  mkdirVisible, mkdirName, renameVisible, renameNewName,
  moveCopyVisible, moveCopyMode, moveCopyPath, moveCopyEntries, moveCopyLoading,
  strmVisible, strmDir, strmForm, strmLoading, submitting,
  editVisible, editTarget, editContent, editLoading,
  archiveVisible, archiveTarget, archiveName, archiveFmt, archiveLoading,
  chmodVisible, chmodTarget, chmodMode, chmodRecursive, chmodLoading,
  dirSizeMap, dirSizeLoading, previewVisible, previewTarget, previewContent, previewLoading,
  curFs, curReadOnly, crumbs,
  itemKey, loadStorages, testOne, statusOf, selectStorage, refresh, onSelectionChange, enter, jump, up, dlUrl,
} = fm
const extra = useFileExtraOps(fm, refresh)
const {
  onStorageMenu, openCreate, openEdit, submit, delEditingStorage,
  doMkdir, handleUpload, openRename, doRename, delOne, delBatch,
  openMoveCopy, moveCopyUp, moveCopyEnter, doMoveCopy, openStrm, doStrm,
  toggleView, toggleSelect, isSelected, fmtBytes, itemIcon, itemLabel,
} = ops
const {
  openEditFile, doSaveFile, openArchive, doArchive, doExtract, openChmod, doChmod, calcDirSize, openPreview,
} = extra

function onFileExtra(cmd: string, e: FileEntry) {
  if (cmd === 'edit') openEditFile(e)
  else if (cmd === 'archive') openArchive(e)
  else if (cmd === 'extract') doExtract(e)
  else if (cmd === 'chmod') openChmod(e)
  else if (cmd === 'dirsize') calcDirSize(e)
  else if (cmd === 'preview') openPreview(e)
}
function isArchive(e: FileEntry): boolean {
  return /\.(zip|tar\.gz|tgz)$/i.test(e.name)
}

onMounted(async () => {
  await Promise.all([fm.loadStorages(), fm.loadAgents()])
})
watch(() => storageDialog.value, (v) => { if (v) fm.loadAgents() })
</script>

<template>
  <div class="fm-page" :class="{ embedded: props.embedded }">
    <!-- 左栏 -->
    <aside class="fm-sidebar">
      <div class="sb-head">
        <span>存储</span>
        <el-button size="small" :icon="Refresh" circle :loading="loading" @click="loadStorages" />
      </div>
      <div v-loading="loading" class="sb-list">
        <div
          v-for="it in list"
          :key="itemKey(it)"
          class="sb-item"
          :class="{ active: current && itemKey(current) === itemKey(it) }"
          @click="selectStorage(it)"
        >
          <span class="sb-icon">{{ itemIcon(it) }}</span>
          <div class="sb-main">
            <div class="sb-name">{{ it.kind === 'fs' ? it.fs.name : it.drive.name }}</div>
            <div class="sb-type">{{ itemLabel(it) }}<span v-if="it.kind === 'cloud'" class="ro">·只读</span></div>
          </div>
          <span class="dot" :class="statusOf(it)" :title="statusOf(it) === 'fail' ? '连接失败' : statusOf(it) === 'ok' ? '连接正常' : '检测中…'" @click.stop="testOne(it)"></span>
          <el-dropdown trigger="click" placement="bottom-end" @command="(c) => onStorageMenu(c as string, it)">
            <span class="sb-more" @click.stop>⋯</span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="test">测试连接</el-dropdown-item>
                <template v-if="it.kind === 'fs'">
                  <el-dropdown-item command="edit">编辑</el-dropdown-item>
                  <el-dropdown-item command="delete" divided>删除存储</el-dropdown-item>
                </template>
                <el-dropdown-item v-else disabled>云盘请在「云盘」页管理</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
        <div v-if="!list.length && !loading" class="sb-empty">
          <EmptyState icon="📁" title="还没有存储" description="接入本机、WebDAV、SMB、Agent 或云盘后即可浏览文件。" />
        </div>
      </div>
      <div class="sb-actions">
        <el-button :icon="Plus" @click="openCreate">添加存储</el-button>
      </div>
    </aside>

    <!-- 右栏 -->
    <section class="fm-main">
      <template v-if="current">
        <div class="fm-toolbar">
          <el-button size="small" :icon="Top" :disabled="path === '/'" @click="up">上级</el-button>
          <div class="crumbs">
            <template v-for="(c, i) in crumbs" :key="i">
              <span class="crumb" @click="jump(c.path)">{{ c.name }}</span>
              <span v-if="i < crumbs.length - 1" class="crumb-sep">/</span>
            </template>
          </div>
          <div class="tb-actions">
            <el-button v-if="!curReadOnly" size="small" :icon="FolderAdd" @click="mkdirVisible = true">新建文件夹</el-button>
            <el-upload v-if="!curReadOnly" :show-file-list="false" :http-request="handleUpload">
              <el-button size="small" type="primary" :icon="Upload" :disabled="uploadProgress >= 0">上传</el-button>
            </el-upload>
            <el-button size="small" @click="toggleView" :title="viewMode === 'list' ? '网格' : '列表'">{{ viewMode === 'list' ? '▤' : '☰' }}</el-button>
            <el-button size="small" :icon="Refresh" circle @click="refresh" />
          </div>
        </div>
        <el-progress v-if="uploadProgress >= 0" :percentage="uploadProgress" :stroke-width="6" style="margin: 0 12px 8px" />

        <transition name="fade">
          <div v-if="selected.length && !curReadOnly" class="batch-bar glass">
            <span>已选 {{ selected.length }} 项</span>
            <el-button size="small" :icon="MoveIcon" @click="openMoveCopy('move', selected)">移动</el-button>
            <el-button size="small" :icon="CopyDocument" @click="openMoveCopy('copy', selected)">复制</el-button>
            <el-button size="small" type="danger" plain :icon="Delete" @click="delBatch">删除</el-button>
            <el-button size="small" text @click="selected = []">取消</el-button>
          </div>
        </transition>

        <div v-loading="bloading" class="fm-files">
          <el-table v-if="viewMode === 'list'" :data="entries" empty-text="该目录为空" @selection-change="onSelectionChange" style="width: 100%">
            <el-table-column v-if="!curReadOnly" type="selection" width="40" />
            <el-table-column label="名称" min-width="220">
              <template #default="{ row }">
                <a v-if="row.is_dir" class="link" @click="enter(row as FileEntry)"><el-icon><Folder /></el-icon>&nbsp;{{ row.name }}</a>
                <span v-else><el-icon><Document /></el-icon>&nbsp;{{ row.name }}</span>
              </template>
            </el-table-column>
            <el-table-column label="大小" width="120"><template #default="{ row }"><span v-if="row.is_dir">{{ dirSizeMap[row.path] || '-' }}</span><span v-else>{{ fmtBytes(row.size) }}</span></template></el-table-column>
            <el-table-column :label="curReadOnly ? '操作' : ''" :width="curReadOnly ? 130 : 280">
              <template #default="{ row }">
                <el-button v-if="row.is_dir && curReadOnly" size="small" link :icon="Link" @click="openStrm(row as FileEntry)">生成 STRM</el-button>
                <template v-else-if="!curReadOnly">
                  <el-button v-if="!row.is_dir" size="small" link :icon="Download" tag="a" :href="dlUrl(row as FileEntry)" target="_blank">下载</el-button>
                  <el-button size="small" link :icon="Edit" @click="openRename(row as FileEntry)">重命名</el-button>
                  <el-button size="small" link :icon="MoveIcon" @click="openMoveCopy('move', [row as FileEntry])">移动</el-button>
                  <el-button size="small" link :icon="CopyDocument" @click="openMoveCopy('copy', [row as FileEntry])">复制</el-button>
                  <el-button size="small" link type="danger" :icon="Delete" @click="delOne(row as FileEntry)">删除</el-button>
                  <el-dropdown v-if="curFs && (curFs.type === 'local' || curFs.type === 'agent')" trigger="click" @command="(cmd) => onFileExtra(cmd, row as FileEntry)">
                    <el-button size="small" link>更多<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
                    <template #dropdown><el-dropdown-menu>
                      <el-dropdown-item v-if="!row.is_dir" command="preview">预览</el-dropdown-item>
                      <el-dropdown-item v-if="!row.is_dir" command="edit">编辑</el-dropdown-item>
                      <el-dropdown-item command="archive">压缩</el-dropdown-item>
                      <el-dropdown-item v-if="isArchive(row as FileEntry)" command="extract">解压</el-dropdown-item>
                      <el-dropdown-item command="chmod">权限</el-dropdown-item>
                      <el-dropdown-item v-if="row.is_dir" command="dirsize">计算大小</el-dropdown-item>
                    </el-dropdown-menu></template>
                  </el-dropdown>
                </template>
              </template>
            </el-table-column>
          </el-table>

          <div v-else class="grid-view">
            <div v-for="e in entries" :key="e.path" class="grid-card glass glass-hover" :class="{ sel: !curReadOnly && !e.is_dir && isSelected(e) }" @click="e.is_dir ? enter(e) : (!curReadOnly && toggleSelect(e))">
              <div class="grid-icon">{{ e.is_dir ? '📁' : '🎬' }}</div>
              <div class="grid-name" :title="e.name">{{ e.name }}</div>
              <div class="grid-meta">{{ e.is_dir ? '文件夹' : fmtBytes(e.size) }}</div>
              <el-button v-if="e.is_dir && curReadOnly" size="small" link class="grid-strm" :icon="Link" @click.stop="openStrm(e)">STRM</el-button>
            </div>
            <div v-if="!entries.length" class="grid-empty">该目录为空</div>
          </div>
        </div>
      </template>

      <div v-else class="fm-empty">
        <EmptyState icon="📂" :title="list.length ? '请选择左侧存储' : '还没有接入存储'" :description="list.length ? '点击左侧任一存储开始浏览。' : '点击左下角「添加存储」接入。'" />
      </div>
    </section>

    <!-- 对话框组 -->
    <FileManagerDialogs
      v-model:storage-dialog="storageDialog" v-model:editing-id="editingId" v-model:form="form" v-model:agents="agents" v-model:submitting="submitting"
      v-model:mkdir-visible="mkdirVisible" v-model:mkdir-name="mkdirName" v-model:rename-visible="renameVisible" v-model:rename-new-name="renameNewName"
      v-model:move-copy-visible="moveCopyVisible" v-model:move-copy-mode="moveCopyMode" v-model:move-copy-path="moveCopyPath" v-model:move-copy-loading="moveCopyLoading" v-model:move-copy-entries="moveCopyEntries"
      v-model:strm-visible="strmVisible" v-model:strm-dir="strmDir" v-model:strm-form="strmForm" v-model:strm-loading="strmLoading"
      @submit="submit" @del-editing-storage="delEditingStorage" @do-mkdir="doMkdir" @do-rename="doRename"
      @move-copy-up="moveCopyUp" @move-copy-enter="moveCopyEnter" @do-move-copy="doMoveCopy" @do-strm="doStrm"
    />
    <FileManagerExtraDialogs
      v-model:edit-visible="editVisible" v-model:edit-content="editContent" :edit-target="editTarget" :edit-loading="editLoading"
      v-model:archive-visible="archiveVisible" v-model:archive-name="archiveName" v-model:archive-fmt="archiveFmt" :archive-target="archiveTarget" :archive-loading="archiveLoading"
      v-model:chmod-visible="chmodVisible" v-model:chmod-mode="chmodMode" v-model:chmod-recursive="chmodRecursive" :chmod-target="chmodTarget" :chmod-loading="chmodLoading"
      v-model:preview-visible="previewVisible" :preview-target="previewTarget" :preview-content="previewContent" :preview-loading="previewLoading" :storage-id="curFs?.id ?? 0"
      @save="doSaveFile" @archive="doArchive" @chmod="doChmod"
    />
  </div>
</template>

<style scoped>
.fm-page { display: flex; gap: 14px; height: calc(100vh - 120px); min-height: 480px; }
.fm-sidebar { flex: 0 0 220px; display: flex; flex-direction: column; gap: 10px; }
.sb-head { display: flex; align-items: center; justify-content: space-between; font-weight: 700; font-size: 14px; padding: 0 4px; }
.sb-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; padding: 6px; border: 1px solid var(--glass-border); border-radius: 10px; background: color-mix(in srgb, var(--glass-bg) 86%, transparent); }
.sb-item { display: flex; align-items: center; gap: 10px; padding: 9px 10px; border-radius: 8px; cursor: pointer; transition: background 0.15s; }
.sb-item:hover { background: color-mix(in srgb, var(--pt-primary, #4361ee) 10%, transparent); }
.sb-item.active { background: linear-gradient(135deg, color-mix(in srgb, var(--pt-primary, #6366f1) 22%, transparent), color-mix(in srgb, var(--pt-primary2, #4361ee) 16%, transparent)); }
.sb-icon { font-size: 20px; }
.sb-main { flex: 1; min-width: 0; }
.sb-name { font-weight: 600; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sb-type { font-size: 11px; color: var(--text-muted, #909399); text-transform: uppercase; }
.sb-type .ro { color: #f59e0b; text-transform: none; margin-left: 3px; }
.sb-empty { padding: 24px 8px; }
.sb-actions { display: flex; flex-direction: column; gap: 6px; }
.sb-actions .el-button { width: 100%; }
.dot { width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0; cursor: pointer; }
.dot.checking { background: var(--text-muted, #909399); opacity: 0.5; }
.dot.ok { background: #2ecc71; box-shadow: 0 0 4px rgba(46, 204, 113, 0.6); }
.dot.fail { background: #e74c3c; }
.sb-more { cursor: pointer; padding: 0 2px; font-size: 18px; line-height: 1; color: var(--text-muted, #909399); flex-shrink: 0; }
.sb-more:hover { color: var(--pt-primary, #4361ee); }
.fm-main { flex: 1; min-width: 0; display: flex; flex-direction: column; border: 1px solid var(--glass-border); border-radius: 10px; background: color-mix(in srgb, var(--glass-bg) 86%, transparent); overflow: hidden; }
.fm-toolbar { display: flex; align-items: center; gap: 10px; padding: 10px 12px; border-bottom: 1px solid var(--glass-border); flex-wrap: wrap; }
.crumbs { flex: 1; min-width: 0; display: flex; align-items: center; gap: 4px; overflow-x: auto; padding: 2px 0; }
.crumbs::-webkit-scrollbar { height: 4px; }
.crumb { color: var(--pt-primary, #4361ee); cursor: pointer; white-space: nowrap; font-size: 13px; }
.crumb:hover { text-decoration: underline; }
.crumb-sep { color: var(--text-muted, #909399); }
.tb-actions { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
.batch-bar { display: flex; align-items: center; gap: 10px; padding: 8px 14px; margin: 10px 12px 0; border-radius: 8px; font-size: 13px; }
.fm-files { flex: 1; overflow-y: auto; padding: 8px 12px 12px; }
.link { color: var(--pt-primary, #4361ee); cursor: pointer; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) { background-color: transparent !important; }
.grid-view { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 12px; padding: 6px 0; }
.grid-card { position: relative; padding: 14px 10px; border-radius: 10px; cursor: pointer; text-align: center; transition: transform 0.12s, box-shadow 0.12s; }
.grid-card:hover { transform: translateY(-2px); }
.grid-card.sel { border-color: var(--pt-primary, #4361ee); box-shadow: 0 0 0 2px color-mix(in srgb, var(--pt-primary, #4361ee) 40%, transparent); }
.grid-icon { font-size: 34px; }
.grid-name { margin-top: 6px; font-size: 12px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.grid-meta { font-size: 11px; color: var(--text-muted, #909399); }
.grid-strm { margin-top: 6px; }
.grid-empty { grid-column: 1 / -1; text-align: center; padding: 40px; color: var(--text-muted, #909399); }
.fm-empty { flex: 1; display: grid; place-items: center; }
.fade-enter-active, .fade-leave-active { transition: opacity 0.18s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
@media (max-width: 760px) {
  .fm-page { flex-direction: column; height: auto; }
  .fm-sidebar { flex: none; }
  .sb-list { max-height: 200px; }
}
.fm-page.embedded { height: calc(100vh - 240px); min-height: 420px; }
</style>
