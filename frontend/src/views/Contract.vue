<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { contractApi, type ContractRule, type ContractCheckResult } from '@/api/contract'
import { siteApi } from '@/api/site'
import SiteFavicon from '@/components/SiteFavicon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'

const { t } = useI18n()
const route = useRoute()
const focusSiteId = computed(() => {
  const n = Number(route.query.site_id)
  return Number.isFinite(n) && n > 0 ? n : 0
})
const rules = ref<ContractRule[]>([])
const displayRules = computed(() =>
  focusSiteId.value ? rules.value.filter((r) => r.site_id === focusSiteId.value) : rules.value,
)
const sites = ref<{ id: number; name: string; base_url: string }[]>([])
const focusSiteName = computed(() => {
  if (!focusSiteId.value) return ''
  const fromList = sites.value.find((s) => s.id === focusSiteId.value)?.name
  const q = route.query.site_name
  return fromList || (typeof q === 'string' ? q : '') || `#${focusSiteId.value}`
})
const loading = ref(false)
const results = ref<Record<number, ContractCheckResult>>({})

async function load() {
  loading.value = true
  try {
    const [rs, ss] = await Promise.all([contractApi.list(), siteApi.list()])
    rules.value = rs
    sites.value = ss.map((s) => ({ id: s.id, name: s.name, base_url: s.base_url }))
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

const showForm = ref(false)
const editing = ref<ContractRule | null>(null)
const form = ref<Partial<ContractRule>>(defaultForm())

function defaultForm(): Partial<ContractRule> {
  return {
    site_id: 0,
    enabled: true,
    min_ratio: 0,
    min_seeding: 0,
    min_upload_gb: 0,
    official_only: false,
    min_seeding_size_gb: 0,
    period: '',
  }
}

function openCreate() {
  editing.value = null
  form.value = defaultForm()
  if (focusSiteId.value) form.value.site_id = focusSiteId.value
  else if (sites.value.length) form.value.site_id = sites.value[0].id
  showForm.value = true
}

function openEdit(r: ContractRule) {
  editing.value = r
  form.value = { ...r }
  showForm.value = true
}

async function save() {
  if (!form.value.site_id) {
    ElMessage.warning(t('contract.selectSiteFirst'))
    return
  }
  try {
    await contractApi.upsert(form.value)
    ElMessage.success(t('common.saved'))
    showForm.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.saveFailed'))
  }
}

async function remove(r: ContractRule) {
  try {
    await ElMessageBox.confirm(t('contract.confirmDelete', { site: siteName(r.site_id) }), t('messages.tip'), {
      type: 'warning',
    })
  } catch {
    return // 用户取消
  }
  try {
    await contractApi.remove(r.id)
  } catch (e: any) {
    // 从前这里没有兜底：删除失败（404「规则已被别人删掉」/400「ID 不合法」/500）会逃出
    // async 的点击回调进未处理 Promise——既不报失败也不报成功，那一行还留在列表里，
    // 看起来就是「删除点了没反应」。
    ElMessage.error(e?.userMessage || e?.message || t('common.deleteFailed'))
    await load() // 让列表回到库里的真值：可能它其实已经被别人删了
    return
  }
  ElMessage.success(t('common.deleted'))
  await load()
}

async function toggle(r: ContractRule, val: boolean) {
  try {
    await contractApi.upsert({ ...r, enabled: val })
  } catch (e: any) {
    // 开关的显示值已经被拨过去了：写不进去就要按库里的真值拉回来，
    // 否则界面留着「已启用」的幻影，而 cron 其实根本不会检查这条规则。
    ElMessage.error(e?.userMessage || e?.message || t('common.saveFailed'))
    await load()
    return
  }
  r.enabled = val
}

async function check(r: ContractRule) {
  try {
    const res = await contractApi.check(r.id)
    results.value[r.id] = res
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('contract.checkFailed'))
  }
}

function siteName(id: number) {
  return sites.value.find((s) => s.id === id)?.name || `#${id}`
}
function siteBaseUrl(id: number) {
  return sites.value.find((s) => s.id === id)?.base_url || ''
}
function statusType(s: string): 'success' | 'warning' | 'danger' | 'info' {
  return ({ ok: 'success', breach: 'warning', error: 'danger' } as const)[s as 'ok'] || 'info'
}
function statusLabel(s: string) {
  return (
    ({ ok: t('contract.st.ok'), breach: t('contract.st.breach'), error: t('contract.st.error') } as const)[s as 'ok'] ||
    t('contract.st.unchecked')
  )
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('contract.title') }}</h2>
      <el-button type="primary" @click="openCreate">{{ t('contract.create') }}</el-button>
    </div>
    <p class="sub">{{ t('contract.subtitle') }}</p>

    <el-alert
      v-if="focusSiteId"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      :title="t('contract.focusTitle', { site: focusSiteName, n: displayRules.length })"
    />

    <div v-loading="loading" class="grid">
      <div v-for="r in displayRules" :key="r.id" class="glass glass-hover card">
        <div class="card-head">
          <SiteFavicon :name="siteName(r.site_id)" :base-url="siteBaseUrl(r.site_id)" :size="34" />
          <div class="card-title">
            <div class="name">{{ siteName(r.site_id) }}</div>
            <div class="meta">
              {{ t('contract.ratioGte') }}{{ r.min_ratio || '-' }} · {{ t('contract.seedingGte')
              }}{{ r.min_seeding || '-' }} · {{ t('contract.uploadGte') }}{{ r.min_upload_gb || '-' }}GiB<template
                v-if="r.official_only"
              >
                · {{ t('subscriptions.officialOnly') }}</template
              ><template v-if="r.min_seeding_size_gb">
                · {{ t('contract.sizeGte') }}{{ r.min_seeding_size_gb }}GiB</template
              ><template v-if="r.period">
                · {{ r.period === 'weekly' ? t('contract.weekly') : t('contract.monthly') }}</template
              >
            </div>
          </div>
          <el-switch :model-value="r.enabled" @update:model-value="(v: any) => toggle(r, !!v)" />
        </div>
        <div class="status-row">
          <el-tag :type="statusType(r.last_status)" size="small">{{ statusLabel(r.last_status) }}</el-tag>
          <span v-if="r.last_detail" class="detail">{{ r.last_detail }}</span>
          <span v-if="r.last_checked_at" class="time">{{ new Date(r.last_checked_at).toLocaleString() }}</span>
        </div>
        <div v-if="results[r.id]" class="check-result">
          <template v-if="results[r.id].measured">
            {{ t('contract.livePrefix') }}{{ t('contract.ratioShort') }} {{ results[r.id].ratio.toFixed(2) }} ·
            {{ t('contract.seedingShort') }} {{ results[r.id].seeding
            }}<template v-if="results[r.id].seeding_official"
              >（{{ t('contract.officialShort') }} {{ results[r.id].seeding_official }}）</template
            >
            · {{ t('contract.uploadShort') }} {{ results[r.id].upload_gb.toFixed(1) }}GiB ·
            {{ t('contract.sizeShort') }} {{ results[r.id].seeding_size_gb.toFixed(1) }}GiB<template
              v-if="results[r.id].unsupported"
            >
              · ⚠ {{ results[r.id].unsupported }}</template
            >
          </template>
          <!-- 没取到数（未到生效时间 / 站点被删 / 抓取失败）时四个字段都是零值占位，
               画出来就是一行以假乱真的「实时：分享率 0.00 · 做种 0」，只能说这次没读数。 -->
          <span v-else>{{ results[r.id].detail || t('contract.st.unchecked') }}</span>
        </div>
        <div class="actions">
          <el-button size="small" @click="check(r)">{{ t('rss.check') }}</el-button>
          <el-button size="small" @click="openEdit(r)">{{ t('common.edit') }}</el-button>
          <el-button size="small" type="danger" plain @click="remove(r)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
      <EmptyState
        v-if="!displayRules.length && !loading"
        icon="🔖"
        :title="focusSiteId ? t('contract.emptySiteTitle') : t('contract.emptyTitle')"
        :description="focusSiteId ? t('contract.emptySiteDesc') : t('contract.emptyDesc')"
      >
        <el-button type="primary" @click="openCreate">{{ t('contract.newRule') }}</el-button>
      </EmptyState>
    </div>

    <el-dialog v-model="showForm" :title="editing ? t('contract.editTitle') : t('contract.newTitle')" width="480px">
      <el-form :model="form" label-width="110px">
        <el-form-item :label="t('contract.f.site')">
          <el-select v-model="form.site_id" :disabled="!!editing">
            <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('rss.f.enabled')"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item :label="t('contract.f.minRatio')">
          <el-input-number v-model="form.min_ratio" :min="0" :step="0.1" controls-position="right" />
        </el-form-item>
        <el-form-item :label="t('contract.f.minSeeding')"
          ><el-input-number v-model="form.min_seeding" :min="0" controls-position="right"
        /></el-form-item>
        <el-form-item :label="t('contract.f.minUpload')"
          ><el-input-number v-model="form.min_upload_gb" :min="0" controls-position="right"
        /></el-form-item>
        <el-divider content-position="left"
          ><span class="text-muted">{{ t('contract.f.extDims') }}</span></el-divider
        >
        <el-form-item :label="t('subscriptions.officialOnly')">
          <el-switch v-model="form.official_only" />
          <span class="hint text-muted" style="margin-left: 8px">{{ t('contract.f.officialHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('contract.f.minSeedingSize')">
          <el-input-number v-model="form.min_seeding_size_gb" :min="0" controls-position="right" />
        </el-form-item>
        <el-form-item :label="t('contract.f.period')">
          <el-select v-model="form.period" style="width: 130px">
            <el-option :label="t('contract.daily')" value="" />
            <el-option :label="t('contract.weekly')" value="weekly" />
            <el-option :label="t('contract.monthly')" value="monthly" />
          </el-select>
          <span class="hint text-muted" style="margin-left: 8px">{{ t('contract.f.periodHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('contract.f.startTime')">
          <el-date-picker
            v-model="form.start_time"
            type="datetime"
            :placeholder="t('contract.f.startTimeHint')"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            style="width: 220px"
          />
        </el-form-item>
        <p class="hint text-muted">{{ t('contract.f.zeroHint') }}</p>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  margin: 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.sub {
  margin: -8px 0 0;
  font-size: var(--fs-sm);
  color: var(--text-muted);
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--sp-3);
}
.card {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.card-head {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
}
.card-title {
  flex: 1;
  min-width: 0;
}
.name {
  font-weight: 600;
  font-size: var(--fs-body);
}
.meta {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.status-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  flex-wrap: wrap;
  font-size: var(--fs-xs);
}
.detail {
  color: var(--pt-danger);
}
.time {
  color: var(--text-muted);
  margin-left: auto;
}
.check-result {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  background: rgba(127, 127, 127, 0.12);
  border-radius: var(--radius-xs);
  padding: 6px 8px;
}
.actions {
  display: flex;
  gap: var(--sp-1);
}
.hint {
  margin: 0;
}
</style>
