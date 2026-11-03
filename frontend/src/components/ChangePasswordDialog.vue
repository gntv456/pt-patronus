<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { authApi } from '@/api/auth'

// 修改本地密码对话框（好学站点关闭后的本地密码主路径，#<issue>）。
// 顶栏用户菜单触发；校验旧密码，成功后提示（不改 token，会话不中断）。
const visible = defineModel<boolean>({ default: false })

const { t } = useI18n()
const oldPwd = ref('')
const newPwd = ref('')
const confirmPwd = ref('')
const saving = ref(false)

function reset() {
  oldPwd.value = ''
  newPwd.value = ''
  confirmPwd.value = ''
}

async function submit() {
  if (!oldPwd.value || !newPwd.value) {
    ElMessage.warning(t('pwdDialog.err.fill'))
    return
  }
  if (newPwd.value !== confirmPwd.value) {
    ElMessage.warning(t('pwdDialog.err.mismatch'))
    return
  }
  if (!/[a-zA-Z]/.test(newPwd.value) || !/\d/.test(newPwd.value) || newPwd.value.length < 8) {
    ElMessage.warning(t('login.localForm.passwordRule'))
    return
  }
  saving.value = true
  try {
    await authApi.changePassword(oldPwd.value, newPwd.value)
    ElMessage.success(t('pwdDialog.ok'))
    visible.value = false
    reset()
  } catch (e: unknown) {
    const r = (e as { response?: { data?: { message?: string; error?: string } } })?.response?.data
    ElMessage.error(r?.message || r?.error || t('pwdDialog.err.failed'))
  } finally {
    saving.value = false
  }
}

function onClose() {
  reset()
}
</script>

<template>
  <el-dialog v-model="visible" :title="t('pwdDialog.title')" width="380px" @closed="onClose">
    <el-form label-position="top" @submit.prevent="submit">
      <el-form-item :label="t('pwdDialog.old')">
        <el-input v-model="oldPwd" type="password" show-password autocomplete="current-password" />
      </el-form-item>
      <el-form-item :label="t('pwdDialog.new')">
        <el-input v-model="newPwd" type="password" show-password autocomplete="new-password" />
      </el-form-item>
      <el-form-item :label="t('pwdDialog.confirm')">
        <el-input v-model="confirmPwd" type="password" show-password autocomplete="new-password" />
      </el-form-item>
      <p class="rule-hint">{{ t('login.localForm.passwordRule') }}</p>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">{{ t('pwdDialog.cancel') }}</el-button>
      <el-button type="primary" :loading="saving" @click="submit">{{ t('pwdDialog.submit') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.rule-hint {
  margin: 0 0 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
