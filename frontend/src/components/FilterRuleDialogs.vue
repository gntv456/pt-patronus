<script setup lang="ts">
import type { CustomRuleInput, RuleGroupInput, FilterRule, FilterRuleGroup } from '@/api/filter'

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
  <el-dialog class="pt-dialog" append-to-body align-center v-model="showCustomForm" :title="editingCustom ? '编辑自定义规则' : '新建自定义规则'" width="560px">
    <el-form :model="customForm" label-width="110px" size="small">
      <el-form-item label="规则 ID" required>
        <el-input v-model="customForm.id" placeholder="仅字母数字，如 CN_AUDIO" :disabled="!!editingCustom" />
      </el-form-item>
      <el-form-item label="名称"><el-input v-model="customForm.name" placeholder="如：中文音轨" /></el-form-item>
      <el-form-item label="包含 (include)">
        <el-input v-model="customForm.include" placeholder="正则，命中标题/副标题/标签即保留，如 国语|普通话" />
      </el-form-item>
      <el-form-item label="排除 (exclude)">
        <el-input v-model="customForm.exclude" placeholder="正则，命中即拒绝，如 原盘|REMUX" />
      </el-form-item>
      <el-form-item label="大小范围(MB)">
        <el-input v-model="customForm.size_range" placeholder="1000-5000 / >1000 / <5000" />
      </el-form-item>
      <el-form-item label="最少做种">
        <el-input-number v-model="customForm.seeders" :min="0" controls-position="right" />
      </el-form-item>
      <el-form-item label="发布时间(分)">
        <el-input v-model="customForm.publish_time" placeholder="60 / 60-1440（种子发布距今分钟数）" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showCustomForm = false">取消</el-button>
      <el-button type="primary" :loading="savingCustom" @click="emit('saveCustom')">保存</el-button>
    </template>
  </el-dialog>

  <!-- 规则组表单 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="showGroupForm" :title="editingGroup ? '编辑规则组' : '新建规则组'" width="560px">
    <el-form :model="groupForm" label-width="110px" size="small">
      <el-form-item label="组名" required>
        <el-input v-model="groupForm.name" placeholder="如：默认规则组" :disabled="!!editingGroup" />
      </el-form-item>
      <el-form-item label="规则表达式" required>
        <el-input v-model="groupForm.rule_string" type="textarea" :rows="2" placeholder="如：4K & !BLU > 1080P" />
      </el-form-item>
      <el-form-item label="媒体类型">
        <el-select v-model="groupForm.media_type" style="width: 100%">
          <el-option label="通用（不限）" value="" />
          <el-option label="电影" value="电影" />
          <el-option label="电视剧" value="电视剧" />
        </el-select>
      </el-form-item>
      <el-form-item label="分类"><el-input v-model="groupForm.category" placeholder="留空=不限，如 动画" /></el-form-item>
    </el-form>
    <p class="syntax">
      语法：<code>&gt;</code> 分优先级层（左优先级最高）；<code>&amp;</code> 与、<code>|</code> 或、<code>!</code> 非、<code>()</code> 分组；操作数为规则 id。
    </p>
    <template #footer>
      <el-button @click="showGroupForm = false">取消</el-button>
      <el-button type="primary" :loading="savingGroup" @click="emit('saveGroup')">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.syntax { font-size: 12px; color: var(--text-muted); line-height: 1.6; margin: 0; }
.syntax code { background: var(--surface-hover, rgba(127, 127, 127, 0.15)); padding: 1px 5px; border-radius: 4px; }
</style>
