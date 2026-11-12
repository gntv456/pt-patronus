// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NasServer 凭据中心（OpsCredential CRUD）—— 从 NasServer.vue 抽出的状态簇。
// 依赖注入：targets（凭据使用计数）/ reload（保存后刷新整体，即组件 loadAll）。
import { ref, type Ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { credentialApi, type MonitorTarget, type OpsCredential, type OpsCredentialInput } from '@/api/monitor'


const t = (k: string) => i18n.global.t(k)

export function useNasCredentials(targets: Ref<MonitorTarget[]>, reload: () => Promise<void>) {
  const credentials = ref<OpsCredential[]>([])
  const credentialVisible = ref(false)
  const credentialFormVisible = ref(false)
  const editingCredential = ref<OpsCredential | null>(null)
  const credentialForm = ref<OpsCredentialInput>({
    name: '',
    type: 'ssh',
    username: '',
    password: '',
    private_key: '',
    tags: '',
    note: '',
  })

  function blankCredential(): OpsCredentialInput {
    return { name: '', type: 'ssh', username: '', password: '', private_key: '', tags: '', note: '' }
  }

  function openCredentialCenter() {
    credentialVisible.value = true
  }

  function openCredentialAdd() {
    editingCredential.value = null
    credentialForm.value = blankCredential()
    credentialFormVisible.value = true
  }

  function openCredentialEdit(c: OpsCredential) {
    editingCredential.value = c
    credentialForm.value = {
      name: c.name,
      type: c.type || 'ssh',
      username: c.username || '',
      password: '',
      private_key: '',
      tags: c.tags || '',
      note: c.note || '',
    }
    credentialFormVisible.value = true
  }

  function credentialUsage(id: number) {
    return targets.value.filter((t) => t.credential_id === id).length
  }

  async function saveCredential() {
    if (!credentialForm.value.name?.trim()) {
      ElMessage.warning(t('opusenascredentials.k1'))
      return
    }
    try {
      if (editingCredential.value) {
        await credentialApi.update(editingCredential.value.id, credentialForm.value)
        ElMessage.success(t('opusenascredentials.k2'))
      } else {
        await credentialApi.create(credentialForm.value)
        ElMessage.success(t('opusenascredentials.k3'))
      }
      credentialFormVisible.value = false
      credentials.value = await credentialApi.list()
      await reload()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.saveFailed'))
    }
  }

  async function removeCredential(c: OpsCredential) {
    try {
      await ElMessageBox.confirm(`确定删除凭据「${c.name}」？`, t('opusenascredentials.k4'), { type: 'warning' })
    } catch {
      return
    }
    try {
      await credentialApi.remove(c.id)
      ElMessage.success(t('opusenascredentials.k5'))
      credentials.value = await credentialApi.list()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.deleteFailed'))
    }
  }

  return {
    credentials,
    credentialVisible,
    credentialFormVisible,
    editingCredential,
    credentialForm,
    openCredentialCenter,
    openCredentialAdd,
    openCredentialEdit,
    credentialUsage,
    saveCredential,
    removeCredential,
  }
}
