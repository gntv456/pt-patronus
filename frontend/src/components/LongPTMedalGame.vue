<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import { entertainmentApi, type MedalNode, type MedalShard } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()

const GAME = 'longpt_medal'

// 层级顺序 + 中文名（太极→两仪→四象→八卦→六十四卦）
const LEVELS = [
  { type: 'taiji', name: '太极' },
  { type: 'liangyi', name: '两仪' },
  { type: 'sixiang', name: '四象' },
  { type: 'bagua', name: '八卦' },
  { type: 'liushisi', name: '六十四卦' },
]

const tree = ref<MedalNode[]>([])
const shards = ref<MedalShard | null>(null)
const loading = ref(true)
const composing = ref('') // 正在合成的 medal_id
const converting = ref(0) // 正在转换的方向（1/-1）
const fetchError = ref('') // 勋章列表加载失败原因（诊断用）

function isBigType(type: string) {
  return type === 'bagua' || type === 'liushisi'
}

// 按 level 分组的金字塔
const levels = computed(() =>
  LEVELS.map((l) => ({ ...l, nodes: tree.value.filter((n) => n.type === l.type) })).filter((l) => l.nodes.length),
)

// 某 medalId 的直接子勋章
function childrenOf(node: MedalNode) {
  return tree.value.filter((n) => n.parent_id === node.medal_id)
}

// 大勋章是否可合成（全部子勋章 owned）
function canComposeBig(node: MedalNode) {
  const kids = childrenOf(node)
  return kids.length > 0 && kids.every((c) => c.owned)
}

// 碎片合成是否够（阳爻/阴爻余额 ≥ 所需）
function canComposeShard(node: MedalNode) {
  if (!shards.value) return false
  return shards.value.yang_yao >= node.need_yang && shards.value.yin_yao >= node.need_yin
}

// 综合：该勋章当前是否可合成（碎片层看余额，大勋章层看子勋章）
function canCompose(node: MedalNode) {
  return isBigType(node.type) ? canComposeBig(node) : canComposeShard(node)
}

function costText(node: MedalNode) {
  if (isBigType(node.type)) return `消耗 ${node.big_cost} 魔力 + 全部子勋章`
  return `消耗 ${node.need_yang} 阳爻 + ${node.need_yin} 阴爻`
}

async function refresh() {
  loading.value = true
  fetchError.value = ''
  try {
    const [t, s] = await Promise.all([
      entertainmentApi.medalTree(props.siteId, GAME).catch((e: unknown) => {
        fetchError.value = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || '加载勋章列表失败'
        return [] as MedalNode[]
      }),
      entertainmentApi.medalShards(props.siteId, GAME).catch(() => null),
    ])
    tree.value = t
    shards.value = s
  } finally {
    loading.value = false
  }
}

async function compose(node: MedalNode) {
  const big = isBigType(node.type)
  if (big && !canComposeBig(node)) {
    ElMessage.warning('需要拥有全部子勋章才能合成')
    return
  }
  try {
    await ElMessageBox.confirm(`是否${costText(node)}合成【${node.name}】？`, '合成确认', {
      type: 'warning',
      confirmButtonText: '确认合成',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  composing.value = node.medal_id
  try {
    const res = big
      ? await entertainmentApi.composeBigMedal(props.siteId, GAME, node.medal_id)
      : await entertainmentApi.composeMedal(props.siteId, GAME, node.medal_id)
    if (res.success) ElMessage.success(res.message || '合成成功')
    else ElMessage.warning(res.message || '合成失败')
    await refresh()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    composing.value = ''
  }
}

async function convert(yiYang: number) {
  try {
    await ElMessageBox.confirm(
      `是否花费 10000 魔力将 1 个${yiYang < 0 ? '阳爻' : '阴爻'}转换为${yiYang < 0 ? '阴爻' : '阳爻'}？`,
      '阴阳转换',
      { type: 'warning', confirmButtonText: '确认转换', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  converting.value = yiYang
  try {
    const res = await entertainmentApi.convertShard(props.siteId, GAME, yiYang)
    if (res.success) ElMessage.success(res.message || '转换成功')
    else ElMessage.warning(res.message || '转换失败')
    await refresh()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    converting.value = 0
  }
}

onMounted(refresh)
</script>

<template>
  <div class="lpt-medal" v-loading="loading">
    <div class="banner">
      <span class="banner-title">☯ 六十四卦勋章</span>
      <span class="banner-sub">收集阳爻/阴爻碎片，合成太极→两仪→四象→八卦→六十四卦</span>
    </div>

    <!-- 碎片栏 -->
    <div class="shard-bar glass">
      <div class="shard">
        <span class="shard-icon">⚊</span>
        <div>
          <div class="shard-val">{{ shards?.yang_yao ?? '--' }}</div>
          <div class="shard-lbl">阳爻</div>
        </div>
        <el-button size="small" :disabled="!shards || shards.yang_yao < 1 || converting === -1"
          :loading="converting === -1" @click="convert(-1)">→阴爻</el-button>
      </div>
      <div class="shard">
        <span class="shard-icon">⚋</span>
        <div>
          <div class="shard-val">{{ shards?.yin_yao ?? '--' }}</div>
          <div class="shard-lbl">阴爻</div>
        </div>
        <el-button size="small" :disabled="!shards || shards.yin_yao < 1 || converting === 1"
          :loading="converting === 1" @click="convert(1)">→阳爻</el-button>
      </div>
      <div class="convert-tip text-muted">阴阳互转：10000 魔力 / 个</div>
    </div>

    <!-- 金字塔 -->
    <div v-for="lvl in levels" :key="lvl.type" class="level">
      <div class="level-title">
        {{ lvl.name }}勋章
        <span class="level-count text-muted">{{ lvl.nodes.filter((n) => n.owned).length }}/{{ lvl.nodes.length }}</span>
      </div>
      <div class="node-grid">
        <div v-for="node in lvl.nodes" :key="node.medal_id" class="node" :class="{ owned: node.owned }">
          <div class="node-img-wrap">
            <img v-if="node.image_url" :src="node.image_url" :alt="node.name" class="node-img" :class="{ gray: !node.owned }" />
            <span v-else class="node-placeholder">☯</span>
            <span v-if="node.owned" class="owned-badge">✓</span>
          </div>
          <div class="node-name">{{ node.name }}</div>
          <div class="node-cost text-muted">{{ costText(node) }}</div>
          <el-button v-if="!node.owned" size="small" :type="canCompose(node) ? 'primary' : 'info'"
            :disabled="!!composing || !canCompose(node)"
            :loading="composing === node.medal_id" @click="compose(node)">
            {{ canCompose(node) ? (isBigType(node.type) ? '大合成' : '合成') : (isBigType(node.type) ? '子勋章不足' : '碎片不足') }}
          </el-button>
          <div v-else class="owned-tag">已拥有</div>
        </div>
      </div>
    </div>

    <div v-if="!loading && !tree.length" class="empty glass">
      <div v-if="fetchError" class="err">{{ fetchError }}</div>
      <div v-else class="text-muted">暂无勋章数据</div>
    </div>
  </div>
</template>

<style scoped>
.lpt-medal { display: flex; flex-direction: column; gap: 16px; max-width: 720px; margin: 0 auto; }
.banner { text-align: center; padding: 14px; border-radius: 14px; background: linear-gradient(135deg, #2d3436, #636e72); }
.banner-title { display: block; font-size: 22px; font-weight: 800; color: #ffd76a; letter-spacing: 2px; }
.banner-sub { display: block; margin-top: 3px; font-size: 12px; color: rgba(255, 255, 255, 0.7); }

.shard-bar { display: flex; gap: 16px; padding: 14px; border-radius: 12px; align-items: center; flex-wrap: wrap; }
.shard { display: flex; align-items: center; gap: 8px; }
.shard-icon { font-size: 28px; color: var(--pt-primary, #6c5ce7); }
.shard-val { font-size: 20px; font-weight: 800; }
.shard-lbl { font-size: 11px; color: var(--text-muted, #999); }
.convert-tip { width: 100%; font-size: 11px; margin-top: 4px; }

.level { display: flex; flex-direction: column; gap: 8px; }
.level-title { font-weight: 700; font-size: 15px; display: flex; align-items: center; gap: 8px; }
.level-count { font-size: 12px; font-weight: 400; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 10px; }

.node { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 12px 8px; border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); text-align: center; }
.node.owned { border-color: rgba(46, 204, 113, 0.4); }
.node-img-wrap { position: relative; width: 56px; height: 56px; }
.node-img { width: 100%; height: 100%; object-fit: contain; }
.node-img.gray { filter: grayscale(1) opacity(0.5); }
.node-placeholder { font-size: 40px; color: var(--text-muted, #999); }
.owned-badge { position: absolute; top: -4px; right: -4px; width: 20px; height: 20px; border-radius: 50%; background: #2ecc71; color: #fff; font-size: 12px; display: flex; align-items: center; justify-content: center; }
.node-name { font-size: 12px; font-weight: 600; }
.node-cost { font-size: 10px; line-height: 1.3; min-height: 26px; }
.owned-tag { font-size: 12px; color: #2ecc71; font-weight: 600; height: 24px; line-height: 24px; }
.empty { padding: 40px; text-align: center; }
.err { color: var(--pt-danger); font-size: 13px; word-break: break-all; }
</style>
