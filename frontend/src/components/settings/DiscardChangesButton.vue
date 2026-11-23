<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->
<script setup lang="ts">
// 配置面板的「放弃修改」出口：与 useUnsavedGuard 的 beforeunload 守护互补。
// 守护防止误丢；本按钮给主动反悔的用户一个明确、安全的一键恢复路径，
// 而不是逼用户逐字段改回或强制刷新（刷新还会被守护拦截）。
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { useUnsavedGuard } from '@/composables/useUnsavedGuard'

const props = defineProps<{
  // 面板已有的 loadXxx()：重新从服务端拉取权威配置回填表单。
  reload: () => Promise<unknown>
  // 可选：指定要检查的表单；不传则看全局（任一表单脏即显示）。
  guardKey?: symbol
}>()

const { t } = useI18n()
const { isDirty } = useUnsavedGuard()

async function discard() {
  try {
    await ElMessageBox.confirm(
      t('common.discardConfirm'),
      t('common.discardTitle'),
      {
        type: 'warning',
        confirmButtonText: t('common.discard'),
        cancelButtonText: t('common.cancel'),
      },
    )
  } catch {
    return // 用户取消
  }
  await props.reload()
}
</script>

<template>
  <el-button v-if="isDirty(guardKey)" text @click="discard">
    {{ t('common.discard') }}
  </el-button>
</template>
