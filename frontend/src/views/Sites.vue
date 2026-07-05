<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { siteApi, type Site, type SiteDef, type TestResult, type CheckinResult, type CheckinRecord } from '@/api/site'
import { dashboardApi } from '@/api/dashboard'
import { fmtTime } from '@/utils/format'
import { useAuthStore } from '@/stores/auth'
import EmptyState from '@/components/EmptyState.vue'

// 非 admin 的 VIP 用户可读可用，但删除站点是 admin 专属（后端 RequireAdmin 守卫）。
const auth = useAuthStore()

const sites = ref<Site[]>([])
const defs = ref<SiteDef[]>([])
const loading = ref(false)
const testing = ref<number | null>(null)
const checkining = ref<number | null>(null)
const checkinAllLoading = ref(false)
const recordsDrawer = ref(false)
const records = ref<CheckinRecord[]>([])
const recordsLoading = ref(false)

const dialogVisible = ref(false)
// 编辑态：null=新建，否则持有正在编辑的站点 id
const editingId = ref<number | null>(null)
const isEdit = computed(() => editingId.value !== null)
const dialogTitle = computed(() => (isEdit.value ? '编辑站点' : '添加站点'))
const form = ref({
  name: '',
  definition: '',
  base_url: '',
  cookie: '',
  api_key: '',
  username: '',
  password: '',
  is_active: true,
})

// 当前选中的站点定义（据其 schema 决定表单形态：馒头走 x-api-key，无需 cookie/账密）
const currentDef = computed(() => defs.value.find((d) => d.id === form.value.definition))
const isMTeam = computed(() => currentDef.value?.schema === 'mteam')
// 取定义默认地址：首个真实 URL（馒头 urls[0] 即 api.m-team.cc），无则回退占位。
function defUrl(d?: SiteDef) {
  if (d?.urls?.length) return d.urls[0]
  return d?.schema === 'mteam' ? 'https://api.m-team.cc' : 'https://hxpt.org'
}
// 占位优先取当前定义真实地址。
const baseUrlPlaceholder = computed(() => defUrl(currentDef.value))

// 选站下拉排序：可用站在前、死站在后（死站保留可选，仅标注「已关站」）。
const sortedDefs = computed(() =>
  [...defs.value].sort((a, b) => (a.dead === b.dead ? 0 : a.dead ? 1 : -1)),
)

// 已知默认值集合：切定义时若名称/地址仍等于任一定义默认值（或为空）则跟随当前定义更新，
// 用户手填的自定义值不覆盖；编辑态既有非默认值同样保留。
const knownNames = computed(() => defs.value.map((d) => d.name))
const knownUrls = computed(() => defs.value.map((d) => defUrl(d)))

// 切换定义时：名称/地址跟随当前定义（仅在仍是默认值或为空时），馒头无需特判。
function onDefChange() {
  const def = currentDef.value
  if (!def) return
  if (!form.value.name || knownNames.value.includes(form.value.name)) form.value.name = def.name
  const url = defUrl(def)
  if (!form.value.base_url || knownUrls.value.includes(form.value.base_url)) {
    form.value.base_url = url
  }
}

async function load() {
  loading.value = true
  try {
    ;[sites.value, defs.value] = await Promise.all([siteApi.list(), siteApi.definitions()])
  } catch (e) {
    ElMessage.error('加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  form.value = { name: '', definition: '', base_url: '', cookie: '', api_key: '', username: '', password: '', is_active: true }
  dialogVisible.value = true
}

function openEdit(s: Site) {
  editingId.value = s.id
  form.value = {
    name: s.name,
    definition: s.definition,
    base_url: s.base_url,
    cookie: '', // 留空=不修改（后端 Update 仅在非空时覆写）
    api_key: '', // 留空=不修改（馒头 x-api-key）
    username: s.username || '',
    password: '', // 留空=不修改
    is_active: s.is_active,
  }
  dialogVisible.value = true
}

async function submit() {
  if (!form.value.name || !form.value.base_url) {
    ElMessage.warning('名称与地址必填')
    return
  }
  // 馒头走 x-api-key 鉴权：新建必填 API Key，否则后续连接测试/搜索全部 401
  if (isMTeam.value && !isEdit.value && !form.value.api_key.trim()) {
    ElMessage.warning('馒头需填写 API Key（M-Team 控制台 → 实验室 → 存储令牌）')
    return
  }
  // 根据定义自动推断 platform
  const def = defs.value.find((d) => d.id === form.value.definition)
  try {
    if (isEdit.value) {
      await siteApi.update(editingId.value!, {
        name: form.value.name,
        definition: form.value.definition,
        platform: def?.schema || 'nexusphp',
        base_url: form.value.base_url,
        cookie: form.value.cookie,
        api_key: form.value.api_key,
        username: form.value.username,
        password: form.value.password,
        is_active: form.value.is_active,
      })
      ElMessage.success('已更新')
    } else {
      await siteApi.create({
        name: form.value.name,
        definition: form.value.definition,
        platform: def?.schema || 'nexusphp',
        base_url: form.value.base_url,
        cookie: form.value.cookie,
        api_key: form.value.api_key,
        username: form.value.username,
        password: form.value.password,
        is_active: form.value.is_active,
      })
      ElMessage.success('已添加')
      // 新建站点：立即抓取统计，避免切到仪表盘看到全 0。
      dashboardApi.sync().catch(() => {})
    }
    dialogVisible.value = false
    await load()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || (isEdit.value ? '更新失败' : '添加失败'))
  }
}

async function testSite(s: Site) {
  testing.value = s.id
  try {
    const r: TestResult = await siteApi.test(s.id)
    if (r.ok) ElMessage.success(`${s.name}：连接正常，等级 ${r.level || '-'}`)
    else ElMessage.warning(`${s.name}：${r.error || '连接失败'}`)
  } catch {
    ElMessage.error('请求失败')
  } finally {
    testing.value = null
  }
}

// 单站签到：仅 NexusPHP 站适配器实现了 CheckinCapable（馒头等走 API key 无传统签到）。
async function checkinSite(s: Site) {
  checkining.value = s.id
  try {
    const r: CheckinResult = await siteApi.checkin(s.id)
    if (r.success) {
      if (r.bonus > 0) ElMessage.success(`${s.name}：签到成功，获得 ${r.bonus} 魔力`)
      else if (r.message?.includes('已签到')) ElMessage.info(`${s.name}：今天已签到`)
      else ElMessage.success(`${s.name}：${r.message || '签到成功'}`)
    } else {
      const reason = r.error || r.message || '签到失败'
      if (reason.includes('not supported')) ElMessage.warning(`${s.name}：该站点暂不支持签到`)
      else if (reason.includes('not logged in')) ElMessage.warning(`${s.name}：未登录，请先登录`)
      else ElMessage.error(`${s.name}：${reason}`)
    }
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || `${s.name}：请求失败`)
  } finally {
    checkining.value = null
  }
}

// 一键签到全部启用站点。不支持签到的站（馒头等）聚合时跳过，不计入失败，避免误导。
async function checkinAll() {
  checkinAllLoading.value = true
  try {
    const results = await siteApi.checkinAll()
    if (!results.length) {
      ElMessage.info('没有启用的站点')
      return
    }
    let ok = 0
    let already = 0
    const failed: string[] = []
    for (const r of results) {
      if (r.success) {
        if (r.message?.includes('已签到')) already++
        else ok++
      } else if (!(r.error || '').includes('not supported')) {
        failed.push(r.site_name)
      }
    }
    const summary = `成功 ${ok} / 已签 ${already}`
    if (!failed.length) ElMessage.success(`一键签到完成：${summary}`)
    else ElMessage.warning(`一键签到完成：${summary} / 失败 ${failed.length}（${failed.join('、')}）`)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '一键签到请求失败')
  } finally {
    checkinAllLoading.value = false
  }
}

// 签到记录抽屉：siteId 省略=全部站点最近 100 条（含 cron 自动 + 手动）。
async function openRecords(siteId?: number) {
  recordsDrawer.value = true
  recordsLoading.value = true
  try {
    records.value = await siteApi.records(siteId)
  } catch {
    ElMessage.error('加载签到记录失败')
    records.value = []
  } finally {
    recordsLoading.value = false
  }
}

async function removeSite(s: Site) {
  await ElMessageBox.confirm(`确认删除站点「${s.name}」？`, '删除', { type: 'warning' })
  await siteApi.remove(s.id)
  ElMessage.success('已删除')
  await load()
}

onMounted(load)
</script>

<template>
  <div class="sites">
    <div class="head">
      <h2 class="page-title">站点</h2>
      <div class="head-actions">
        <el-button @click="openRecords()">签到记录</el-button>
        <el-button :loading="checkinAllLoading" @click="checkinAll">一键签到</el-button>
        <el-button type="primary" @click="openCreate">+ 添加站点</el-button>
      </div>
    </div>

    <div v-loading="loading" class="card glass">
      <el-table :data="sites" style="width: 100%" :row-style="{ background: 'transparent' }">
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="definition" label="定义" min-width="120" />
        <el-table-column prop="platform" label="平台" min-width="100" />
        <el-table-column prop="base_url" label="地址" min-width="220" show-overflow-tooltip />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <span :class="['pill', row.is_active ? 'on' : 'off']">
              {{ row.is_active ? '启用' : '停用' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="320">
          <template #default="{ row }">
            <el-button
              v-if="row.platform === 'nexusphp'"
              size="small"
              :loading="checkining === row.id"
              @click="checkinSite(row as Site)"
            >
              签到
            </el-button>
            <el-button size="small" :loading="testing === row.id" @click="testSite(row as Site)">
              连接测试
            </el-button>
            <el-button size="small" @click="openEdit(row as Site)">编辑</el-button>
            <el-button v-if="auth.isAdmin" size="small" type="danger" plain @click="removeSite(row as Site)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <EmptyState
        v-if="!sites.length && !loading"
        icon="🌐"
        title="暂无站点"
        description="添加第一个 PT 站点并粘贴 Cookie/API Key，随后即可做跨站统计、签到、H&R 告警和内容搜索。"
      >
        <el-button type="primary" @click="openCreate">+ 添加站点</el-button>
      </EmptyState>
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="480">
      <el-form label-width="80">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="如：好学" />
        </el-form-item>
        <el-form-item label="定义">
          <el-select
            v-model="form.definition"
            placeholder="选择站点定义"
            style="width: 100%"
            filterable
            @change="onDefChange"
          >
            <el-option
              v-for="d in sortedDefs"
              :key="d.id"
              :label="`${d.name}（${d.schema || '通用'}）${d.aka?.length ? ' · ' + d.aka.join(' / ') : ''}${d.dead ? ' · 已关站' : ''}`"
              :value="d.id"
            >
              <span>{{ d.name }}（{{ d.schema || '通用' }}）</span>
              <el-tag v-if="d.aka?.length" size="small" type="primary" effect="plain" style="margin-left: 8px">
                {{ d.aka.join(' / ') }}
              </el-tag>
              <el-tag v-if="d.dead" size="small" type="info" effect="plain" style="margin-left: 8px">
                已关站
              </el-tag>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.base_url" :placeholder="baseUrlPlaceholder" />
        </el-form-item>
        <!-- 馒头走 api.m-team.cc 的 JSON API + x-api-key，无 cookie/账密概念 -->
        <el-form-item v-if="isMTeam" label="API Key">
          <el-input
            v-model="form.api_key"
            type="password"
            show-password
            :placeholder="isEdit ? '留空则不修改' : 'M-Team x-api-key（控制台 → 实验室 → 存储令牌）'"
          />
        </el-form-item>
        <el-form-item v-if="!isMTeam" label="Cookie">
          <el-input
            v-model="form.cookie"
            type="textarea"
            :rows="3"
            :placeholder="isEdit ? '留空则不修改' : '粘贴站点 cookie（k=v; k2=v2），加密存储'"
          />
        </el-form-item>
        <el-form-item v-if="!isMTeam" label="账号">
          <el-input v-model="form.username" placeholder="站点用户名（财神刮刮乐 API 登录用）" />
        </el-form-item>
        <el-form-item v-if="!isMTeam" label="密码">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="isEdit ? '留空则不修改' : '站点密码（加密存储）'"
          />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.is_active" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">{{ isEdit ? '保存' : '添加' }}</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="recordsDrawer" title="签到记录" size="600">
      <div v-loading="recordsLoading">
        <el-table :data="records" style="width: 100%" max-height="100%">
          <el-table-column label="时间" width="150">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="site_name" label="站点" min-width="100" />
          <el-table-column label="结果" min-width="160">
            <template #default="{ row }">
              <span v-if="row.success" class="pill on">
                {{ row.bonus > 0 ? `+${row.bonus} 魔力` : row.message || '成功' }}
              </span>
              <span v-else class="pill off">{{ row.error || row.message || '失败' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="来源" width="80">
            <template #default="{ row }">{{ row.source === 'cron' ? '自动' : '手动' }}</template>
          </el-table-column>
        </el-table>
        <div v-if="!records.length && !recordsLoading" class="empty text-muted">暂无签到记录</div>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.sites {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.head-actions {
  display: flex;
  gap: 8px;
}
.page-title {
  margin: 4px 0;
  font-size: 22px;
  font-weight: 700;
}
.card {
  padding: 18px;
  min-height: 200px;
}
.pill {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
}
.pill.on {
  background: rgba(46, 204, 113, 0.2);
  color: var(--pt-success);
}
.pill.off {
  background: rgba(231, 76, 60, 0.15);
  color: var(--pt-danger);
}
.empty {
  padding: 40px;
  text-align: center;
}
:deep(.el-table),
:deep(.el-table tr),
:deep(.el-table td),
:deep(.el-table th) {
  background-color: transparent !important;
}
:deep(.el-table) {
  --el-table-border-color: rgba(0, 0, 0, 0.06);
}
</style>
