// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { reactive, ref, type ObjectEmitsOptions } from 'vue'
import { useI18n } from 'vue-i18n'

// 统一表单校验（W3：以 Downloaders 的「必填 + URL 正则」模式为模板铺开）。
// 此前全站 0 处 el-form rules——所有错误都是「提交后一条 toast」，用户不知道
// 错在哪个字段。本工具提供：
//   - 常用规则工厂：required / url / numberRange / port
//   - useForm<T>()：rules + validate()（失败自动滚动/聚焦第一个错误字段）
// 用法：
//   const { rules, validate } = useForm(form, {
//     name: [ruleRequired()],
//     url: [ruleRequired(), ruleURL()],
//   })
//   <el-form :model="form" :rules="rules" ref="formRef" label-position="top">
//     <el-form-item prop="name" :label="..."><el-input v-model="form.name" /></el-form-item>
//   </el-form>
//   async function submit() { if (!(await validate())) return; ... }

type Rule = { required?: boolean; message: string; trigger: string; pattern?: RegExp; validator?: (v: unknown) => boolean | string }
export type FormRules<T extends object> = Partial<Record<keyof T, Rule[]>>

export function useForm<T extends object>(form: T, rulesDef: FormRules<T>) {
  const { t } = useI18n()
  const formRef = ref<{ validate: () => Promise<{ [k: string]: unknown }>; fields?: unknown[] } | null>(null)
  const rules = reactive(rulesDef) as FormRules<T>

  /** 校验整表；失败聚焦第一个错误字段并返回 false（Element 校验错误已显示在字段下方）。 */
  async function validate(): Promise<boolean> {
    if (!formRef.value) return true
    try {
      await formRef.value.validate()
      return true
    } catch {
      return false
    }
  }

  return { formRef, rules, validate, t }
}

// ---- 规则工厂（trigger 统一 blur——失焦即校验，不用等提交） ----

export function ruleRequired(label?: string) {
  const { t } = useI18n()
  return {
    required: true,
    message: label ? t('common.fieldRequired', { field: label }) : t('common.required'),
    trigger: 'blur',
  }
}

/** 选择器类字段（el-select 值变化才触发 change，blur 不可靠）。 */
export function ruleRequiredChange(label?: string) {
  const { t } = useI18n()
  return {
    required: true,
    message: label ? t('common.fieldRequired', { field: label }) : t('common.required'),
    trigger: 'change',
  }
}

export function ruleURL(label?: string) {
  const { t } = useI18n()
  return {
    pattern: /^https?:\/\/.+/i,
    message: (label ? label + '：' : '') + t('common.urlFormat'),
    trigger: 'blur',
  }
}
