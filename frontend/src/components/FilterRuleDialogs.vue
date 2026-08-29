<script setup lang="ts">
import type { CustomRuleInput, RuleGroupInput, FilterRule, FilterRuleGroup } from '@/api/filter'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const showCustomForm = defineModel<boolean>('showCustomForm', { required: true })
const editingCustom = defineModel<FilterRule | null>('editingCustom', { required: true })
const customForm = defineModel<CustomRuleInput>('customForm', { required: true })
const savingCustom = defineModel<boolean>('savingCustom', { required: true })
const showGroupForm = defineModel<boolean>('showGroupForm', { required: true })
const editingGroup = defineModel<FilterRuleGroup | null>('editingGroup', { required: true })
const groupForm = defineModel<RuleGroupInput>('groupForm', { required: true })
const savingGroup = defineModel<boolean>('savingGroup', { required: true })

const emit = defineEmits<{ (e: 'saveCustom'): void; (e: 'saveGroup'): void }>()
</script>

<template>
  <!-- 自定义规则表单 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="showCustomForm" :title="editingCustom ? t('frd.editCustom') : t('frd.newCustom')" width="560px">
    <el-form :model="customForm" label-width="110px" size="small">
      <el-form-item :label="t('frd.ruleId')" required>
        <el-input v-model="customForm.id" :placeholder="t('frd.idPh')" :disabled="!!editingCustom" />
      </el-form-item>
      <el-form-item :label="t('common.name')"><el-input v-model="customForm.name" :placeholder="t('frd.namePh')" /></el-form-item>
      <el-form-item :label="t('frd.include')">
        <el-input v-model="customForm.include" :placeholder="t('frd.includePh')" />
      </el-form-item>
      <el-form-item :label="t('frd.exclude')">
        <el-input v-model="customForm.exclude" :placeholder="t('frd.excludePh')" />
      </el-form-item>
      <el-form-item :label="t('frd.sizeRange')">
        <el-input v-model="customForm.size_range" placeholder="1000-5000 / >1000 / <5000" />
      </el-form-item>
      <el-form-item :label="t('frd.minSeeders')">
        <el-input-number v-model="customForm.seeders" :min="0" controls-position="right" />
      </el-form-item>
      <el-form-item :label="t('frd.publishMin')">
        <el-input v-model="customForm.publish_time" :placeholder="t('frd.publishPh')" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showCustomForm = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="savingCustom" @click="emit('saveCustom')">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>

  <!-- 规则组表单 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="showGroupForm" :title="editingGroup ? t('frd.editGroup') : t('frd.newGroup')" width="560px">
    <el-form :model="groupForm" label-width="110px" size="small">
      <el-form-item :label="t('frd.groupName')" required>
        <el-input v-model="groupForm.name" :placeholder="t('frd.groupPh')" :disabled="!!editingGroup" />
      </el-form-item>
      <el-form-item :label="t('frd.ruleExpr')" required>
        <el-input v-model="groupForm.rule_string" type="textarea" :rows="2" :placeholder="t('frd.exprPh')" />
      </el-form-item>
      <el-form-item :label="t('frd.mediaType')">
        <el-select v-model="groupForm.media_type" style="width: 100%">
          <el-option :label="t('frd.mtAny')" value="" />
          <el-option :label="t('frd.mtMovie')" value="\u7535\u5f71" />
          <el-option :label="t('frd.mtTv')" value="\u7535\u89c6\u5267" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('frd.category')"><el-input v-model="groupForm.category" :placeholder="t('frd.categoryPh')" /></el-form-item>
    </el-form>
    <p class="syntax">
      {{ t('frd.syntax') }}
    </p>
    <template #footer>
      <el-button @click="showGroupForm = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="savingGroup" @click="emit('saveGroup')">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.syntax { font-size: var(--fs-xs); color: var(--text-muted); line-height: 1.6; margin: 0; }
.syntax code { background: var(--surface-hover, rgba(127, 127, 127, 0.15)); padding: 1px 5px; border-radius: var(--radius-2xs); }
</style>
