<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { DefinitionDiagnostic } from '@/api/site'
import { diagKindKey, diagMoreCount, diagTarget } from '@/utils/siteDefinitionDiag'

// 站点定义诊断条：后端四类「配置被静默忽略」的账面（正则编不出来 / 不认识的键 /
// 站点 id 撞车 / 子目录没读）。以前这四条只有日志，用户在站点页看到的只是「这个站抓不到东西」。
const props = defineProps<{ diags: DefinitionDiagnostic[]; failed: boolean; errText: string }>()
const { t } = useI18n()
const MAX_SHOWN = 6
</script>

<template>
  <el-alert v-if="props.diags.length || props.failed" class="def-diag" type="warning" show-icon :closable="false">
    <template #title>
      <!-- 读失败与「真的没有诊断」必须不同形：后者是好消息，前者什么也没说。 -->
      <span v-if="props.diags.length">{{ t('sites.diagTitle', { n: props.diags.length }) }}</span>
      <span v-else>{{ t('sites.diagLoadFailed', { err: props.errText }) }}</span>
    </template>
    <ul v-if="props.diags.length" class="def-diag-list">
      <li v-for="(d, i) in props.diags.slice(0, MAX_SHOWN)" :key="i">
        {{ t(diagKindKey(d.kind)) }} · {{ diagTarget(d) || '—' }}：{{ d.detail }}
      </li>
    </ul>
    <span v-if="diagMoreCount(props.diags.length, MAX_SHOWN)" class="def-diag-more">
      {{ t('sites.diagMore', { n: diagMoreCount(props.diags.length, MAX_SHOWN) }) }}
    </span>
  </el-alert>
</template>

<style scoped>
.def-diag-list {
  margin: 4px 0 0;
  padding-left: 18px;
}
</style>
