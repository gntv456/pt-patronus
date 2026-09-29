<script setup lang="ts">
// 表单对象经 props 传入并双向编辑(宿主 ref 直改深层字段),整文件豁免。
/* eslint-disable vue/no-mutating-props */
// BrushFlow 的任务表单对话框（从 BrushFlow.vue 拆出）。
// W3：①必填字段接入 el-form rules（字段下方红字 + 失焦即校验，替代提交后一条 toast）；
//    ②删除规则/并发路径折叠进 el-collapse——新手 80% 只需填前几个字段，30+ 字段
//    平铺 3-4 屏会吓退用户；折叠标题实时摘要当前删除策略。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ruleRequired, ruleRequiredChange } from '@/composables/useFormRules'

defineModel<boolean>('visible', { default: false })

const props = defineProps<{
  form: Record<string, any>
  editing: boolean
  sites: { id: number; name: string }[]
  downloaders: { id: number; name: string }[]
  filterGroups: Record<string, any>
  filterGroupArray: any[]
}>()

const emit = defineEmits<{
  'update:visible': [v: boolean]
  save: []
  'update:filterGroupArray': [v: unknown]
}>()

const { t } = useI18n()

// ---- 校验（字段级红字提示）----
const formRef = ref()
const rules = {
  name: [ruleRequired(t('rss.f.name'))],
  site_id: [ruleRequiredChange(t('contract.f.site'))],
  downloader_id: [ruleRequiredChange(t('downloaders.title'))],
}

/** 删除策略摘要：折叠标题上直接显示已配置的维度，用户不用展开也能确认。 */
const delSummary = computed(() => {
  const f = props.form
  const parts: string[] = []
  if (f.delete_ratio > 0) parts.push(t('brushflow.f.delRatio') + '≥' + f.delete_ratio)
  if (f.delete_seed_hours > 0) parts.push(t('brushflow.f.delSeedHours') + '≥' + f.delete_seed_hours + 'h')
  if (f.delete_upload_mb > 0) parts.push(t('brushflow.f.delUpload') + '≥' + f.delete_upload_mb + 'MB')
  if (f.delete_rule_expr) parts.push(t('brushflow.f.expr'))
  if (!parts.length) return t('brushflow.f.noDelRule')
  return parts.join(f.delete_logic === 'and' ? ' 且 ' : ' 或 ')
})

async function onSave() {
  if (!formRef.value) return emit('save')
  try {
    await formRef.value.validate()
    emit('save')
  } catch {
    /* 字段错误已显示在对应 el-form-item 下方 */
  }
}

const advancedTabs = ref<string[]>([])
</script>

<template>
    <el-dialog
      :model-value="visible" @update:model-value="emit('update:visible', $event)"
      :title="editing ? t('brushflow.editTitle') : t('brushflow.newTitle')"
      width="640px"
      top="6vh"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" label-position="right">
        <el-form-item :label="t('rss.f.name')" prop="name"
          ><el-input v-model="form.name" :placeholder="t('brushflow.f.nameHint')"
        /></el-form-item>
        <el-form-item :label="t('contract.f.site')" prop="site_id">
          <el-select v-model="form.site_id">
            <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('downloaders.title')" prop="downloader_id">
          <el-select v-model="form.downloader_id">
            <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('rss.f.enabled')"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item :label="t('brushflow.f.keyword')"
          ><el-input v-model="form.keyword" :placeholder="t('brushflow.f.kwHint')"
        /></el-form-item>
        <el-form-item :label="t('rss.f.category')">
          <el-select v-model="form.category" clearable>
            <el-option :label="t('brushflow.cat.all')" value="" />
            <el-option :label="t('search.catMovie')" value="movie" />
            <el-option :label="t('search.catTv')" value="tv" />
            <el-option :label="t('search.catMusic')" value="music" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.sizeMB')">
          <div class="range">
            <el-input-number v-model="form.min_size_mb" :min="0" controls-position="right" />
            <span>~</span>
            <el-input-number v-model="form.max_size_mb" :min="0" controls-position="right" />
            <span class="hint">{{ t('brushflow.f.zeroAny') }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.seeders')">
          <div class="range">
            <el-input-number v-model="form.min_seeders" :min="0" controls-position="right" />
            <span>~</span>
            <el-input-number v-model="form.max_seeders" :min="0" controls-position="right" />
          </div>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.freeOnly')"><el-switch v-model="form.free_only" /></el-form-item>
        <el-form-item :label="t('rss.f.filterGroups')">
          <el-select
            :model-value="filterGroupArray" @update:model-value="emit('update:filterGroupArray', $event)"
            multiple
            collapse-tags
            collapse-tags-tooltip
            :placeholder="t('rss.f.filterGroupsHint')"
            style="width: 100%"
          >
            <el-option v-for="g in filterGroups" :key="g" :label="g" :value="g" />
          </el-select>
        </el-form-item>
        <!-- 高级字段折叠（W3）：删除规则 11 个字段 + 并发 3 个字段对新手是噪音，
             默认收起；折叠标题实时摘要删除策略，编辑老任务时不用展开也能核对。 -->
        <el-collapse v-model="advancedTabs" class="bf-advanced">
          <el-collapse-item name="del">
            <template #title>
              <span class="bf-adv-title">{{ t('brushflow.f.delRules') }}</span>
              <span class="bf-adv-summary">{{ delSummary }}</span>
            </template>
            <el-divider content-position="left">{{ t('brushflow.f.delRules') }}</el-divider>
        <el-form-item :label="t('brushflow.f.logic')">
          <el-radio-group v-model="form.delete_logic">
            <el-radio-button label="or">{{ t('brushflow.f.logicOr') }}</el-radio-button>
            <el-radio-button label="and">{{ t('brushflow.f.logicAnd') }}</el-radio-button>
          </el-radio-group>
          <span class="hint">{{ t('brushflow.f.exprWins') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.delRatio')"
          ><el-input-number v-model="form.delete_ratio" :min="0" :step="0.1" controls-position="right"
        /></el-form-item>
        <el-form-item :label="t('brushflow.f.delSeedHours')"
          ><el-input-number v-model="form.delete_seed_hours" :min="0" :step="0.5" controls-position="right"
        /></el-form-item>
        <el-form-item :label="t('brushflow.f.delUpload')"
          ><el-input-number v-model="form.delete_upload_mb" :min="0" controls-position="right"
        /></el-form-item>
        <el-form-item :label="t('brushflow.f.delMinRate')">
          <el-input-number v-model="form.delete_min_up_rate_kb" :min="0" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.delMinRateHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.delInactive')">
          <el-input-number v-model="form.delete_inactive_hours" :min="0" :step="0.5" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.delInactiveHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.expr')">
          <el-input
            v-model="form.delete_rule_expr"
            type="textarea"
            :rows="2"
            :placeholder="t('brushflow.f.exprPlaceholder')"
          />
          <span class="hint">{{ t('brushflow.f.exprHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.maxTotalSize')">
          <el-input-number v-model="form.max_total_size_gb" :min="0" :step="1" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.maxTotalSizeHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.maxSwarmSeeders')">
          <el-input-number v-model="form.delete_max_swarm_seeders" :min="0" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.maxSwarmSeedersHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.minSwarmLeechers')">
          <el-input-number v-model="form.delete_min_swarm_leechers" :min="0" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.minSwarmLeechersHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.delFiles')"><el-switch v-model="form.delete_files" /></el-form-item>
        <el-form-item :label="t('brushflow.f.nativeStop')">
          <el-switch v-model="form.use_share_limit" />
          <span class="hint">{{ t('brushflow.f.nativeStopHint') }}</span>
        </el-form-item>
          </el-collapse-item>
          <el-collapse-item name="conc">
            <template #title>
              <span class="bf-adv-title">{{ t('brushflow.f.concurrency') }}</span>
            </template>
            <el-divider content-position="left">{{ t('brushflow.f.concurrency') }}</el-divider>
        <el-form-item :label="t('brushflow.f.maxSeeds')"
          ><el-input-number v-model="form.max_seeds" :min="1" controls-position="right"
        /></el-form-item>
        <el-form-item :label="t('rss.f.savePath')"
          ><el-input v-model="form.save_path" :placeholder="t('brushflow.f.savePathHint')"
        /></el-form-item>
        <el-form-item :label="t('brushflow.f.dlCategory')"><el-input v-model="form.category_dl" /></el-form-item>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="emit('update:visible', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="onSave">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
</template>

<style scoped>
.bf-adv-title {
  font-weight: 600;
  margin-right: var(--sp-2);
}
.bf-adv-summary {
  color: var(--text-tertiary);
  font-size: var(--fs-xs);
  /* 摘要过长截断，避免折叠标题撑爆弹窗 */
  max-width: 360px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bf-advanced :deep(.el-collapse-item__wrap) {
  /* 弹窗内折叠面板去默认边框，视觉与表单融为一体 */
  border-bottom: none;
}
</style>
