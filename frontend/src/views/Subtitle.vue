<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  subtitleApi,
  SOURCE_LABEL,
  LANG_LABEL,
  type SubtitleResult,
  type SubtitleConfig,
  type LangCode,
} from '@/api/subtitle'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'

const q = ref('')
const language = ref('')
const results = ref<SubtitleResult[]>([])
const loading = ref(false)
const downloading = ref<string | null>(null)
const searched = ref(false)
const error = ref('')

async function search() {
  if (!q.value.trim()) {
    ElMessage.warning('请输入搜索词')
    return
  }
  loading.value = true
  error.value = ''
  searched.value = true
  try {
    results.value = await subtitleApi.search(q.value.trim(), language.value)
    if (!results.value.length) ElMessage.info('未找到字幕')
  } catch (e: any) {
    ElMessage.error(e?.message || '搜索失败')
    error.value = e?.message || '搜索失败'
    results.value = []
    searched.value = false // 出错时不显示「未找到」，由 error 态承接，避免误导
  } finally {
    loading.value = false
  }
}

async function download(sub: SubtitleResult) {
  const key = `${sub.source}:${sub.source_id}`
  downloading.value = key
  try {
    const { blob, filename } = await subtitleApi.download(sub)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    ElMessage.success(`已下载：${filename}`)
  } catch (e: any) {
    // blob 错误体需手动解析（axios 拦截器对 blob 不解析 JSON）
    ElMessage.error(e?.message || '下载失败')
  } finally {
    downloading.value = null
  }
}

// ---- 源配置 ----
const showConfig = ref(false)
const cfg = ref<SubtitleConfig | null>(null)
const cfgForm = ref({
  assrt_token: '',
  opensubtitles_key: '',
  subhd_cookie: '',
  zimuku_cookie: '',
  target_lang: '',
})
const cfgSaving = ref(false)

async function openConfig() {
  try {
    cfg.value = await subtitleApi.getConfig()
    cfgForm.value = {
      assrt_token: '',
      opensubtitles_key: '',
      subhd_cookie: '',
      zimuku_cookie: '',
      target_lang: cfg.value.target_lang || '',
    }
    showConfig.value = true
  } catch (e: any) {
    ElMessage.error(e?.message || '加载配置失败')
  }
}

async function saveConfig() {
  cfgSaving.value = true
  try {
    await subtitleApi.setConfig({
      assrt_token: cfgForm.value.assrt_token,
      opensubtitles_key: cfgForm.value.opensubtitles_key,
      subhd_cookie: cfgForm.value.subhd_cookie,
      zimuku_cookie: cfgForm.value.zimuku_cookie,
      target_lang: cfgForm.value.target_lang,
    })
    ElMessage.success('已保存')
    showConfig.value = false
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    cfgSaving.value = false
  }
}

const sourceColor: Record<string, string> = {
  assrt: '#4361ee',
  opensubtitles: '#f77f00',
  subhd: '#06a77d',
  zimuku: '#9d4edd',
}

onMounted(() => {})
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">字幕</h2>
      <el-button @click="openConfig">字幕源设置</el-button>
    </div>
    <p class="sub">
      聚合 ASSRT / OpenSubtitles / SubHD / 字幕库 搜索下载；自动解 zip、转 UTF-8、可选繁简转换。
    </p>

    <div class="search-bar glass">
      <GlassIcon name="subtitle" :size="28" from="#4cc9f0" to="#4361ee" />
      <el-input
        v-model="q"
        placeholder="搜索电影 / 剧集名（中英文均可）"
        class="search-input"
        @keyup.enter="search"
      />
      <el-select v-model="language" placeholder="语言" class="lang-select" clearable>
        <el-option label="简体" value="chs" />
        <el-option label="繁体" value="cht" />
        <el-option label="英文" value="eng" />
      </el-select>
      <el-button type="primary" :loading="loading" @click="search">搜索</el-button>
    </div>

    <div v-loading="loading" class="results">
      <div v-for="(s, i) in results" :key="`${s.source}:${s.source_id}:${i}`" class="glass glass-hover result">
        <div class="result-main">
          <el-tag size="small" :style="{ backgroundColor: sourceColor[s.source] + '22', color: sourceColor[s.source], border: 'none' }">
            {{ SOURCE_LABEL[s.source] }}
          </el-tag>
          <el-tag size="small" effect="plain">{{ LANG_LABEL[s.lang_code] || s.language || '未知' }}</el-tag>
          <span class="result-title" :title="s.title">{{ s.title }}</span>
        </div>
        <el-button
          size="small"
          type="primary"
          :loading="downloading === `${s.source}:${s.source_id}`"
          @click="download(s)"
        >下载</el-button>
      </div>
      <EmptyState
        v-if="error && !loading"
        icon="⚠️"
        title="搜索失败"
        :description="error"
      >
        <el-button @click="search">重试</el-button>
      </EmptyState>
      <EmptyState
        v-if="searched && !results.length && !loading"
        icon="💬"
        title="未找到字幕"
        description="试试更换关键词或语言筛选；部分源需在「字幕源设置」配置 token/cookie"
      />
      <EmptyState
        v-if="!searched && !loading"
        icon="💬"
        title="搜索字幕"
        description="输入电影/剧集名，跨 ASSRT / OpenSubtitles / SubHD / 字幕库 聚合搜索"
      />
    </div>

    <!-- 字幕源配置 -->
    <el-dialog v-model="showConfig" title="字幕源设置" width="560px" top="8vh">
      <el-form :model="cfgForm" label-width="150px" label-position="right">
        <el-form-item label="ASSRT Token">
          <el-input v-model="cfgForm.assrt_token" type="password" show-password
            :placeholder="cfg?.assrt_token_configured ? '已配置（留空=不修改）' : 'assrt.net API token'" />
        </el-form-item>
        <el-form-item label="OpenSubtitles Key">
          <el-input v-model="cfgForm.opensubtitles_key" type="password" show-password
            :placeholder="cfg?.opensubtitles_key_configured ? '已配置（留空=不修改）' : 'opensubtitles.com Api-Key'" />
        </el-form-item>
        <el-form-item label="SubHD Cookie">
          <el-input v-model="cfgForm.subhd_cookie" type="textarea" :rows="2"
            :placeholder="cfg?.subhd_cookie_configured ? '已配置（留空=不修改）' : '可选，绕过反爬'" />
        </el-form-item>
        <el-form-item label="字幕库 Cookie">
          <el-input v-model="cfgForm.zimuku_cookie" type="textarea" :rows="2"
            :placeholder="cfg?.zimuku_cookie_configured ? '已配置（留空=不修改）' : '可选，绕过反爬'" />
        </el-form-item>
        <el-form-item label="繁简转换">
          <el-select v-model="cfgForm.target_lang" placeholder="不转换" clearable>
            <el-option label="不转换" value="" />
            <el-option label="转为简体" value="chs" />
            <el-option label="转为繁体" value="cht" />
          </el-select>
          <span class="hint">下载时自动把文本字幕转为简体/繁体（双语/英文不转）</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConfig = false">取消</el-button>
        <el-button type="primary" :loading="cfgSaving" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 14px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 0; font-size: 22px; font-weight: 700; }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.search-bar { display: flex; align-items: center; gap: 10px; padding: 12px 14px; }
.search-input { flex: 1; }
.lang-select { width: 110px; flex-shrink: 0; }
.results { display: flex; flex-direction: column; gap: 8px; min-height: 200px; }
.result { padding: 12px 14px; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.result-main { display: flex; align-items: center; gap: 8px; min-width: 0; flex: 1; }
.result-title { font-size: var(--fs-body); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hint { font-size: var(--fs-xs); color: var(--text-muted); display: block; margin-top: 4px; }
</style>
