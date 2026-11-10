<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import type { OpsCredential, OpsCredentialInput } from '@/api/monitor'
import { useI18n } from 'vue-i18n'
import { credentialTypeText } from '@/views/nasServerHelpers'

// form 用 defineModel 双向绑定（父级 v-model:form）：表单控件改字段经 update:form 回传，
// 消除对 prop 的直接突变。credentialUsage 是父级传入的计算函数（依赖 targets）。
const { t } = useI18n()

const form = defineModel<OpsCredentialInput>('form', { required: true })

defineProps<{
  drawerVisible: boolean
  credentials: OpsCredential[]
  formVisible: boolean
  editing: OpsCredential | null
  credentialUsage: (id: number) => number
}>()

const emit = defineEmits<{
  (e: 'update:drawerVisible', v: boolean): void
  (e: 'update:formVisible', v: boolean): void
  (e: 'add'): void
  (e: 'edit', c: OpsCredential): void
  (e: 'remove', c: OpsCredential): void
  (e: 'save'): void
}>()
</script>

<template>
  <el-drawer
    :model-value="drawerVisible"
    :title="t('ncd.title')"
    size="520px"
    @update:model-value="emit('update:drawerVisible', $event)"
  >
    <div class="cred-toolbar">
      <span>{{ t('ncd.subtitle') }}</span>
      <el-button type="primary" size="small" @click="emit('add')">{{ t('ncd.add') }}</el-button>
    </div>
    <div v-if="credentials.length" class="cred-list">
      <div v-for="c in credentials" :key="c.id" class="cred-card">
        <div class="cred-main">
          <div>
            <b>{{ c.name }}</b>
            <span
              >{{ credentialTypeText(c.type) }} ·
              {{ c.username || (c.has_username ? t('ncd.userUnreadable') : t('ncd.noUser')) }}</span
            >
          </div>
          <el-tag size="small" type="info">{{ t('ncd.devices', { n: credentialUsage(c.id) }) }}</el-tag>
        </div>
        <div class="cred-flags">
          <span v-if="c.has_password">{{ t('ncd.password') }}</span>
          <span v-if="c.has_private_key">{{ t('ncd.privateKey') }}</span>
          <span v-if="c.tags">{{ c.tags }}</span>
        </div>
        <p v-if="c.note">{{ c.note }}</p>
        <div class="cred-actions">
          <el-button size="small" @click="emit('edit', c)">{{ t('common.edit') }}</el-button>
          <el-button size="small" type="danger" plain @click="emit('remove', c)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
    </div>
    <div v-else class="cred-empty">
      {{ t('ncd.empty') }}
      <el-button type="primary" size="small" @click="emit('add')">{{ t('ncd.addFirst') }}</el-button>
    </div>
  </el-drawer>

  <el-dialog
    :model-value="formVisible"
    :title="editing ? t('ncd.editTitle') : t('ncd.add')"
    width="520px"
    destroy-on-close
    @update:model-value="emit('update:formVisible', $event)"
  >
    <el-form label-width="82px">
      <el-form-item :label="t('common.name')" required>
        <el-input v-model="form.name" :placeholder="t('ncd.namePh')" />
      </el-form-item>
      <el-form-item :label="t('ncd.type')">
        <el-select v-model="form.type" style="width: 100%">
          <el-option label="SSH" value="ssh" />
          <el-option label="NAS" value="nas" />
          <el-option :label="t('ncd.generic')" value="generic" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('ncd.username')">
        <el-input v-model="form.username" placeholder="root / admin" />
      </el-form-item>
      <el-form-item :label="t('ncd.password')">
        <el-input
          v-model="form.password"
          type="password"
          show-password
          :placeholder="editing ? t('ncd.keepIfEmpty') : t('ncd.secretPh')"
        />
      </el-form-item>
      <el-form-item :label="t('ncd.privateKey')">
        <el-input
          v-model="form.private_key"
          type="textarea"
          :rows="6"
          resize="vertical"
          :placeholder="editing?.has_private_key ? t('ncd.keySetKeep') : t('ncd.keyPh')"
        />
      </el-form-item>
      <el-form-item :label="t('ncd.tags')">
        <el-input v-model="form.tags" :placeholder="t('ncd.tagsPh')" />
      </el-form-item>
      <el-form-item :label="t('ncd.note')">
        <el-input v-model="form.note" type="textarea" :rows="3" resize="vertical" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:formVisible', false)">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" @click="emit('save')">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped src="../../views/nas-shared/credential.css"></style>
