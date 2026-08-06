<script setup lang="ts">
import { Top, Folder } from '@element-plus/icons-vue'
import type { Agent } from '@/api/monitor'
import type { FileEntry } from '@/api/filemanager'

const storageDialog = defineModel<boolean>('storageDialog', { required: true })
const editingId = defineModel<number | null>('editingId', { required: true })
const form = defineModel<{ name: string; type: string; url: string; username: string; password: string; base_path: string; agent_id: number }>('form', { required: true })
const agents = defineModel<Agent[]>('agents', { required: true })
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
const strmForm = defineModel<{ output_dir: string; mount_root: string; template: string; recursive: boolean; keep_ext: boolean }>('strmForm', { required: true })
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
</script>

<template>
  <!-- 存储添加/编辑 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="storageDialog" :title="editingId ? '编辑存储' : '添加存储'" width="460">
    <el-form label-width="80">
      <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
      <el-form-item label="类型">
        <el-radio-group v-model="form.type">
          <el-radio value="local">本机</el-radio>
          <el-radio value="webdav">WebDAV</el-radio>
          <el-radio value="alist">Alist</el-radio>
          <el-radio value="smb">SMB</el-radio>
          <el-radio value="rclone">Rclone</el-radio>
          <el-radio value="agent">Agent</el-radio>
        </el-radio-group>
        <div v-if="form.type === 'local'" class="hint">本机：直接读写服务所在机器的本地目录。根路径填绝对路径（如 <code>D:\Media</code> 或 <code>/mnt/nas</code>）。⚠️ 仅配置可信目录。</div>
        <div v-else-if="form.type === 'webdav'" class="hint">WebDAV：地址填完整 URL（如 http://host:5005）。</div>
        <div v-else-if="form.type === 'alist'" class="hint">Alist：地址填服务地址（如 http://host:5244），账密默认可留空。</div>
        <div v-else-if="form.type === 'rclone'" class="hint">Rclone：二进制路径填 url，remote 名填 username，密码框粘 rclone.conf 全文。</div>
      </el-form-item>
      <template v-if="form.type === 'agent'">
        <el-form-item label="Agent">
          <el-select v-model="form.agent_id" placeholder="选择 Agent" style="width: 100%">
            <el-option v-for="a in agents" :key="a.id" :label="`${a.name}${a.online ? '' : '（离线）'}`" :value="a.id" />
          </el-select>
        </el-form-item>
        <div class="hint">Agent：选择已部署 agent 的设备，根路径填 agent 机器上的绝对路径。</div>
      </template>
      <template v-else-if="form.type === 'rclone'">
        <el-form-item label="二进制"><el-input v-model="form.url" placeholder="rclone（留空用 PATH）" /></el-form-item>
        <el-form-item label="remote"><el-input v-model="form.username" placeholder="如 myremote" /></el-form-item>
        <el-form-item label="conf"><el-input v-model="form.password" type="textarea" :rows="5" :placeholder="editingId ? '留空不修改' : '粘贴 rclone.conf'" /></el-form-item>
      </template>
      <template v-else-if="form.type !== 'local'">
        <el-form-item label="地址"><el-input v-model="form.url" /></el-form-item>
        <el-form-item label="用户名"><el-input v-model="form.username" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="form.password" type="password" show-password :placeholder="editingId ? '留空不修改' : ''" /></el-form-item>
        <div v-if="form.type === 'smb'" class="hint">SMB：地址填 smb://主机[:端口]，根路径填 /共享名/子目录。</div>
      </template>
      <el-form-item label="根路径"><el-input v-model="form.base_path" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button v-if="editingId" type="danger" plain @click="emit('delEditingStorage')">删除该存储</el-button>
      <el-button @click="storageDialog = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('submit')">{{ editingId ? '保存' : '添加' }}</el-button>
    </template>
  </el-dialog>

  <!-- 新建文件夹 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="mkdirVisible" title="新建文件夹" width="360">
    <el-input v-model="mkdirName" placeholder="文件夹名称" @keyup.enter="emit('doMkdir')" />
    <template #footer>
      <el-button @click="mkdirVisible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('doMkdir')">新建</el-button>
    </template>
  </el-dialog>

  <!-- 重命名 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="renameVisible" title="重命名" width="360">
    <el-input v-model="renameNewName" @keyup.enter="emit('doRename')" />
    <template #footer>
      <el-button @click="renameVisible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('doRename')">确认</el-button>
    </template>
  </el-dialog>

  <!-- 移动/复制 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="moveCopyVisible" :title="moveCopyMode === 'move' ? '移动到…' : '复制到…'" width="560">
    <div class="mc-bar">
      <el-button size="small" :icon="Top" :disabled="moveCopyPath === '/'" @click="emit('moveCopyUp')">上级</el-button>
      <span class="mc-path">{{ moveCopyPath }}</span>
    </div>
    <div v-loading="moveCopyLoading" class="mc-list">
      <div v-for="d in moveCopyEntries" :key="d.path" class="mc-item" @click="emit('moveCopyEnter', d)">
        <el-icon><Folder /></el-icon>&nbsp;{{ d.name }}
      </div>
      <div v-if="!moveCopyEntries.length && !moveCopyLoading" class="mc-empty">（无子文件夹）</div>
    </div>
    <template #footer>
      <el-button @click="moveCopyVisible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="emit('doMoveCopy')">{{ moveCopyMode === 'move' ? '移动到此处' : '复制到此处' }}</el-button>
    </template>
  </el-dialog>

  <!-- STRM 即时生成 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="strmVisible" title="生成 STRM" width="480">
    <div class="hint">为云盘目录「{{ strmDir?.name }}」下的媒体文件生成本地 .strm（媒体服务器读取后直链播放云盘资源，无需落地）。</div>
    <el-form label-width="88" style="margin-top: 8px">
      <el-form-item label="输出目录"><el-input v-model="strmForm.output_dir" placeholder="本地 .strm 输出根目录，如 /Media/STRM" /></el-form-item>
      <el-form-item label="挂载根"><el-input v-model="strmForm.mount_root" placeholder="如 /CloudDrive2/115（模板 {mount}）" /></el-form-item>
      <el-form-item label="模板"><el-input v-model="strmForm.template" placeholder="留空用默认 {mount}{cloudpath}" /></el-form-item>
      <el-form-item label="递归"><el-switch v-model="strmForm.recursive" /></el-form-item>
      <el-form-item label="保留扩展名"><el-switch v-model="strmForm.keep_ext" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="strmVisible = false">取消</el-button>
      <el-button type="primary" :loading="strmLoading" @click="emit('doStrm')">生成</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint { margin: -4px 0 8px; padding: 8px 12px; font-size: 12px; color: var(--text-muted, #909399); background: var(--pt-bg-soft, rgba(255, 255, 255, 0.04)); border-radius: 6px; line-height: 1.6; }
.mc-bar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.mc-path { flex: 1; font-family: monospace; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mc-list { max-height: 320px; overflow-y: auto; border: 1px solid var(--glass-border); border-radius: 8px; padding: 6px; }
.mc-item { display: flex; align-items: center; padding: 7px 10px; border-radius: 6px; cursor: pointer; font-size: 13px; }
.mc-item:hover { background: color-mix(in srgb, var(--pt-primary, #4361ee) 10%, transparent); }
.mc-empty { text-align: center; color: var(--text-muted, #909399); padding: 20px; }
</style>
