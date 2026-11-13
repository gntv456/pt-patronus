<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { entertainmentApi, type OverviewResp, type OverviewSite } from '@/api/entertainment'
import SptIcon from '@/components/SptIcon.vue'
import { useQueryState } from '@/composables/useUrlState'

// 娱乐中心总览页（批19）：全部站点 × 已适配游戏矩阵，统计卡 + 分类筛选 + 点击直达。
const { t } = useI18n()
const router = useRouter()

const data = ref<OverviewResp | null>(null)
const loading = ref(false)
// 分类筛选写回 URL query（?cat=），支持刷新 / 分享 / 后退恢复；空串表示「全部」。
const filterCat = useQueryState('cat', '')

// 结构性图标统一用单色 SptIcon（去 emoji），值为 SptIcon 的 name。
const CAT_META: Record<string, { icon: string; key: string }> = {
  wheel: { icon: 'loader', key: 'eo.catWheel' },
  bet: { icon: 'percent', key: 'eo.catBet' },
  stateful: { icon: 'layers', key: 'eo.catStateful' },
  farm: { icon: 'package', key: 'eo.catFarm' },
  table: { icon: 'grid', key: 'eo.catTable' },
  other: { icon: 'target', key: 'eo.catOther' },
}

const gameSites = computed(() => (data.value?.sites ?? []).filter((s) => s.games.length > 0))
const idleSites = computed(() => (data.value?.sites ?? []).filter((s) => s.games.length === 0))
const visibleSites = computed(() =>
  !filterCat.value
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

// 未适配站点：一次带上全部 id 前往站点管理配置（query.site = 逗号分隔的 id 列表）。
function goConfigure(ids: number[]) {
  router.push({ name: 'sites', query: { site: ids.join(',') } })
}

onMounted(load)
</script>

<template>
  <div class="page eo">
    <div class="head">
      <h2 class="title">{{ t('eo.title') }}</h2>
      <el-button type="primary" @click="router.push({ name: 'entertainment' })">
        <SptIcon name="gamepad" :size="16" aria-hidden="true" /> {{ t('eo.goPlay') }}
      </el-button>
    </div>

    <section v-if="data" class="stat-row">
      <div class="stat-card solid-card">
        <div class="s-num">{{ data.game_sites }}</div>
        <div class="s-label text-muted">{{ t('eo.gameSites') }}</div>
      </div>
      <div class="stat-card solid-card">
        <div class="s-num">{{ data.total_games }}</div>
        <div class="s-label text-muted">{{ t('eo.totalGames') }}</div>
      </div>
      <div class="stat-card solid-card">
        <div class="s-num">{{ data.total_sites }}</div>
        <div class="s-label text-muted">{{ t('eo.totalSites') }}</div>
      </div>
      <button
        v-for="(n, cat) in data.categories"
        :key="cat"
        type="button"
        class="stat-card solid-card cat"
        :class="{ dim: filterCat !== '' && filterCat !== cat }"
        :aria-pressed="filterCat === cat"
        @click="filterCat = filterCat === cat ? '' : String(cat)"
      >
        <div class="s-num">
          <SptIcon :name="CAT_META[cat]?.icon ?? 'target'" :size="16" aria-hidden="true" /> {{ n }}
        </div>
        <div class="s-label text-muted">{{ t(CAT_META[cat]?.key ?? 'eo.catOther') }}</div>
      </button>
    </section>

    <div v-if="filterCat !== ''" class="filter-bar">
      <el-tag closable @close="filterCat = ''">
        <SptIcon :name="CAT_META[filterCat]?.icon ?? 'target'" :size="16" aria-hidden="true" />
        {{ t(CAT_META[filterCat]?.key ?? 'eo.catOther') }}
      </el-tag>
    </div>

    <section class="matrix">
      <template v-if="loading">
        <div v-for="n in 6" :key="n" class="site-card solid-card">
          <el-skeleton :rows="3" animated />
        </div>
      </template>
      <template v-else>
        <div v-for="site in visibleSites" :key="site.id" class="site-card solid-card">
          <button type="button" class="site-head" @click="goPlay(site)">
            <span class="site-name">{{ site.name }}</span>
            <span class="site-count">{{ site.games.length }}</span>
          </button>
          <div class="game-chips">
            <button
              v-for="g in site.games"
              :key="g.id"
              type="button"
              class="game-chip"
              :class="g.category"
              :title="t(CAT_META[g.category]?.key ?? 'eo.catOther')"
              @click.stop="goPlay(site, g.id)"
            >
              <SptIcon :name="CAT_META[g.category]?.icon ?? 'target'" :size="16" aria-hidden="true" />
              {{ g.name }}
            </button>
          </div>
        </div>
        <EmptyState v-if="!visibleSites.length" :title="t('eo.empty')" />
      </template>
    </section>

    <details v-if="idleSites.length" class="idle">
      <summary class="text-muted">{{ t('eo.idleSites', { n: idleSites.length }) }}</summary>
      <div class="idle-chips">
        <el-tag v-for="s in idleSites" :key="s.id" size="small" type="info">{{ s.name }}</el-tag>
        <button
          type="button"
          class="idle-config"
          :aria-label="t('dts.goSites')"
          @click="goConfigure(idleSites.map((s) => s.id))"
        >
          <SptIcon name="wrench" :size="14" aria-hidden="true" />
          {{ t('dts.goSites') }}
        </button>
      </div>
    </details>
  </div>
</template>

<style scoped lang="css">
.eo { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { font-size: var(--fs-title); font-weight: 800; }
.stat-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 10px; }
.stat-card {
  padding: 14px;
  text-align: center;
  border-radius: var(--radius-md);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  font: inherit;
}
.stat-card.cat { cursor: pointer; transition: border-color var(--dur-fast) var(--ease); }
.stat-card.cat:hover { border-color: var(--color-border-strong); }
.stat-card.cat[aria-pressed='true'] { border-color: var(--color-primary); }
/* 不用 opacity 压暗（会拉低文字对比度）：改用边框色区分选中态，并补 aria-pressed。 */
.stat-card.dim { opacity: 1; border-color: var(--color-border); }
.s-num {
  font-size: var(--fs-2xl);
  font-weight: 800;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-variant-numeric: tabular-nums;
}
.s-label { font-size: var(--fs-xs); margin-top: 2px; }
.filter-bar { margin-top: -6px; }
.matrix { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; min-height: 120px; }
.site-card { padding: 12px 14px; border-radius: var(--radius-md); }
.site-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
  margin-bottom: 8px;
  width: 100%;
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  text-align: left;
}
.site-name { font-weight: 800; font-size: 15px; }
.site-count {
  background: var(--color-primary);
  color: var(--color-on-accent);
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
  font-weight: 700;
  padding: 1px 9px;
  font-variant-numeric: tabular-nums;
}
.game-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.game-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 24px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-pill);
  background: var(--color-surface);
  color: var(--color-text-secondary);
  padding: 3px 10px;
  font-size: var(--fs-xs);
  font-weight: 600;
  cursor: pointer;
  transition: border-color .15s, color .15s;
}
.game-chip:hover { border-color: var(--color-primary); color: var(--color-primary); }
.idle { margin-top: 6px; }
.idle summary { cursor: pointer; font-size: 13px; }
.idle-chips { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-top: 8px; }
.idle-config {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 24px;
  padding: 2px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-surface);
  color: var(--color-primary);
  font-size: var(--fs-xs);
  font-weight: 600;
  cursor: pointer;
}
.idle-config:hover { border-color: var(--color-primary); background: var(--color-primary-soft); }
@media (max-width: 640px) {
  .s-label, .game-chip, .idle-config { font-size: 16px; }
}
</style>
