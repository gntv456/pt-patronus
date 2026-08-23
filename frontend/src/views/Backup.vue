<script setup lang="ts">
import { ref } from 'vue'
import { backupApi } from '@/api/backup'
import { useAuthStore } from '@/stores/auth'
import type { UploadFile } from 'element-plus'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const auth = useAuthStore()
const importing = ref(false)
const exporting = ref(false)

async function doExport() {
  // 导出含明文 Cookie / API Key / 密码，需二次确认
  try {
    await ElMessageBox.confirm('将导出含明文 Cookie / API Key / 密码的配置，请妥善保管。', '确认导出', {
      type: 'warning',
      confirmButtonText: '导出',
      cancelButtonText: '取消',
    })
  } catch {
    return // 用户取消
  }
  // 带 token 直接下载
  exporting.value = true
  const token = auth.token
  fetch(backupApi.exportUrl, { headers: { Authorization: `Bearer ${token}` } })
    .then((r) => r.blob())
    .then((b) => {
      const url = URL.createObjectURL(b)
      const a = document.createElement('a')
      a.href = url
      a.download = 'ptpatronus-backup.json'
      a.click()
      URL.revokeObjectURL(url)
      ElMessage.success('已导出')
    })
    .catch(() => ElMessage.error('导出失败'))
    .finally(() => {
      exporting.value = false
    })
}

function onFile(file: File | undefined, kind: 'own' | 'moviepilot') {
  if (!file) return
  importing.value = true
  const reader = new FileReader()
  reader.onload = async () => {
    try {
      const data = JSON.parse(String(reader.result))
      const fn = kind === 'own' ? backupApi.importOwn : backupApi.importMoviePilot
      const res = await fn(data)
      ElMessage.success(`导入完成：站点 ${res.imported_sites ?? 0}${res.imported_downloaders != null ? ' / 下载器 ' + res.imported_downloaders : ''}`)
    } catch (err) {
      ElMessage.error('导入失败：' + (err as Error).message)
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
    .then((res) => ElMessage.success(`导入完成：站点 ${res.imported_sites ?? 0}`))
    .catch((err) => ElMessage.error('导入失败：' + (err as Error).message))
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
    <h2 v-if="!props.embedded" class="title">备份</h2>
    <h3 v-else class="title embedded-title">备份</h3>

    <div class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">导出配置</div>
          <div class="bdesc text-muted">导出全部站点与下载器配置（含明文密钥，请妥善保管）为 JSON 文件。</div>
        </div>
        <el-button type="primary" :loading="exporting" :disabled="exporting" @click="doExport">导出</el-button>
      </div>
    </div>

    <div v-if="auth.isAdmin" class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">导入（PTPatronus 格式）</div>
          <div class="bdesc text-muted">从 PTPatronus 备份 JSON 恢复站点与下载器。重复项自动跳过。</div>
        </div>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".json" :on-change="onOwnUpload">
          <el-button :loading="importing">选择文件</el-button>
        </el-upload>
      </div>
    </div>

    <div v-if="auth.isAdmin" class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">从 MoviePilot 导入站点</div>
          <div class="bdesc text-muted">导入 MoviePilot 站点 JSON 数组，自动创建为 NexusPHP 站点。</div>
        </div>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".json" :on-change="onMoviePilotUpload">
          <el-button :loading="importing">选择文件</el-button>
        </el-upload>
      </div>
    </div>

    <div v-if="auth.isAdmin" class="glass card">
      <div class="brow">
        <div>
          <div class="btitle">从 PT-Depiler (.ptd) 导入站点</div>
          <div class="bdesc text-muted">导入 PT-Depiler 浏览器扩展导出的 .ptd 备份，自动恢复站点 URL/Cookie/Passkey/账号。加密备份需先在 PT-Depiler 导出未加密版本。重复项自动跳过。</div>
        </div>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".ptd" :on-change="onPTDUpload">
          <el-button :loading="importing">选择文件</el-button>
        </el-upload>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 760px; }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.card { padding: 20px; }
.brow { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-4); }
.btitle { font-weight: 600; font-size: var(--fs-lg); }
.bdesc { margin-top: 4px; font-size: var(--fs-body); }

/* embedded：嵌进设置页时去掉外页边距，标题交给分区 Tab */
.page.embedded { padding: 0; max-width: none; }
.page.embedded .embedded-title { font-size: var(--fs-lg); font-weight: 600; margin: 0; }
.page.embedded .embedded-sub { margin-top: 0; margin-bottom: 10px; font-size: var(--fs-xs); }
.page.embedded > .sub { display: none; }

</style>
