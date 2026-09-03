<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { entertainmentApi, type FarmState, type FarmItem, type PlayResult } from '@/api/entertainment'

const { t } = useI18n()
const props = defineProps<{ siteId: number; game?: string }>()
const GAME = computed(() => props.game || 'magic_farm')

const farm = ref<FarmState | null>(null)
const busy = ref(false)
const last = ref<PlayResult | null>(null)

async function refresh() {
  farm.value = await entertainmentApi.farm(props.siteId, GAME.value).catch(() => farm.value)
}

async function doAct(fn: () => Promise<PlayResult>) {
  if (busy.value) return
  busy.value = true
  try {
    const res = await fn()
    last.value = res
    if (res.success) ElMessage.success(res.message || t('sp.done'))
    else ElMessage.warning(res.message || t('common.actionFailed'))
    await refresh()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    busy.value = false
  }
}

const plant = (it: FarmItem) => doAct(() => entertainmentApi.plant(props.siteId, GAME.value, it.section, it.id))
const harvest = (it: FarmItem) => doAct(() => entertainmentApi.harvest(props.siteId, GAME.value, it.section, it.id))
const harvestAll = () => doAct(() => entertainmentApi.harvestAll(props.siteId, GAME.value))
const sell = (key: string) => doAct(() => entertainmentApi.sell(props.siteId, GAME.value, key))

function emoji(name: string) {
  if (/\u9e21/.test(name)) return '🐔'
  if (/\u732a/.test(name)) return '🐷'
  if (/\u7f8a/.test(name)) return '🐑'
  if (/\u725b/.test(name)) return '🐮'
  if (/\u9ea6/.test(name)) return '🌾'
  if (/\u7389\u7c73/.test(name)) return '🌽'
  if (/\u82b1\u751f/.test(name)) return '🥜'
  if (/\u571f\u8c46/.test(name)) return '🥔'
  return '🌱'
}
function statusText(s: string) {
  return s === 'ready' ? t('mf.ready') : s === 'growing' ? t('mf.growing') : t('mf.empty')
}

onMounted(refresh)
</script>

<template>
  <div class="farm">
    <div class="hero">
      <div class="hero-title">🚜 {{ farm?.farm_name || t('mf.title') }}</div>
      <div class="hero-sub">
        {{ t('mf.curMagic') }}: <b>{{ farm?.balance?.toLocaleString() ?? '—' }}</b>
      </div>
      <button class="harvest-all" :disabled="busy" @click="harvestAll">
        {{ busy ? t('mf.busy') : t('mf.harvestAll') }}
      </button>
    </div>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <span class="r-msg">{{ last.message }}</span>
    </div>

    <section v-if="farm" class="glass section">
      <div class="sec-title">
        🌾 {{ t('mf.crops') }} <small>{{ t('mf.valid2d') }}</small>
      </div>
      <div class="grid">
        <div v-for="c in farm.crops" :key="'c' + c.id" class="item">
          <div class="i-emoji">{{ emoji(c.name) }}</div>
          <div class="i-name">{{ c.name }}</div>
          <div class="i-meta">
            {{ c.reward ? t('mf.metaR', { h: c.grow_hours, p: c.price, r: c.reward }) : t('mf.meta', { h: c.grow_hours, p: c.price }) }}
          </div>
          <div class="i-status" :class="c.status">
            {{ c.locked ? '🔒 ' + t('mf.locked') : statusText(c.status) }}
          </div>
          <div v-if="c.locked" class="i-unlock text-muted">{{ c.unlock_text }}</div>
          <button
            class="i-btn"
            :disabled="busy || c.status === 'growing' || c.locked"
            @click="c.status === 'ready' ? harvest(c) : plant(c)"
          >
            {{ c.status === 'ready' ? t('mf.harvest') : c.status === 'growing' ? t('mf.growing') : t('mf.plant') }}
          </button>
        </div>
      </div>
    </section>

    <section v-if="farm" class="glass section">
      <div class="sec-title">
        🐔 {{ t('mf.animals') }} <small>{{ t('mf.valid5d') }}</small>
      </div>
      <div class="grid">
        <div v-for="a in farm.animals" :key="'a' + a.id" class="item">
          <div class="i-emoji">{{ emoji(a.name) }}</div>
          <div class="i-name">{{ a.name }}</div>
          <div class="i-meta">{{ t('mf.meta', { h: a.grow_hours, p: a.price }) }}</div>
          <div class="i-status" :class="a.status">{{ statusText(a.status) }}</div>
          <button
            class="i-btn"
            :disabled="busy || a.status === 'growing'"
            @click="a.status === 'ready' ? harvest(a) : plant(a)"
          >
            {{ a.status === 'ready' ? t('mf.harvest') : a.status === 'growing' ? t('mf.growing') : t('mf.raise') }}
          </button>
        </div>
      </div>
    </section>

    <section v-if="farm?.warehouse?.length" class="glass section">
      <div class="sec-title">📦 {{ t('mf.barn') }}</div>
      <div class="list">
        <div v-for="(w, i) in farm.warehouse" :key="i" class="row">
          <span class="row-name">{{ w.name }} ×{{ w.count }}</span>
          <span v-if="w.market_price" class="text-muted">{{ t('mf.marketPrice', { v: w.market_price }) }}</span>
          <button class="sell-btn" :disabled="busy" @click="sell(w.key)">{{ t('mf.sell') }}</button>
        </div>
      </div>
    </section>

    <section v-if="farm?.market?.length" class="glass section">
      <div class="sec-title">
        🛒 {{ t('mf.market') }} <small>{{ t('mf.marketHint') }}</small>
      </div>
      <div class="list">
        <div v-for="(m, i) in farm.market" :key="i" class="row">
          <span class="row-name"
            >{{ m.name }}
            <small class="text-muted">{{ m.section === 'crop' ? t('mf.cropKind') : t('mf.animalKind') }}</small></span
          >
          <b class="mkt-price">{{ m.market_price.toLocaleString() }}</b>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.farm {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 720px;
  margin: 0 auto;
}
.hero {
  padding: 18px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, #166534, #16a34a);
  text-align: center;
}
.hero-title {
  font-size: var(--fs-title);
  font-weight: 800;
  color: #fff;
}
.hero-sub {
  margin-top: 4px;
  font-size: var(--fs-body);
  color: rgba(255, 255, 255, 0.9);
}
.hero-sub b {
  color: #fef08a;
  font-size: var(--fs-lg);
}
.harvest-all {
  margin-top: 12px;
  padding: 9px 22px;
  border: 0;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-weight: 700;
  color: #166534;
  background: #fff;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}
.harvest-all:disabled {
  opacity: 0.7;
  cursor: wait;
}

.section {
  padding: 16px;
  border-radius: var(--radius-md);
}
.sec-title {
  font-weight: 700;
  margin-bottom: 12px;
  font-size: var(--fs-lg);
}
.sec-title small {
  font-weight: 400;
  color: var(--text-muted, #999);
  font-size: var(--fs-xs);
  margin-left: 6px;
}

.grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--sp-2);
}
@media (max-width: 560px) {
  .grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
.item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-1);
  padding: 12px 8px;
  border-radius: var(--radius-sm);
  background: var(--glass-bg, rgba(255, 255, 255, 0.06));
  border: 1px solid var(--border-soft, rgba(255, 255, 255, 0.08));
}
.i-emoji {
  font-size: var(--fs-hero);
}
.i-name {
  font-weight: 700;
}
.i-meta {
  font-size: var(--fs-2xs);
  color: var(--text-muted, #999);
}
.i-status {
  font-size: var(--fs-2xs);
  padding: 2px 8px;
  border-radius: var(--radius-xs);
}
.i-status.ready {
  background: var(--pt-success-bg);
  color: var(--pt-success);
}
.i-status.growing {
  background: var(--pt-warn-bg);
  color: var(--pt-warn);
}
.i-status.idle {
  background: rgba(148, 163, 184, 0.18);
  color: #94a3b8;
}
.i-btn {
  margin-top: 6px;
  padding: 5px 14px;
  border: 0;
  border-radius: var(--radius-xs);
  cursor: pointer;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: #fff;
  background: #16a34a;
}
.i-btn:disabled {
  background: #94a3b8;
  cursor: not-allowed;
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.row {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 7px 4px;
  font-size: var(--fs-body);
  border-bottom: 1px solid var(--border-soft, rgba(255, 255, 255, 0.05));
}
.row-name {
  font-weight: 600;
}
.mkt-price {
  margin-left: auto;
  color: var(--pt-warn);
}
.sell-btn {
  margin-left: auto;
  padding: 3px 12px;
  border: 0;
  border-radius: var(--radius-xs);
  cursor: pointer;
  font-size: var(--fs-xs);
  color: #fff;
  background: #3b82f6;
}
.sell-btn:disabled {
  opacity: 0.6;
  cursor: wait;
}

.result {
  padding: 12px;
  border-radius: var(--radius-sm);
  display: flex;
  gap: var(--sp-2);
  align-items: center;
}
.result.win {
  background: var(--pt-success-bg);
}
.result.lose {
  background: var(--pt-danger-bg);
}
.badge {
  font-weight: 700;
}
.r-msg {
  font-size: var(--fs-body);
}

.i-unlock { font-size: 11px; line-height: 1.3; margin: 2px 0; }
</style>
