<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { backupApi, importFailureNotice, importOutcomeKind } from '@/api/backup'
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
    .then(async (r) => {
      // 导出走 fetch（要触发浏览器下载），必须显式检查状态：失败时响应体是
      // errcode JSON 信封，若不拦截会被当备份文件下载还给用户假「成功」。
      if (!r.ok) {
        let msg = ''
        try {
          msg = (await r.json())?.error || ''
        } catch {
          /* 非 JSON body */
        }
        throw new Error(msg || t('backup.exportFailed'))
      }
      return r.blob()
    })
    .then((b) => {
      const url = URL.createObjectURL(b)
      const a = document.createElement('a')
      a.href = url
      a.download = 'ptpatronus-backup.json'
      a.click()
      URL.revokeObjectURL(url)
      ElMessage.success(password ? t('backup.exportedEncrypted') : t('backup.exported'))
    })
    .catch((e: unknown) => ElMessage.error((e as Error)?.message || t('backup.exportFailed')))
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
      // 导入前确认预览（W1：导入是覆盖式写入站点/下载器配置，此前选文件即导入，
      // 误选旧备份直接污染现网数据且不可撤销）。解析成功后展示摘要，确认才执行。
      const previewSites = Array.isArray(data?.sites) ? data.sites.length : 0
      const previewDl = Array.isArray(data?.downloaders) ? data.downloaders.length : 0
      try {
        await ElMessageBox.confirm(
          t('backup.importConfirmBody', { sites: previewSites, downloaders: previewDl }),
          t('backup.importConfirmTitle'),
          {
            type: 'warning',
            confirmButtonText: t('backup.importConfirmYes'),
            cancelButtonText: t('common.cancel'),
          },
        )
      } catch {
        importing.value = false
        return
      }
      const res = await backupApi.importOwn(data, password)
      // downloaders_skipped：同类型+同地址的下载器已存在而跳过（重复导入不再翻倍）。
      const skipped =
        (res.sites_skipped ?? 0) + (res.downloaders_skipped ?? 0) + (res.downloaders_failed ?? 0)
      const msg = t('backup.importDone', {
        sites: res.imported_sites ?? 0,
        dlPart: res.imported_downloaders != null ? ' ' + t('backup.dlPart', { n: res.imported_downloaders }) : '',
      })
      // 跳过/失败必须可见：重复域或非法条目曾被静默丢弃，只报成功数
      if (res.empty) ElMessage.warning(t('backup.importEmpty'))
      else if (skipped > 0) ElMessage.warning(msg + t('backup.importSkipPart', { n: skipped }))
      else ElMessage.success(msg)
      const plan = importFailureNotice(res.failures)
      if (plan.total) {
        ElMessage.warning(
          t('backup.importFailures', {
            total: plan.total,
            shown: plan.shown.length,
            list: plan.shown.join('；'),
          }),
        )
      }
    } catch (err) {
      const code = (err as { code?: string })?.code
      // 后端对该码已带「口令错或文件损坏」的专属原因（specific），别用固定译文覆盖。
      if (code === 'backup_password_incorrect')
        ElMessage.error((err as Error).message || t('backup.passwordIncorrect'))
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
      // 0 条不是「导入完成」：拿自有备份去点 MoviePilot 入口就是这种返回，
      // 从前无条件弹绿色 success（后端这条链不发通知，所以只有界面在说谎）。
      if (importOutcomeKind(res?.imported_sites, res?.imported_downloaders) === 'none') {
        ElMessage.warning(t('backup.importNothing'))
      } else {
        const msg = t('backup.importDone', {
          sites: res.imported_sites ?? 0,
          dlPart: res.imported_downloaders != null ? ' ' + t('backup.dlPart', { n: res.imported_downloaders }) : '',
        })
        // 缺 name / 注册域重复的条目不在成功数里：只报成功数就是把部分没进来讲成全部讲完了（#257）。
        if ((res?.sites_skipped ?? 0) > 0) ElMessage.warning(msg + t('backup.importSkipPart', { n: res.sites_skipped }))
        else ElMessage.success(msg)
      }
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
    .then((res) =>
      // 同上：.ptd 选错/空包时 imported_sites=0，不能再弹绿色成功（后端也已改成 0 条不发通知）。
      importOutcomeKind(res?.imported_sites) === 'none'
        ? ElMessage.warning(t('backup.importNothing'))
        : (res?.sites_skipped ?? 0) > 0
          ? // 包里没 URL 的条目、与现有站点重复的都记在跳过账上（#257）。
            ElMessage.warning(
              t('backup.importDoneSimple', { sites: res.imported_sites ?? 0 }) +
                t('backup.importSkipPart', { n: res.sites_skipped }),
            )
          : ElMessage.success(t('backup.importDoneSimple', { sites: res.imported_sites ?? 0 })),
    )
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
