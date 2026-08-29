<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { backupApi } from '@/api/backup'
import { useAuthStore } from '@/stores/auth'
import type { UploadFile } from 'element-plus'

const { t } = useI18n()
const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const auth = useAuthStore()
const importing = ref(false)
const exporting = ref(false)

// 加密备份信封检测（与后端 backup.NeedsPassword 同字段）。
function isEncryptedBackup(text: string): boolean {
  try {
    const obj = JSON.parse(text)
    return !!obj && typeof obj === 'object' && 'ptpatronus_backup_enc' in obj
  } catch {
    return false
  }
}

async function doExport() {
  // 导出含明文 Cookie / API Key / 密码，需二次确认
  try {
    await ElMessageBox.confirm(t('backup.exportConfirmMsg'), t('backup.exportConfirmTitle'), {
      type: 'warning',
      confirmButtonText: t('backup.exportAction'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return // 用户取消
  }
  // 可选口令加密：填了口令 → 服务端 PBKDF2+AES-GCM 密封（备份文件落盘即密文）；
  // 留空 → 旧行为明文 JSON（提示用户妥善保管）。
  let password = ''
  try {
    const { value } = await ElMessageBox.prompt(t('backup.exportPasswordHint'), t('backup.exportPasswordTitle'), {
      inputPlaceholder: t('backup.exportPasswordPlaceholder'),
      inputValidator: () => true,
    })
    password = (value || '').trim()
  } catch {
    return // 取消导出
  }
  exporting.value = true
  const token = auth.token
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` }
  if (password) headers['X-Backup-Password'] = password
  fetch(backupApi.exportUrl, { headers })
    .then((r) => r.blob())
    .then((b) => {
      const url = URL.createObjectURL(b)
      const a = document.createElement('a')
      a.href = url
      a.download = 'ptpatronus-backup.json'
      a.click()
      URL.revokeObjectURL(url)
      ElMessage.success(password ? t('backup.exportedEncrypted') : t('backup.exported'))
    })
    .catch(() => ElMessage.error(t('backup.exportFailed')))
    .finally(() => {
      exporting.value = false
    })
}

function onFile(file: File | undefined, kind: 'own' | 'moviepilot') {
  if (!file) return
  if (kind !== 'own') return onPlainFile(file, kind)
  importing.value = true
  const reader = new FileReader()
  reader.onload = async () => {
    const text = String(reader.result)
    try {
      let password = ''
      if (isEncryptedBackup(text)) {
        try {
          const { value } = await ElMessageBox.prompt(
            t('backup.importPasswordPrompt'),
            t('backup.exportPasswordTitle'),
            {
              inputType: 'password',
              confirmButtonText: t('common.confirm'),
              cancelButtonText: t('common.cancel'),
            },
          )
          password = value || ''
        } catch {
          importing.value = false
          return
        }
      }
      const data = JSON.parse(text)
      const res = await backupApi.importOwn(data, password)
      ElMessage.success(
        t('backup.importDone', {
          sites: res.imported_sites ?? 0,
          dlPart: res.imported_downloaders != null ? ' ' + t('backup.dlPart', { n: res.imported_downloaders }) : '',
        }),
      )
    } catch (err) {
      const code = (err as { code?: string })?.code
      if (code === 'backup_password_incorrect') ElMessage.error(t('backup.passwordIncorrect'))
      else ElMessage.error(t('backup.importFailed') + (err as Error).message)
    } finally {
      importing.value = false
    }
  }
  reader.readAsText(file)
}

// MoviePilot 明文导入（不支持加密信封，保持原路径）。
function onPlainFile(file: File, kind: 'own' | 'moviepilot') {
  importing.value = true
  const reader = new FileReader()
  reader.onload = async () => {
    try {
      const data = JSON.parse(String(reader.result))
      const fn = backupApi.importMoviePilot
      const res = await fn(data)
      ElMessage.success(
        t('backup.importDone', {
          sites: res.imported_sites ?? 0,
          dlPart: res.imported_downloaders != null ? ' ' + t('backup.dlPart', { n: res.imported_downloaders }) : '',
        }),
      )
    } catch (err) {
      ElMessage.error(t('backup.importFailed') + (err as Error).message)
    } finally {
      importing.value = false
    }
  }
  reader.readAsText(file)
}

// .ptd 是二进制 ZIP，直接以 FormData 上传（不走 readAsText/JSON.parse）。
function onPTDFile(file: File | undefined) {
  if (!file) return
  importing.value = true
  backupApi
    .importPTD(file)
    .then((res) => ElMessage.success(t('backup.importDoneSimple', { sites: res.imported_sites ?? 0 })))
    .catch((err) => ElMessage.error(t('backup.importFailed') + (err as Error).message))
    .finally(() => {
      importing.value = false
    })
}

function onOwnUpload(f: UploadFile) {
  onFile(f.raw, 'own')
}

function onMoviePilotUpload(f: UploadFile) {
  onFile(f.raw, 'moviepilot')
}

function onPTDUpload(f: UploadFile) {
  onPTDFile(f.raw)
}
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <h2 v-if="!props.embedded" class="title">{{ t('backup.title') }}</h2>
    <h3 v-else class="title embedded-title">{{ t('backup.title') }}</h3>

    <div class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">{{ t('backup.exportTitle') }}</div>
          <div class="bdesc text-muted">{{ t('backup.exportDesc') }}</div>
        </div>
        <el-button type="primary" :loading="exporting" :disabled="exporting" @click="doExport">{{
          t('backup.exportAction')
        }}</el-button>
      </div>
    </div>

    <div v-if="auth.isAdmin" class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">{{ t('backup.importOwnTitle') }}</div>
          <div class="bdesc text-muted">{{ t('backup.importOwnDesc') }}</div>
        </div>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".json" :on-change="onOwnUpload">
          <el-button :loading="importing">{{ t('backup.chooseFile') }}</el-button>
        </el-upload>
      </div>
    </div>

    <div v-if="auth.isAdmin" class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">{{ t('backup.importMPTitle') }}</div>
          <div class="bdesc text-muted">{{ t('backup.importMPDesc') }}</div>
        </div>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".json" :on-change="onMoviePilotUpload">
          <el-button :loading="importing">{{ t('backup.chooseFile') }}</el-button>
        </el-upload>
      </div>
    </div>

    <div v-if="auth.isAdmin" class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">{{ t('backup.importPTDTitle') }}</div>
          <div class="bdesc text-muted">{{ t('backup.importPTDDesc') }}</div>
        </div>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".ptd" :on-change="onPTDUpload">
          <el-button :loading="importing">{{ t('backup.chooseFile') }}</el-button>
        </el-upload>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 760px;
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.card {
  padding: 20px;
}
.brow {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-4);
}
.btitle {
  font-weight: 600;
  font-size: var(--fs-lg);
}
.bdesc {
  margin-top: 4px;
  font-size: var(--fs-body);
}

/* embedded：嵌进设置页时去掉外页边距，标题交给分区 Tab */
.page.embedded {
  padding: 0;
  max-width: none;
}
.page.embedded .embedded-title {
  font-size: var(--fs-lg);
  font-weight: 600;
  margin: 0;
}
.page.embedded .embedded-sub {
  margin-top: 0;
  margin-bottom: 10px;
  font-size: var(--fs-xs);
}
.page.embedded > .sub {
  display: none;
}
</style>
