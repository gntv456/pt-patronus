<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { subscribeApi, type Subscription } from '@/api/subscribe'
import MediaPoster from '@/components/MediaPoster.vue'

// 订阅列表：展示订阅状态，支持手动检查 / 暂停-恢复 / 改分辨率 / 删除。
const list = ref<Subscription[]>([])
const loading = ref(false)
const checking = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    list.value = await subscribeApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function checkNow(s: Subscription) {
  checking.value = s.id
  try {
    const r = await subscribeApi.check(s.id)
    if (r.ok) {
      ElMessage.success(r.pushed > 0 ? `命中并推送 ${r.pushed} 条` : '本轮暂无新资源')
    } else {
      ElMessage.warning(r.error || '检查失败')
    }
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '检查失败')
  } finally {
    checking.value = null
  }
}

async function toggleStatus(s: Subscription) {
  const next = s.status === 'paused' ? 'subscribed' : 'paused'
  try {
    await subscribeApi.update(s.id, { status: next })
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '操作失败')
  }
}

async function setResolution(s: Subscription) {
  try {
    await subscribeApi.update(s.id, { resolution: s.resolution })
    ElMessage.success('分辨率已更新')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '更新失败')
  }
}

async function remove(s: Subscription) {
  try {
    await subscribeApi.remove(s.id)
    await load()
    ElMessage.success('已删除')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '删除失败')
  }
}

function statusText(s: string) {
  return ({ subscribed: '订阅中', completed: '已完成', paused: '已暂停' } as Record<string, string>)[s] || s
}
function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '—'
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">订阅</h2>
      <span class="text-muted hint">cron 自动跨站搜索，命中即推送到默认下载器并通知。点此 <router-link to="/search">搜索</router-link> → 影视详情页订阅。</span>
    </div>

    <div v-loading="loading" class="card glass">
      <div v-if="list.length" class="subs">
        <div v-for="s in list" :key="s.id" class="sub glass-hover">
          <div class="poster"><MediaPoster :url="s.poster_url" :kind="s.type" /></div>
          <div class="info">
            <div class="row1">
              <span class="s-title">{{ s.title }}</span>
              <span v-if="s.year" class="text-muted">({{ s.year }})</span>
              <span class="chip" :class="s.status">{{ statusText(s.status) }}</span>
              <span v-if="s.resolution" class="chip res">{{ s.resolution }}</span>
            </div>
            <div class="text-muted small">
              上次检查：{{ fmtTime(s.last_checked_at) }} · 类型 {{ s.type === 'tv' ? '剧集' : '电影' }} · 源 {{ s.source }}
            </div>
            <div class="res-row">
              <span class="small">分辨率过滤：</span>
              <el-input
                v-model="s.resolution"
                size="small"
                style="width: 120px"
                placeholder="不限"
                @change="setResolution(s)"
              />
              <span class="small text-muted">（如 1080p / 2160p，留空=不限）</span>
            </div>
            <div class="actions">
              <el-button size="small" type="primary" :loading="checking === s.id" @click="checkNow(s)">立即检查</el-button>
              <el-button size="small" @click="toggleStatus(s)">{{ s.status === 'paused' ? '恢复' : '暂停' }}</el-button>
              <el-button size="small" type="danger" plain @click="remove(s)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
      <div v-else-if="!loading" class="empty text-muted">
        暂无订阅。去 <router-link to="/search">搜索</router-link> → 切到「影视」→ 点结果进详情 → 订阅。
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; flex-direction: column; gap: 4px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.hint { font-size: 13px; }
.card { padding: 18px; min-height: 160px; }
.subs { display: flex; flex-direction: column; gap: 14px; }
.sub { display: flex; gap: 14px; padding: 14px; border-radius: 12px; background: rgba(0,0,0,0.03); }
.poster { width: 72px; height: 104px; flex: none; border-radius: 8px; overflow: hidden; background: rgba(0,0,0,0.08); }
.info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
.row1 { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.s-title { font-weight: 700; font-size: 16px; }
.small { font-size: 12px; }
.chip { padding: 2px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; }
.chip.subscribed { background: rgba(46,204,113,0.18); color: var(--pt-success, #2ecc71); }
.chip.completed { background: rgba(108,92,231,0.18); color: #6c5ce7; }
.chip.paused { background: rgba(243,156,18,0.18); color: var(--pt-warn, #f39c12); }
.chip.res { background: rgba(66,133,244,0.18); color: #4285f4; }
.res-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 2px; }
.empty { padding: 40px; text-align: center; }
</style>
