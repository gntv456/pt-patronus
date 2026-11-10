<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { authApi } from '@/api/auth'

// 修改本地密码对话框（本地密码主路径）。顶栏用户菜单触发；校验旧密码，成功后提示
// （不改 token，会话不中断）。
// setupMode：升级后的存量好学账号没有本地密码，此时隐藏「当前密码」、改走首次设密，
// 让用户在网页上点一下完成迁移，不用在终端跑 CLI / 改 compose。
const visible = defineModel<boolean>({ default: false })
const props = withDefaults(defineProps<{ setupMode?: boolean }>(), { setupMode: false })
const emit = defineEmits<{ setupDone: [] }>()

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
  if (!props.setupMode && !oldPwd.value) {
    ElMessage.warning(t('pwdDialog.err.fill'))
    return
  }
  if (!newPwd.value) {
    ElMessage.warning(t('pwdDialog.err.fillNew'))
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
    await authApi.changePassword(props.setupMode ? '' : oldPwd.value, newPwd.value)
    ElMessage.success(t(props.setupMode ? 'pwdDialog.setupOk' : 'pwdDialog.ok'))
    visible.value = false
    reset()
    if (props.setupMode) emit('setupDone')
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
  <el-dialog
    v-model="visible"
    :title="t(setupMode ? 'pwdDialog.setupTitle' : 'pwdDialog.title')"
    width="380px"
    :close-on-click-modal="!setupMode"
    @closed="onClose"
  >
    <el-form label-position="top" @submit.prevent="submit">
      <el-form-item v-if="!setupMode" :label="t('pwdDialog.old')">
        <el-input v-model="oldPwd" type="password" show-password autocomplete="current-password" />
      </el-form-item>
      <p v-else class="rule-hint">{{ t('pwdDialog.setupHint') }}</p>
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
