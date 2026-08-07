<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi } from '@/api/docker'

interface Project {
  name: string; path: string; dir: string; file: string; content: string; host_dir: string
}
const projects = ref<Project[]>([])
const loading = ref(false)
const scanDir = ref('')
const available = ref(false)
const detailVisible = ref(false)
const detailContent = ref('')
const detailName = ref('')
const outputVisible = ref(false)
const outputText = ref('')
const actionLoading = ref(false)

async function refresh() {
  loading.value = true
  try {
    const r = await dockerApi.localProjects()
    projects.value = r.projects || []
    scanDir.value = r.scan_dir
    available.value = r.available
  } catch (e: any) { ElMessage.error(e.message) }
  finally { loading.value = false }
}

async function showDetail(p: Project) {
  try {
    const r = await dockerApi.localProjectDetail(p.dir)
    detailContent.value = r.content
    detailName.value = p.name
    detailVisible.value = true
  } catch (e: any) { ElMessage.error(e.message) }
}

async function doAction(p: Project, action: 'up' | 'down' | 'restart' | 'pull') {
  const labels = { up: '部署', down: '停止', restart: '重启', pull: '拉取镜像' }
  try {
    if (action === 'down') {
      await ElMessageBox.confirm(`确定要停止项目「${p.name}」吗？`, '确认', { type: 'warning' })
    }
    actionLoading.value = true
    outputText.value = ''
    outputVisible.value = true
    const res = await dockerApi.localProjectAction(p.dir, action)
    outputText.value = res.output || (res.ok ? '成功' : '失败')
    if (res.ok) ElMessage.success(labels[action] + '成功')
  } catch (e: any) {
    if (e !== 'cancel') { outputText.value = e.message; outputVisible.value = true }
  } finally { actionLoading.value = false }
}

onMounted(() => refresh())
</script>

<template>
  <div class="compose-projects">
    <div v-if="!available" class="not-available glass">
      <p>未挂载宿主 compose 目录。</p>
      <p class="hint">在 docker-compose.yml 添加：<code>- ${'$'}{COMPOSE_ROOT:-/opt}:/host/compose:ro</code></p>
      <p class="hint">然后设置环境变量 <code>COMPOSE_ROOT</code> 指向你的 compose 项目根目录。</p>
    </div>
    <div v-else v-loading="loading" class="project-list">
      <div class="list-head">
        <span>扫描目录：{{ scanDir }}（{{ projects.length }} 个项目）</span>
        <el-button size="small" @click="refresh">刷新</el-button>
      </div>
      <div v-for="p in projects" :key="p.dir" class="project-card glass">
        <div class="project-head">
          <div>
            <strong class="project-name">{{ p.name }}</strong>
            <span class="project-file">{{ p.file }}</span>
            <span class="project-host" :title="p.host_dir">{{ p.host_dir }}</span>
          </div>
          <div class="project-actions">
            <el-button size="small" link @click="showDetail(p)">查看</el-button>
            <el-button size="small" type="warning" link :disabled="actionLoading" @click="doAction(p, 'pull')">拉取</el-button>
            <el-button size="small" type="warning" link :disabled="actionLoading" @click="doAction(p, 'restart')">重启</el-button>
            <el-button size="small" type="danger" link :disabled="actionLoading" @click="doAction(p, 'down')">停止</el-button>
            <el-button size="small" type="primary" link :disabled="actionLoading" @click="doAction(p, 'up')">部署</el-button>
          </div>
        </div>
        <pre class="project-preview">{{ p.content }}</pre>
      </div>
      <div v-if="!projects.length" class="empty">未找到 compose 项目。将项目放在扫描目录下（含 docker-compose.yml）。</div>
    </div>

    <el-drawer v-model="detailVisible" :title="`项目: ${detailName}`" size="60%" direction="rtl">
      <pre class="detail-content">{{ detailContent }}</pre>
    </el-drawer>
    <el-drawer v-model="outputVisible" title="命令输出" size="50%" direction="rtl">
      <pre class="output-content">{{ outputText }}</pre>
    </el-drawer>
  </div>
</template>

<style scoped>
.compose-projects { padding: 0 4px; }
.not-available { padding: 24px; text-align: center; }
.not-available p { margin: 6px 0; }
.hint { font-size: 13px; color: var(--text-secondary, #888); }
.hint code { background: rgba(0,0,0,.08); padding: 2px 6px; border-radius: 3px; font-size: 12px; }
.list-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; font-size: 13px; color: var(--text-secondary,#888); }
.project-card { padding: 14px 16px; margin-bottom: 12px; }
.project-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.project-name { font-size: 15px; margin-right: 8px; }
.project-file { font-size: 11px; padding: 1px 6px; border-radius: 4px; background: rgba(0,0,0,.08); font-family: monospace; margin-right: 8px; }
.project-host { font-size: 11px; color: var(--text-tertiary,#aaa); font-family: monospace; }
.project-actions { display: flex; gap: 4px; }
.project-preview { font-size: 12px; font-family: monospace; color: var(--text-secondary,#888); max-height: 120px; overflow: auto; margin: 0; white-space: pre-wrap; background: rgba(0,0,0,.04); padding: 8px; border-radius: 4px; }
.detail-content, .output-content { font-size: 13px; font-family: monospace; white-space: pre-wrap; padding: 12px; }
.empty { text-align: center; padding: 32px; color: var(--text-tertiary, #aaa); }
</style>
