<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  mediaServerApi,
  type MediaServer,
  type MediaServerType,
  type MediaLibrary,
} from '@/api/mediaServer'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { useMediaServerOps, fmtDuration } from '@/composables/useMediaServerOps'

const servers = ref<MediaServer[]>([])
const loading = ref(false)
const testing = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    servers.value = await mediaServerApi.list()
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

// ---- CRUD ----
const showForm = ref(false)
const editing = ref<MediaServer | null>(null)
const form = ref<Partial<MediaServer> & { token?: string }>(defaultForm())

function defaultForm() {
  return {
    name: '',
    type: 'emby' as MediaServerType,
    url: '',
    username: '',
    token: '',
    is_default: false,
    refresh_on_download: true,
    notify_on_play: false,
    direct_link_enabled: false,
    direct_link_base: '',
  }
}

function openCreate() {
  editing.value = null
  form.value = defaultForm()
  showForm.value = true
}

function openEdit(s: MediaServer) {
  editing.value = s
  form.value = { ...s, token: '' } // token 空串=保留；UI 不回显秘密
  showForm.value = true
}

async function save() {
  if (!form.value.name || !form.value.url || !form.value.type) {
    ElMessage.warning('名称、类型、地址必填')
    return
  }
  try {
    if (editing.value) {
      await mediaServerApi.update(editing.value.id, form.value)
      ElMessage.success('已更新')
    } else {
      if (!form.value.token) {
        ElMessage.warning('新建需填写 API Key / Token')
        return
      }
      await mediaServerApi.create(form.value)
      ElMessage.success('已创建')
    }
    showForm.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

async function remove(s: MediaServer) {
  try {
    await ElMessageBox.confirm(`确定删除媒体服务器「${s.name}」？`, '提示', { type: 'warning' })
  } catch {
    return // 用户取消
  }
  try {
    await mediaServerApi.remove(s.id)
    ElMessage.success('已删除')
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

async function test(s: MediaServer) {
  testing.value = s.id
  try {
    const r = await mediaServerApi.test(s.id)
    r.ok ? ElMessage.success(`${s.name}：${r.info || '连接正常'}`) : ElMessage.warning(r.error || '连接失败')
  } catch (e: any) {
    ElMessage.error(e?.message || '请求失败')
  } finally {
    testing.value = null
  }
}

async function toggle(s: MediaServer, field: 'refresh_on_download' | 'notify_on_play', val: boolean) {
  try {
    await mediaServerApi.update(s.id, { [field]: val })
    ;(s as any)[field] = val
  } catch (e: any) {
    ElMessage.error(e?.message || '更新失败')
  }
}

// ---- 库 / 报告 / 查重（composable） ----
const {
  showLibs, libs, libsLoading, libsServer, coverURL, coverLoading,
  openLibs, scanLib, refreshLib, genCover,
  showReport, report, reportLoading, reportDays, openReport, reloadReport,
  showDup, dupGroups, dupLoading, dupLib, openDup, runDup,
} = useMediaServerOps()

const TYPE_LABEL: Record<MediaServerType, string> = {
  emby: 'Emby',
  jellyfin: 'Jellyfin',
  plex: 'Plex',
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">媒体服务器</h2>
      <el-button type="primary" @click="openCreate">+ 添加服务器</el-button>
    </div>
    <p class="sub">
      统一管理 Emby / Jellyfin / Plex：库扫描、元数据刷新、管理员通知、海报墙封面生成；
      开启「下载刷新」后种子下载完成自动触发该服务器扫库。
    </p>

    <div v-loading="loading" class="grid">
      <div v-for="s in servers" :key="s.id" class="glass glass-hover card">
        <div class="card-head">
          <GlassIcon name="mediaserver" :size="34" from="#52b788" to="#2d6a4f" />
          <div class="card-title">
            <div class="name">
              {{ s.name }}
              <el-tag size="small" effect="plain">{{ TYPE_LABEL[s.type] }}</el-tag>
              <el-tag v-if="s.is_default" size="small" type="success">默认</el-tag>
            </div>
            <div class="meta">{{ s.url }}</div>
          </div>
        </div>

        <div class="toggles">
          <div class="toggle-row">
            <span>下载完成刷新</span>
            <el-switch
              :model-value="s.refresh_on_download"
              @update:model-value="(v: any) => toggle(s, 'refresh_on_download', !!v)"
            />
          </div>
          <div class="toggle-row">
            <span>播放通知</span>
            <el-switch
              :model-value="s.notify_on_play"
              @update:model-value="(v: any) => toggle(s, 'notify_on_play', !!v)"
            />
          </div>
        </div>

        <div class="actions">
          <el-button size="small" :loading="testing === s.id" @click="test(s)">连接测试</el-button>
          <el-button size="small" @click="openLibs(s)">媒体库</el-button>
          <el-button size="small" @click="openReport(s)">观影报告</el-button>
          <el-button size="small" @click="openDup(s)">查重</el-button>
          <el-button size="small" @click="openEdit(s)">编辑</el-button>
          <el-button size="small" type="danger" plain @click="remove(s)">删除</el-button>
        </div>
      </div>
      <EmptyState
        v-if="!servers.length && !loading"
        icon="🎬"
        title="尚未添加媒体服务器"
        description="添加 Emby / Jellyfin / Plex 服务器以启用库扫描与封面生成"
      >
        <el-button type="primary" @click="openCreate">添加服务器</el-button>
      </EmptyState>
    </div>

    <!-- CRUD dialog -->
    <el-dialog v-model="showForm" :title="editing ? '编辑服务器' : '添加服务器'" width="560px" top="8vh">
      <el-form :model="form" label-width="120px" label-position="right">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如：客厅 Emby" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.type">
            <el-option label="Emby" value="emby" />
            <el-option label="Jellyfin" value="jellyfin" />
            <el-option label="Plex" value="plex" />
          </el-select>
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.url" placeholder="http://192.168.1.10:8096" />
        </el-form-item>
        <el-form-item label="API Key / Token">
          <el-input
            v-model="form.token"
            type="password"
            show-password
            :placeholder="editing ? '留空=保留原凭据' : 'Emby/JF API Key 或 Plex X-Plex-Token'"
          />
        </el-form-item>
        <el-form-item label="用户名（可选）">
          <el-input v-model="form.username" placeholder="Emby/JF 管理员（通知目标）" />
        </el-form-item>
        <el-form-item label="设为默认"><el-switch v-model="form.is_default" /></el-form-item>
        <el-form-item label="下载完成刷新">
          <el-switch v-model="form.refresh_on_download" />
          <span class="hint">种子下载完成自动扫库</span>
        </el-form-item>
        <el-form-item label="播放通知">
          <el-switch v-model="form.notify_on_play" />
          <span class="hint">收到 webhook 播放事件通知</span>
        </el-form-item>
        <el-divider content-position="left">302 直链反代</el-divider>
        <el-form-item label="启用直链">
          <el-switch v-model="form.direct_link_enabled" />
          <span class="hint">把 Emby 文件路径改写到 CDN 直链</span>
        </el-form-item>
        <el-form-item label="直链基址">
          <el-input v-model="form.direct_link_base" placeholder="https://cdn.example.com/library" />
          <span class="hint">/mediaserver/:id/direct/&lt;path&gt; → {基址}/{path} 302</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 媒体库 + 封面预览 -->
    <el-dialog v-model="showLibs" :title="`媒体库 · ${libsServer?.name ?? ''}`" width="720px" top="6vh">
      <div class="libs-bar">
        <el-button size="small" @click="scanLib(null)">全库扫描</el-button>
        <span class="hint">播放回调地址：{{ libsServer ? `POST /api/v1/mediaserver/${libsServer.id}/webhook` : '' }}</span>
      </div>
      <div v-loading="libsLoading">
        <el-table :data="libs" size="small" empty-text="未获取到媒体库">
          <el-table-column prop="name" label="库名" min-width="120" show-overflow-tooltip />
          <el-table-column prop="type" label="类型" width="100" />
          <el-table-column label="操作" width="280">
            <template #default="{ row }">
              <el-button size="small" link @click="scanLib(row as MediaLibrary)">扫描</el-button>
              <el-button size="small" link @click="refreshLib(row as MediaLibrary)">刷新</el-button>
              <el-button size="small" link type="primary" @click="genCover(row as MediaLibrary)">生成封面</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <div v-if="coverLoading || coverURL" class="cover-preview" v-loading="coverLoading">
        <img v-if="coverURL" :src="coverURL" alt="封面拼图" />
      </div>
    </el-dialog>

    <!-- 观影报告 -->
    <el-dialog v-model="showReport" :title="`观影报告 · ${libsServer?.name ?? ''}`" width="820px" top="6vh">
      <div class="libs-bar">
        <div>
          近
          <el-input-number v-model="reportDays" :min="1" :max="365" size="small" controls-position="right" style="width: 90px" />
          天 · 共 {{ report?.total_plays ?? 0 }} 次播放
        </div>
        <el-button size="small" :loading="reportLoading" @click="reloadReport">刷新</el-button>
      </div>
      <div v-loading="reportLoading">
        <div v-if="report" class="report-block">
          <div class="sub-title">用户 ({{ report.users.length }})</div>
          <div class="user-chips">
            <el-tag v-for="u in report.users" :key="u.id" size="small">
              {{ u.name }} · {{ u.play_count }} 次
            </el-tag>
            <span v-if="!report.users.length" class="hint">无用户数据</span>
          </div>
        </div>
        <div v-if="report" class="report-block">
          <div class="sub-title">热门条目</div>
          <el-table :data="report.top_items" size="small" empty-text="暂无数据">
            <el-table-column prop="name" label="标题" min-width="160" show-overflow-tooltip />
            <el-table-column prop="play_count" label="播放次数" width="100" />
            <el-table-column label="时长" width="100">
              <template #default="{ row }">{{ fmtDuration(row.duration) }}</template>
            </el-table-column>
          </el-table>
        </div>
        <div v-if="report" class="report-block">
          <div class="sub-title">最近播放</div>
          <el-table :data="report.recent_plays" size="small" empty-text="暂无数据">
            <el-table-column prop="name" label="标题" min-width="140" show-overflow-tooltip />
            <el-table-column prop="user_name" label="用户" width="100" />
            <el-table-column prop="play_count" label="次数" width="70" />
            <el-table-column label="最近播放" width="170">
              <template #default="{ row }">{{ row.last_played ? new Date(row.last_played).toLocaleString() : '-' }}</template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </el-dialog>

    <!-- 重复检测 -->
    <el-dialog v-model="showDup" :title="`重复检测 · ${libsServer?.name ?? ''}`" width="820px" top="6vh">
      <div class="libs-bar">
        <el-select v-model="dupLib" placeholder="选择媒体库" size="small" style="width: 220px">
          <el-option v-for="l in libs" :key="l.id" :label="l.name" :value="l.id" />
        </el-select>
        <el-button size="small" type="primary" :loading="dupLoading" @click="runDup">检测</el-button>
      </div>
      <div v-loading="dupLoading">
        <div class="hint" style="margin-bottom: 8px">
          按 TMDB/IMDb ID（优先）或归一化标题+年份归并；共 {{ dupGroups.length }} 组疑似重复。
        </div>
        <el-table :data="dupGroups" size="small" empty-text="无重复（或未检测）">
          <el-table-column prop="title" label="标题" min-width="140" show-overflow-tooltip />
          <el-table-column prop="year" label="年份" width="80" />
          <el-table-column prop="count" label="份数" width="70" />
          <el-table-column label="条目" min-width="220">
            <template #default="{ row }">
              <div v-for="(it, i) in row.items" :key="i" class="dup-item">
                {{ it.name }}<span v-if="it.path" class="muted"> · {{ it.path }}</span>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-3); }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 0; font-size: var(--fs-title); font-weight: 700; }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: var(--sp-3); }
.card { padding: 16px; display: flex; flex-direction: column; gap: var(--sp-3); }
.card-head { display: flex; align-items: center; gap: var(--sp-3); }
.card-title { flex: 1; min-width: 0; }
.name { font-weight: 600; font-size: var(--fs-body); display: flex; align-items: center; gap: var(--sp-1); flex-wrap: wrap; }
.meta { font-size: var(--fs-xs); color: var(--text-muted); word-break: break-all; }
.toggles { display: flex; flex-direction: column; gap: var(--sp-1); }
.toggle-row { display: flex; align-items: center; justify-content: space-between; font-size: var(--fs-sm); }
.actions { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.hint { font-size: var(--fs-xs); color: var(--text-muted); margin-left: 8px; }
.libs-bar { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-2); margin-bottom: 10px; }
.cover-preview { margin-top: 14px; text-align: center; }
.cover-preview img { max-width: 100%; border-radius: var(--radius-xs); }
.report-block { margin-bottom: 16px; }
.sub-title { font-size: var(--fs-sm); font-weight: 700; margin-bottom: 6px; }
.user-chips { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.dup-item { font-size: var(--fs-xs); line-height: 1.6; word-break: break-all; }
.muted { color: var(--text-muted); }
</style>
