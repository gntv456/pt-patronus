<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { entertainmentApi, type FarmState, type FarmItem, type PlayResult } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()
const GAME = 'magic_farm'

const farm = ref<FarmState | null>(null)
const busy = ref(false)
const last = ref<PlayResult | null>(null)

async function refresh() {
  farm.value = await entertainmentApi.farm(props.siteId, GAME).catch(() => farm.value)
}

async function doAct(fn: () => Promise<PlayResult>) {
  if (busy.value) return
  busy.value = true
  try {
    const res = await fn()
    last.value = res
    if (res.success) ElMessage.success(res.message || '完成')
    else ElMessage.warning(res.message || '操作失败')
    await refresh()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    busy.value = false
  }
}

const plant = (it: FarmItem) => doAct(() => entertainmentApi.plant(props.siteId, GAME, it.section, it.id))
const harvest = (it: FarmItem) => doAct(() => entertainmentApi.harvest(props.siteId, GAME, it.section, it.id))
const harvestAll = () => doAct(() => entertainmentApi.harvestAll(props.siteId, GAME))
const sell = (key: string) => doAct(() => entertainmentApi.sell(props.siteId, GAME, key))

function emoji(name: string) {
  if (/鸡/.test(name)) return '🐔'
  if (/猪/.test(name)) return '🐷'
  if (/羊/.test(name)) return '🐑'
  if (/牛/.test(name)) return '🐮'
  if (/麦/.test(name)) return '🌾'
  if (/玉米/.test(name)) return '🌽'
  if (/花生/.test(name)) return '🥜'
  if (/土豆/.test(name)) return '🥔'
  return '🌱'
}
function statusText(s: string) {
  return s === 'ready' ? '可收获' : s === 'growing' ? '生长中' : '未种植'
}

onMounted(refresh)
</script>

<template>
  <div class="farm">
    <div class="hero">
      <div class="hero-title">🚜 {{ farm?.farm_name || '魔力农场' }}</div>
      <div class="hero-sub">当前魔力值: <b>{{ farm?.balance?.toLocaleString() ?? '—' }}</b></div>
      <button class="harvest-all" :disabled="busy" @click="harvestAll">{{ busy ? '处理中…' : '一键收获 🧺' }}</button>
    </div>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <span class="r-msg">{{ last.message }}</span>
    </div>

    <section v-if="farm" class="glass section">
      <div class="sec-title">🌾 农作物种植区 <small>有效期 2 天</small></div>
      <div class="grid">
        <div v-for="c in farm.crops" :key="'c' + c.id" class="item">
          <div class="i-emoji">{{ emoji(c.name) }}</div>
          <div class="i-name">{{ c.name }}</div>
          <div class="i-meta">成长 {{ c.grow_hours }}h · 买入 {{ c.price }}</div>
          <div class="i-status" :class="c.status">{{ statusText(c.status) }}</div>
          <button class="i-btn" :disabled="busy || c.status === 'growing'" @click="c.status === 'ready' ? harvest(c) : plant(c)">
            {{ c.status === 'ready' ? '收获' : c.status === 'growing' ? '生长中' : '种植' }}
          </button>
        </div>
      </div>
    </section>

    <section v-if="farm" class="glass section">
      <div class="sec-title">🐔 动物养殖区 <small>有效期 5 天</small></div>
      <div class="grid">
        <div v-for="a in farm.animals" :key="'a' + a.id" class="item">
          <div class="i-emoji">{{ emoji(a.name) }}</div>
          <div class="i-name">{{ a.name }}</div>
          <div class="i-meta">成长 {{ a.grow_hours }}h · 买入 {{ a.price }}</div>
          <div class="i-status" :class="a.status">{{ statusText(a.status) }}</div>
          <button class="i-btn" :disabled="busy || a.status === 'growing'" @click="a.status === 'ready' ? harvest(a) : plant(a)">
            {{ a.status === 'ready' ? '收获' : a.status === 'growing' ? '生长中' : '养殖' }}
          </button>
        </div>
      </div>
    </section>

    <section v-if="farm?.warehouse?.length" class="glass section">
      <div class="sec-title">📦 仓库</div>
      <div class="list">
        <div v-for="(w, i) in farm.warehouse" :key="i" class="row">
          <span class="row-name">{{ w.name }} ×{{ w.count }}</span>
          <span v-if="w.market_price" class="text-muted">市价 {{ w.market_price }}</span>
          <button class="sell-btn" :disabled="busy" @click="sell(w.key)">出售</button>
        </div>
      </div>
    </section>

    <section v-if="farm?.market?.length" class="glass section">
      <div class="sec-title">🛒 菜市场 <small>每 24h 更新，波动 ±20%</small></div>
      <div class="list">
        <div v-for="(m, i) in farm.market" :key="i" class="row">
          <span class="row-name">{{ m.name }} <small class="text-muted">{{ m.section === 'crop' ? '农作物' : '动物' }}</small></span>
          <b class="mkt-price">{{ m.market_price.toLocaleString() }}</b>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.farm { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 720px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #166534, #16a34a); text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; color: #fff; }
.hero-sub { margin-top: 4px; font-size: var(--fs-body); color: rgba(255, 255, 255, 0.9); }
.hero-sub b { color: #fef08a; font-size: var(--fs-lg); }
.harvest-all { margin-top: 12px; padding: 9px 22px; border: 0; border-radius: var(--radius-sm); cursor: pointer; font-weight: 700; color: #166534; background: #fff; box-shadow: 0 4px 12px rgba(0,0,0,0.15); }
.harvest-all:disabled { opacity: 0.7; cursor: wait; }

.section { padding: 16px; border-radius: var(--radius-md); }
.sec-title { font-weight: 700; margin-bottom: 12px; font-size: var(--fs-lg); }
.sec-title small { font-weight: 400; color: var(--text-muted, #999); font-size: var(--fs-xs); margin-left: 6px; }

.grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--sp-2); }
@media (max-width: 560px) { .grid { grid-template-columns: repeat(2, 1fr); } }
.item { display: flex; flex-direction: column; align-items: center; gap: var(--sp-1); padding: 12px 8px; border-radius: var(--radius-sm); background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); }
.i-emoji { font-size: var(--fs-hero); }
.i-name { font-weight: 700; }
.i-meta { font-size: var(--fs-2xs); color: var(--text-muted, #999); }
.i-status { font-size: var(--fs-2xs); padding: 2px 8px; border-radius: var(--radius-xs); }
.i-status.ready { background: var(--pt-success-bg); color: var(--pt-success); }
.i-status.growing { background: var(--pt-warn-bg); color: var(--pt-warn); }
.i-status.idle { background: rgba(148,163,184,0.18); color: #94a3b8; }
.i-btn { margin-top: 6px; padding: 5px 14px; border: 0; border-radius: var(--radius-xs); cursor: pointer; font-size: var(--fs-xs); font-weight: 600; color: #fff; background: #16a34a; }
.i-btn:disabled { background: #94a3b8; cursor: not-allowed; }

.list { display: flex; flex-direction: column; gap: var(--sp-1); }
.row { display: flex; align-items: center; gap: var(--sp-3); padding: 7px 4px; font-size: var(--fs-body); border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.row-name { font-weight: 600; }
.mkt-price { margin-left: auto; color: var(--pt-warn); }
.sell-btn { margin-left: auto; padding: 3px 12px; border: 0; border-radius: var(--radius-xs); cursor: pointer; font-size: var(--fs-xs); color: #fff; background: #3b82f6; }
.sell-btn:disabled { opacity: 0.6; cursor: wait; }

.result { padding: 12px; border-radius: var(--radius-sm); display: flex; gap: var(--sp-2); align-items: center; }
.result.win { background: var(--pt-success-bg); }
.result.lose { background: var(--pt-danger-bg); }
.badge { font-weight: 700; }
.r-msg { font-size: var(--fs-body); }
</style>
