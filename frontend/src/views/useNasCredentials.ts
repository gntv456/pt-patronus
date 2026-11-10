// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NasServer 凭据中心（OpsCredential CRUD）—— 从 NasServer.vue 抽出的状态簇。
// 依赖注入：targets（凭据使用计数）/ reload（保存后刷新整体，即组件 loadAll）。
import { ref, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { credentialApi, type MonitorTarget, type OpsCredential, type OpsCredentialInput } from '@/api/monitor'

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
      ElMessage.warning('请填写凭据名称')
      return
    }
    try {
      if (editingCredential.value) {
        await credentialApi.update(editingCredential.value.id, credentialForm.value)
        ElMessage.success('凭据已更新')
      } else {
        await credentialApi.create(credentialForm.value)
        ElMessage.success('凭据已创建')
      }
      credentialFormVisible.value = false
      credentials.value = await credentialApi.list()
      await reload()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '保存失败')
    }
  }

  async function removeCredential(c: OpsCredential) {
    try {
      await ElMessageBox.confirm(`确定删除凭据「${c.name}」？`, '删除凭据', { type: 'warning' })
    } catch {
      return
    }
    try {
      await credentialApi.remove(c.id)
      ElMessage.success('凭据已删除')
      credentials.value = await credentialApi.list()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '删除失败')
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
