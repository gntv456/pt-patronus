<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { entertainmentApi, type MedalNode, type MedalShard } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()

const GAME = 'longpt_medal'

// 层级顺序 + 中文名（太极→两仪→四象→八卦→六十四卦）
const LEVELS = computed(() => [
  { type: 'taiji', name: t('mg.lv.taiji') },
  { type: 'liangyi', name: t('mg.lv.liangyi') },
  { type: 'sixiang', name: t('mg.lv.sixiang') },
  { type: 'bagua', name: t('mg.lv.bagua') },
  { type: 'liushisi', name: t('mg.lv.liushisi') },
])

const tree = ref<MedalNode[]>([])
const shards = ref<MedalShard | null>(null)
const loading = ref(true)
const { t } = useI18n()
const composing = ref('') // 正在合成的 medal_id
const converting = ref(0) // 正在转换的方向（1/-1）
const fetchError = ref('') // 勋章列表加载失败原因（诊断用）

function isBigType(type: string) {
  return type === 'bagua' || type === 'liushisi'
}

// 按 level 分组的金字塔
const levels = computed(() =>
  LEVELS.value.map((l) => ({ ...l, nodes: tree.value.filter((n) => n.type === l.type) })).filter((l) => l.nodes.length),
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
  if (isBigType(node.type)) return t('mg.costBig', { n: node.big_cost })
  return t('mg.costShards', { yang: node.need_yang, yin: node.need_yin })
}

async function refresh() {
  loading.value = true
  fetchError.value = ''
  try {
    const [treeData, shardData] = await Promise.all([
      entertainmentApi.medalTree(props.siteId, GAME).catch((e: unknown) => {
        fetchError.value =
          (e as { response?: { data?: { error?: string } } })?.response?.data?.error || t('mg.loadFailed')
        return [] as MedalNode[]
      }),
      entertainmentApi.medalShards(props.siteId, GAME).catch(() => null),
    ])
    tree.value = treeData
    shards.value = shardData
  } finally {
    loading.value = false
  }
}

async function compose(node: MedalNode) {
  const big = isBigType(node.type)
  if (big && !canComposeBig(node)) {
    ElMessage.warning(t('mg.needChildren'))
    return
  }
  try {
    await ElMessageBox.confirm(
      t('mg.composeConfirm', { cost: costText(node), name: node.name }),
      t('mg.composeTitle'),
      {
        type: 'warning',
        confirmButtonText: t('mg.confirmCompose'),
        cancelButtonText: t('common.cancel'),
      },
    )
  } catch {
    return
  }
  composing.value = node.medal_id
  try {
    const res = big
      ? await entertainmentApi.composeBigMedal(props.siteId, GAME, node.medal_id)
      : await entertainmentApi.composeMedal(props.siteId, GAME, node.medal_id)
    if (res.success) ElMessage.success(res.message || t('mg.composeOk'))
    else ElMessage.warning(res.message || t('mg.composeFailed'))
    await refresh()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    composing.value = ''
  }
}

async function convert(yiYang: number) {
  try {
    await ElMessageBox.confirm(
      t('mg.convertConfirm', {
        from: yiYang < 0 ? t('mg.yang') : t('mg.yin'),
        to: yiYang < 0 ? t('mg.yin') : t('mg.yang'),
      }),
      t('mg.convertTitle'),
      { type: 'warning', confirmButtonText: t('mg.confirmConvert'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
  converting.value = yiYang
  try {
    const res = await entertainmentApi.convertShard(props.siteId, GAME, yiYang)
    if (res.success) ElMessage.success(res.message || t('mg.convertOk'))
    else ElMessage.warning(res.message || t('mg.convertFailed'))
    await refresh()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    converting.value = 0
  }
}

onMounted(refresh)
</script>

<template>
  <div class="lpt-medal" v-loading="loading">
    <div class="banner">
      <span class="banner-title">☯ {{ t('mg.banner') }}</span>
      <span class="banner-sub">{{ t('mg.bannerSub') }}</span>
    </div>

    <!-- 碎片栏 -->
    <div class="shard-bar glass">
      <div class="shard">
        <span class="shard-icon">⚊</span>
        <div>
          <div class="shard-val">{{ shards?.yang_yao ?? '--' }}</div>
          <div class="shard-lbl">{{ t('mg.yang') }}</div>
        </div>
        <el-button
          size="small"
          :disabled="!shards || shards.yang_yao < 1 || converting === -1"
          :loading="converting === -1"
          @click="convert(-1)"
          >→{{ t('mg.yin') }}</el-button
        >
      </div>
      <div class="shard">
        <span class="shard-icon">⚋</span>
        <div>
          <div class="shard-val">{{ shards?.yin_yao ?? '--' }}</div>
          <div class="shard-lbl">{{ t('mg.yin') }}</div>
        </div>
        <el-button
          size="small"
          :disabled="!shards || shards.yin_yao < 1 || converting === 1"
          :loading="converting === 1"
          @click="convert(1)"
          >→{{ t('mg.yang') }}</el-button
        >
      </div>
      <div class="convert-tip text-muted">{{ t('mg.convertTip') }}</div>
    </div>

    <!-- 金字塔 -->
    <div v-for="lvl in levels" :key="lvl.type" class="level">
      <div class="level-title">
        {{ t('mg.levelMedals', { name: lvl.name }) }}
        <span class="level-count text-muted">{{ lvl.nodes.filter((n) => n.owned).length }}/{{ lvl.nodes.length }}</span>
      </div>
      <div class="node-grid">
        <div v-for="node in lvl.nodes" :key="node.medal_id" class="node" :class="{ owned: node.owned }">
          <div class="node-img-wrap">
            <img
              v-if="node.image_url"
              :src="node.image_url"
              :alt="node.name"
              class="node-img"
              :class="{ gray: !node.owned }"
            />
            <span v-else class="node-placeholder">☯</span>
            <span v-if="node.owned" class="owned-badge">✓</span>
          </div>
          <div class="node-name">{{ node.name }}</div>
          <div class="node-cost text-muted">{{ costText(node) }}</div>
          <el-button
            v-if="!node.owned"
            size="small"
            :type="canCompose(node) ? 'primary' : 'info'"
            :disabled="!!composing || !canCompose(node)"
            :loading="composing === node.medal_id"
            @click="compose(node)"
          >
            {{
              canCompose(node)
                ? isBigType(node.type)
                  ? t('mg.bigCompose')
                  : t('mg.compose')
                : isBigType(node.type)
                  ? t('mg.childrenMissing')
                  : t('mg.shardsMissing')
            }}
          </el-button>
          <div v-else class="owned-tag">{{ t('mg.owned') }}</div>
        </div>
      </div>
    </div>

    <div v-if="!loading && !tree.length" class="empty glass">
      <div v-if="fetchError" class="err">{{ fetchError }}</div>
      <div v-else class="text-muted">{{ t('mg.noData') }}</div>
    </div>
  </div>
</template>

<style scoped>
.lpt-medal {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 720px;
  margin: 0 auto;
}
.banner {
  text-align: center;
  padding: 14px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, #2d3436, #636e72);
}
.banner-title {
  display: block;
  font-size: var(--fs-title);
  font-weight: 800;
  color: #ffd76a;
  letter-spacing: 2px;
}
.banner-sub {
  display: block;
  margin-top: 3px;
  font-size: var(--fs-xs);
  color: rgba(255, 255, 255, 0.7);
}

.shard-bar {
  display: flex;
  gap: var(--sp-4);
  padding: 14px;
  border-radius: var(--radius-sm);
  align-items: center;
  flex-wrap: wrap;
}
.shard {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.shard-icon {
  font-size: var(--fs-hero);
  color: var(--pt-primary);
}
.shard-val {
  font-size: var(--fs-title);
  font-weight: 800;
}
.shard-lbl {
  font-size: var(--fs-2xs);
  color: var(--text-muted, #999);
}
.convert-tip {
  width: 100%;
  font-size: var(--fs-2xs);
  margin-top: 4px;
}

.level {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.level-title {
  font-weight: 700;
  font-size: var(--fs-lg);
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.level-count {
  font-size: var(--fs-xs);
  font-weight: 400;
}
.node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
  gap: var(--sp-2);
}

.node {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-1);
  padding: 12px 8px;
  border-radius: var(--radius-sm);
  background: var(--glass-bg, rgba(255, 255, 255, 0.06));
  border: 1px solid var(--border-soft, rgba(255, 255, 255, 0.08));
  text-align: center;
}
.node.owned {
  border-color: var(--pt-success-bg);
}
.node-img-wrap {
  position: relative;
  width: 56px;
  height: 56px;
}
.node-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.node-img.gray {
  filter: grayscale(1) opacity(0.5);
}
.node-placeholder {
  font-size: 40px;
  color: var(--text-muted, #999);
}
.owned-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: #2ecc71;
  color: #fff;
  font-size: var(--fs-xs);
  display: flex;
  align-items: center;
  justify-content: center;
}
.node-name {
  font-size: var(--fs-xs);
  font-weight: 600;
}
.node-cost {
  font-size: var(--fs-2xs);
  line-height: 1.3;
  min-height: 26px;
}
.owned-tag {
  font-size: var(--fs-xs);
  color: var(--pt-success);
  font-weight: 600;
  height: 24px;
  line-height: 24px;
}
.empty {
  padding: 40px;
  text-align: center;
}
.err {
  color: var(--pt-danger);
  font-size: var(--fs-body);
  word-break: break-all;
}
</style>
