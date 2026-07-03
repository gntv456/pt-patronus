<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { reseedApi, type ReseedTask, type ReseedSetting } from '@/api/reseed'
import { downloaderApi, type Downloader } from '@/api/downloader'

const config = ref<{ masked: string; configured: boolean }>({ masked: '', configured: false })
const tokenInput = ref('')
const savingToken = ref(false)

const downloaders = ref<Downloader[]>([])
const selectedDl = ref<number | null>(null)
const reseeding = ref(false)

const tasks = ref<ReseedTask[]>([])
const loadingTasks = ref(false)
const statusFilter = ref<'pending' | 'added' | 'skipped' | 'failed' | ''>('pending')
const resolving = ref(new Set<number>())

// 当前选中下载器的辅种设置（自动辅种 / 自动添加）
const setting = ref<ReseedSetting>({
  downloader_id: 0, save_path: '', pause_after_add: true, auto_reseed: false, auto_reseed_add: false,
})
const savingSetting = ref(false)

async function loadConfig() {
  try {
    const c = await reseedApi.getConfig()
    config.value = { masked: c.iyuu_token_masked, configured: c.configured }
  } catch {
    ElMessage.warning('辅种配置加载失败')
  }
}

async function saveToken() {
  if (!tokenInput.value.trim()) {
    ElMessage.warning('请输入 IYUU token')
    return
  }
  savingToken.value = true
  try {
    await reseedApi.setConfig(tokenInput.value.trim())
    tokenInput.value = ''
    ElMessage.success('IYUU token 已保存')
    await loadConfig()
  } catch {
    ElMessage.warning('保存失败')
  } finally {
    savingToken.value = false
  }
}

async function loadDownloaders() {
  try {
    downloaders.value = await downloaderApi.list()
    if (downloaders.value.length && selectedDl.value === null) {
      selectedDl.value = downloaders.value[0].id
    }
  } catch {
    ElMessage.warning('下载器列表加载失败')
  }
}

async function loadTasks() {
  loadingTasks.value = true
  try {
    tasks.value = await reseedApi.tasks(statusFilter.value)
  } catch {
    ElMessage.warning('任务列表加载失败')
  } finally {
    loadingTasks.value = false
  }
}

async function loadSetting() {
  if (selectedDl.value === null) return
  try {
    setting.value = await reseedApi.getSetting(selectedDl.value)
  } catch {
    ElMessage.warning('辅种设置加载失败，显示为默认值')
  }
}

async function saveSettingField() {
  if (selectedDl.value === null) return
  savingSetting.value = true
  try {
    setting.value.downloader_id = selectedDl.value
    await reseedApi.setSetting(setting.value)
  } catch {
    ElMessage.warning('设置保存失败')
  } finally {
    savingSetting.value = false
  }
}

async function startReseed() {
  if (selectedDl.value === null) {
    ElMessage.warning('请选择下载器')
    return
  }
  if (!config.value.configured) {
    ElMessage.warning('请先保存 IYUU token')
    return
  }
  reseeding.value = true
  try {
    const r = await reseedApi.reseed(selectedDl.value)
    ElMessage.success(`辅种完成，新增 ${r.count} 个候选`)
    statusFilter.value = 'pending'
    await loadTasks()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '辅种失败')
  } finally {
    reseeding.value = false
  }
}

async function resolve(row: ReseedTask, action: 'add' | 'skip') {
  resolving.value.add(row.id)
  try {
    await reseedApi.resolve(row.id, action)
    ElMessage.success(action === 'add' ? '已添加辅种' : '已跳过')
    await loadTasks()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '操作失败')
  } finally {
    resolving.value.delete(row.id)
  }
}

function statusType(s: ReseedTask['status']): 'primary' | 'success' | 'warning' | 'info' | 'danger' | undefined {
  const m = { pending: 'info', added: 'success', skipped: undefined, failed: 'danger' } as const
  return m[s]
}
function statusLabel(s: ReseedTask['status']) {
  return { pending: '待处理', added: '已添加', skipped: '已跳过', failed: '失败' }[s] || s
}

watch(selectedDl, () => loadSetting())

onMounted(async () => {
  await Promise.all([loadConfig(), loadDownloaders(), loadTasks()])
  await loadSetting()
})
</script>

<template>
  <div class="reseed">
    <!-- 标题行：标题 + 一句话副标题 + 右侧 IYUU 配置状态 -->
    <div class="page-head">
      <h2 class="page-title">辅种</h2>
      <span class="subtitle text-muted">IYUU 精确跨站匹配 · 一键添加辅种（复用源种子保存路径）</span>
      <span class="cfg-chip" :class="config.configured ? 'ok' : 'warn'">
        <span class="dot"></span>{{ config.configured ? 'IYUU 已配置' : 'IYUU 未配置' }}
      </span>
    </div>

    <!-- 紧凑工具栏：下载器 + 开始辅种 + 设置（token / 自动辅种 收进弹层） -->
    <div class="toolbar card glass">
      <el-select v-model="selectedDl" placeholder="选择下载器" class="dl-select">
        <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
      </el-select>
      <el-button type="primary" :loading="reseeding" @click="startReseed">
        <GlassIcon name="seeding" from="#11998e" to="#38ef7d" :size="16" class="btn-ico" />
        <span>开始辅种</span>
      </el-button>

      <div class="spacer"></div>

      <el-popover trigger="click" placement="bottom-end" :width="340" popper-class="reseed-cfg-pop">
        <template #reference>
          <el-button class="cfg-btn">
            <GlassIcon name="settings" from="#6c5ce7" to="#4285f4" :size="16" class="btn-ico" />
            <span>设置</span>
          </el-button>
        </template>

        <div class="cfg-pop">
          <!-- IYUU token -->
          <div class="cfg-sec">
            <div class="cfg-sec-head">
              <span class="cfg-sec-title">IYUU Token</span>
              <span class="cfg-state" :class="config.configured ? 'ok' : 'warn'">
                {{ config.configured ? `已配置 ${config.masked}` : '未配置' }}
              </span>
            </div>
            <div class="token-row">
              <el-input
                v-model="tokenInput"
                :placeholder="config.configured ? '输入新值覆盖' : '粘贴 IYUU token'"
                type="password"
                show-password
                clearable
              />
              <el-button type="primary" :loading="savingToken" @click="saveToken">保存</el-button>
            </div>
            <a class="cfg-link" href="https://iyuu.cn" target="_blank" rel="noopener">
              前往 iyuu.cn 获取 token 并绑定站点 →
            </a>
          </div>

          <div class="cfg-divider"></div>

          <!-- 自动辅种（依赖已选下载器） -->
          <div class="cfg-sec" v-if="selectedDl !== null">
            <div class="cfg-sec-title">自动辅种</div>
            <div class="switch-row">
              <span>定时自动辅种</span>
              <el-switch v-model="setting.auto_reseed" :loading="savingSetting" @change="saveSettingField" />
            </div>
            <div class="switch-row">
              <span class="sub-label">自动添加新候选</span>
              <el-switch
                v-model="setting.auto_reseed_add"
                :disabled="!setting.auto_reseed"
                :loading="savingSetting"
                @change="saveSettingField"
              />
            </div>
            <p class="cfg-cron text-muted">每天 3 点扫描（KEEPSEED_CRON 可改）</p>
          </div>
          <p v-else class="cfg-empty text-muted">请先选择下载器以配置自动辅种</p>
        </div>
      </el-popover>
    </div>

    <!-- 候选任务表（撑满剩余视口，内部滚动） -->
    <div class="card glass table-card">
      <div class="card-head">
        <span>候选任务<span class="cnt text-muted">（{{ tasks.length }}）</span></span>
        <el-radio-group v-model="statusFilter" size="small" @change="loadTasks">
          <el-radio-button label="pending">待处理</el-radio-button>
          <el-radio-button label="added">已添加</el-radio-button>
          <el-radio-button label="skipped">已跳过</el-radio-button>
          <el-radio-button label="failed">失败</el-radio-button>
          <el-radio-button label="">全部</el-radio-button>
        </el-radio-group>
      </div>
      <div class="table-wrap">
        <el-table :data="tasks" v-loading="loadingTasks" height="100%" empty-text="暂无候选，点「开始辅种」">
          <el-table-column label="源种子" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="src-name">{{ row.source_name || row.info_hash }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="candidate_site" label="候选站点" width="110" />
          <el-table-column label="候选链接" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              <a :href="row.candidate_url" target="_blank" rel="noopener" class="dl-link">下载链接</a>
            </template>
          </el-table-column>
          <el-table-column label="保存路径" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-muted">{{ row.save_path || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150" fixed="right">
            <template #default="{ row }">
              <div v-if="row.status === 'pending'" class="row-actions">
                <el-button size="small" type="primary" :loading="resolving.has(row.id)" @click="resolve(row as ReseedTask, 'add')">
                  添加辅种
                </el-button>
                <el-button size="small" :loading="resolving.has(row.id)" @click="resolve(row as ReseedTask, 'skip')">跳过</el-button>
              </div>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.reseed {
  display: flex;
  flex-direction: column;
  gap: 14px;
  height: 100%;
  min-height: 0;
}

/* 标题行：标题 + 副标题 + 配置状态，单行 */
.page-head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: none;
  min-width: 0;
}
.page-title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  flex: none;
}
.subtitle {
  margin: 0;
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.cfg-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  padding: 5px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  flex: none;
  background: rgba(255, 255, 255, 0.5);
}
:root.dark .cfg-chip {
  background: rgba(255, 255, 255, 0.06);
}
.cfg-chip .dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}
.cfg-chip.ok {
  color: var(--pt-success);
}
.cfg-chip.warn {
  color: var(--pt-warn);
}

/* 紧凑工具栏 */
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  flex-wrap: wrap;
  flex: none;
}
.dl-select {
  width: 220px;
}
.btn-ico {
  margin-right: 5px;
  vertical-align: middle;
}
.spacer {
  flex: 1;
}

/* 候选任务表卡片：撑满剩余视口 */
.table-card {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 14px 16px;
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
  font-weight: 600;
  font-size: 15px;
  flex: none;
}
.cnt {
  font-weight: 400;
  font-size: 13px;
  margin-left: 2px;
}
.table-wrap {
  flex: 1;
  min-height: 0;
}

/* 表格透明背景，贴合玻璃风格 */
:deep(.el-table),
:deep(.el-table tr),
:deep(.el-table td),
:deep(.el-table th) {
  background-color: transparent !important;
}
.src-name {
  font-size: 13px;
}
.dl-link {
  color: #6ea8fe;
  font-size: 13px;
}
.row-actions {
  display: flex;
  gap: 6px;
}
</style>

<!-- 弹层（el-popover teleport 到 body，scoped 选不中，单独用 popper-class 命名空间） -->
<style>
.reseed-cfg-pop {
  --el-popover-padding: 16px;
}
.reseed-cfg-pop .cfg-sec-title {
  font-weight: 600;
  font-size: 13px;
}
.reseed-cfg-pop .cfg-sec-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.reseed-cfg-pop .cfg-state {
  font-size: 12px;
  font-weight: 600;
}
.reseed-cfg-pop .cfg-state.ok {
  color: var(--pt-success);
}
.reseed-cfg-pop .cfg-state.warn {
  color: var(--pt-warn);
}
.reseed-cfg-pop .token-row {
  display: flex;
  gap: 8px;
}
.reseed-cfg-pop .cfg-link {
  display: inline-block;
  margin-top: 8px;
  font-size: 12px;
  color: var(--pt-primary);
  text-decoration: none;
}
.reseed-cfg-pop .cfg-divider {
  height: 1px;
  background: rgba(128, 128, 128, 0.18);
  margin: 14px 0;
}
.reseed-cfg-pop .switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  font-size: 13px;
}
.reseed-cfg-pop .switch-row .sub-label {
  color: var(--text-muted);
}
.reseed-cfg-pop .cfg-cron {
  font-size: 12px;
  margin: 6px 0 0;
}
.reseed-cfg-pop .cfg-empty {
  font-size: 13px;
  margin: 0;
}
</style>
