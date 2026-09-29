<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { entertainmentApi, type OverviewResp, type OverviewSite } from '@/api/entertainment'

// 娱乐中心总览页（批19）：全部站点 × 已适配游戏矩阵，统计卡 + 分类筛选 + 点击直达。
const { t } = useI18n()
const router = useRouter()

const data = ref<OverviewResp | null>(null)
const loading = ref(false)
const filterCat = ref<string>('all')

const CAT_META: Record<string, { icon: string; key: string }> = {
  wheel: { icon: '🎡', key: 'eo.catWheel' },
  bet: { icon: '🎲', key: 'eo.catBet' },
  stateful: { icon: '🃏', key: 'eo.catStateful' },
  farm: { icon: '🌾', key: 'eo.catFarm' },
  table: { icon: '♟️', key: 'eo.catTable' },
  other: { icon: '🎯', key: 'eo.catOther' },
}

const gameSites = computed(() => (data.value?.sites ?? []).filter((s) => s.games.length > 0))
const idleSites = computed(() => (data.value?.sites ?? []).filter((s) => s.games.length === 0))
const visibleSites = computed(() =>
  filterCat.value === 'all'
    ? gameSites.value
    : gameSites.value.filter((s) => s.games.some((g) => g.category === filterCat.value)),
)

async function load() {
  loading.value = true
  try {
    data.value = await entertainmentApi.overview()
  } finally {
    loading.value = false
  }
}

function goPlay(site: OverviewSite, gameId?: string) {
  router.push({
    name: 'entertainment',
    query: { site: String(site.id), ...(gameId ? { game: gameId } : {}) },
  })
}

onMounted(load)
</script>

<template>
  <div class="page eo">
    <div class="head">
      <h2 class="title">{{ t('eo.title') }}</h2>
      <el-button @click="router.push({ name: 'entertainment' })">🎮 {{ t('eo.goPlay') }}</el-button>
    </div>

    <section v-if="data" class="stat-row">
      <div class="stat-card glass">
        <div class="s-num">{{ data.game_sites }}</div>
        <div class="s-label text-muted">{{ t('eo.gameSites') }}</div>
      </div>
      <div class="stat-card glass">
        <div class="s-num">{{ data.total_games }}</div>
        <div class="s-label text-muted">{{ t('eo.totalGames') }}</div>
      </div>
      <div class="stat-card glass">
        <div class="s-num">{{ data.total_sites }}</div>
        <div class="s-label text-muted">{{ t('eo.totalSites') }}</div>
      </div>
      <div v-for="(n, cat) in data.categories" :key="cat" class="stat-card glass cat" :class="{ dim: filterCat !== 'all' && filterCat !== cat }" role="button" @click="filterCat = filterCat === cat ? 'all' : String(cat)">
        <div class="s-num">{{ CAT_META[cat]?.icon ?? '🎯' }} {{ n }}</div>
        <div class="s-label text-muted">{{ t(CAT_META[cat]?.key ?? 'eo.catOther') }}</div>
      </div>
    </section>

    <div v-if="filterCat !== 'all'" class="filter-bar">
      <el-tag closable @close="filterCat = 'all'">
        {{ CAT_META[filterCat]?.icon }} {{ t(CAT_META[filterCat]?.key ?? 'eo.catOther') }}
      </el-tag>
    </div>

    <section v-loading="loading" class="matrix">
      <div v-for="site in visibleSites" :key="site.id" class="site-card glass glass-hover">
        <div class="site-head" role="button" @click="goPlay(site)">
          <span class="site-name">{{ site.name }}</span>
          <span class="site-count">{{ site.games.length }}</span>
        </div>
        <div class="game-chips">
          <button
            v-for="g in site.games"
            :key="g.id"
            class="game-chip"
            :class="g.category"
            :title="t(CAT_META[g.category]?.key ?? 'eo.catOther')"
            @click.stop="goPlay(site, g.id)"
          >
            {{ CAT_META[g.category]?.icon }} {{ g.name }}
          </button>
        </div>
      </div>
      <EmptyState v-if="!loading && !visibleSites.length" :title="t('eo.empty')" />
    </section>

    <details v-if="idleSites.length" class="idle">
      <summary class="text-muted">{{ t('eo.idleSites', { n: idleSites.length }) }}</summary>
      <div class="idle-chips">
        <el-tag v-for="s in idleSites" :key="s.id" size="small" type="info">{{ s.name }}</el-tag>
      </div>
    </details>
  </div>
</template>

<style scoped lang="css">
.eo { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { font-size: var(--fs-title); font-weight: 800; }
.stat-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 10px; }
.stat-card { padding: 14px; text-align: center; border-radius: var(--radius-md); }
.stat-card.cat { cursor: pointer; transition: opacity .15s, transform .15s; }
.stat-card.cat:hover { transform: translateY(-2px); }
.stat-card.dim { opacity: .45; }
.s-num { font-size: 24px; font-weight: 800; }
.s-label { font-size: 12px; margin-top: 2px; }
.filter-bar { margin-top: -6px; }
.matrix { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; min-height: 120px; }
.site-card { padding: 12px 14px; border-radius: var(--radius-md); }
.site-head { display: flex; align-items: center; justify-content: space-between; cursor: pointer; margin-bottom: 8px; }
.site-name { font-weight: 800; font-size: 15px; }
.site-count { background: var(--primary, #409eff); color: #fff; border-radius: 999px; font-size: 12px; font-weight: 700; padding: 1px 9px; }
.game-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.game-chip { border: 1px solid var(--border-light, #e5e7eb); border-radius: 999px; background: var(--bg-card, #fff); padding: 3px 10px; font-size: 12px; font-weight: 600; cursor: pointer; transition: border-color .15s; }
.game-chip:hover { border-color: var(--primary, #409eff); color: var(--primary, #409eff); }
.idle { margin-top: 6px; }
.idle summary { cursor: pointer; font-size: 13px; }
.idle-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
</style>
