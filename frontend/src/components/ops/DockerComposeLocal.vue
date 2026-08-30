<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'
import { composeTemplates, type ComposeTemplate } from '@/data/composeTemplates'

const { t } = useI18n()
const props = defineProps<{ visible: boolean; initialContent?: string; initialName?: string }>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; done: [] }>()

type SourceType = 'template' | 'manual' | 'url'
const sourceType = ref<SourceType | ''>('')
const content = ref('')
const name = ref('')
const output = ref('')
const loading = ref(false)
const templateDrawer = ref(false)
const tplSearch = ref('')
const urlInput = ref('')
const urlLoading = ref(false)

const blankTpl = `services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
    restart: unless-stopped`

watch(
  () => props.visible,
  (v) => {
    if (v) {
      // 如果从应用商店带了模板进来，直接进入编辑
      if (props.initialContent) {
        sourceType.value = 'template'
        content.value = props.initialContent
        name.value = props.initialName || ''
      } else {
        sourceType.value = ''
        content.value = ''
        name.value = ''
      }
      output.value = ''
      templateDrawer.value = false
      tplSearch.value = ''
      urlInput.value = ''
    }
  },
)

const filteredTpls = computed(() => {
  if (!tplSearch.value.trim()) return composeTemplates
  const q = tplSearch.value.toLowerCase()
  return composeTemplates.filter((t) => t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q))
})

function selectSource(type: SourceType) {
  sourceType.value = type
  output.value = ''
  if (type === 'manual' && !content.value) {
    content.value = blankTpl
  }
  if (type === 'template') {
    templateDrawer.value = true
  }
}

function backToSource() {
  sourceType.value = ''
  content.value = ''
  name.value = ''
  output.value = ''
}

function applyTemplate(tpl: ComposeTemplate) {
  content.value = tpl.content
  name.value = tpl.id
  templateDrawer.value = false
  ElMessage.success(t('dcl.tplLoaded', { name: tpl.name }))
}

async function fetchFromUrl() {
  if (!urlInput.value.trim()) {
    ElMessage.warning(t('dcl.urlRequired'))
    return
  }
  urlLoading.value = true
  try {
    const resp = await fetch(urlInput.value.trim())
    if (!resp.ok) throw new Error('HTTP ' + resp.status)
    const text = await resp.text()
    if (!text.includes('services:') && !text.includes('version:')) {
      ElMessage.warning(t('dcl.notCompose'))
    }
    content.value = text
    sourceType.value = 'url'
    ElMessage.success(t('dcl.fetched'))
  } catch (e: any) {
    ElMessage.error(t('dcl.fetchFailed', { err: e.message }))
  } finally {
    urlLoading.value = false
  }
}

async function action(act: 'up' | 'down' | 'validate') {
  if (!content.value.trim()) {
    ElMessage.warning(t('dcl.contentRequired'))
    return
  }
  loading.value = true
  output.value = ''
  try {
    const fn =
      act === 'up'
        ? dockerApi.localComposeUp
        : act === 'down'
          ? dockerApi.localComposeDown
          : dockerApi.localComposeValidate
    const res = await fn(content.value, name.value.trim() || undefined)
    output.value = res.output || (res.ok ? t('dcl.ok') : t('dcl.ko'))
    if (res.ok && act === 'up') {
      ElMessage.success(t('dcl.deployed'))
      emit('done')
    }
    if (res.ok && act === 'down') {
      ElMessage.success(t('dcl.stopped'))
      emit('done')
    }
    if (res.ok && act === 'validate') ElMessage.success(t('dcl.validated'))
  } catch (e: any) {
    output.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    :title="t('dcl.title')"
    width="820px"
    top="5vh"
    destroy-on-close
    class="compose-create-dialog"
  >
    <!-- 步骤一：选择来源 -->
    <div v-if="!sourceType" class="source-picker">
      <p class="source-hint">{{ t('dcl.sourceHint') }}</p>
      <div class="source-cards">
        <div v-a11y-button class="source-card" @click="selectSource('template')">
          <div class="source-card-icon" style="background: var(--pt-primary-bg); color: var(--pt-primary)">📋</div>
          <div class="source-card-body">
            <div class="source-card-title">{{ t('dcl.srcTemplate') }}</div>
            <div class="source-card-desc">{{ t('dcl.srcTemplateDesc') }}</div>
          </div>
          <svg class="source-arrow" viewBox="0 0 24 24">
            <path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" stroke-width="2" />
          </svg>
        </div>

        <div v-a11y-button class="source-card" @click="selectSource('manual')">
          <div class="source-card-icon" style="background: rgba(24, 160, 88, 0.12); color: #18a058">✏️</div>
          <div class="source-card-body">
            <div class="source-card-title">{{ t('dcl.srcManual') }}</div>
            <div class="source-card-desc">{{ t('dcl.srcManualDesc') }}</div>
          </div>
          <svg class="source-arrow" viewBox="0 0 24 24">
            <path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" stroke-width="2" />
          </svg>
        </div>

        <div v-a11y-button class="source-card" @click="selectSource('url')">
          <div class="source-card-icon" style="background: rgba(0, 150, 57, 0.12); color: #009639">🌐</div>
          <div class="source-card-body">
            <div class="source-card-title">{{ t('dcl.srcUrl') }}</div>
            <div class="source-card-desc">{{ t('dcl.srcUrlDesc') }}</div>
          </div>
          <svg class="source-arrow" viewBox="0 0 24 24">
            <path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" stroke-width="2" />
          </svg>
        </div>
      </div>
    </div>

    <!-- 在线拉取输入 -->
    <div v-if="sourceType === 'url' && !content" class="url-fetch-area">
      <el-button text @click="backToSource" class="back-btn">← {{ t('dcl.backToSource') }}</el-button>
      <el-input v-model="urlInput" placeholder="https://raw.githubusercontent.com/.../docker-compose.yml" size="large">
        <template #prepend>URL</template>
      </el-input>
      <el-button type="primary" size="large" :loading="urlLoading" @click="fetchFromUrl" style="margin-top: 12px">{{
        t('dcl.fetchBtn')
      }}</el-button>
      <p class="url-tip">💡 {{ t('dcl.urlTip') }}</p>
    </div>

    <!-- 步骤二：编辑区（模板/手动/URL拉取成功后） -->
    <div v-if="sourceType && content" class="compose-editor-layout">
      <div class="editor-topbar">
        <el-button text @click="backToSource" class="back-btn">← {{ t('common.back') }}</el-button>
        <el-input v-model="name" :placeholder="t('dcl.namePh')" style="width: 220px" clearable size="small">
          <template #prefix
            ><span class="field-label">{{ t('dcl.nameLabel') }}</span></template
          >
        </el-input>
        <el-button size="small" @click="templateDrawer = true">{{ t('dcl.changeTpl') }}</el-button>
        <span class="source-tag">{{
          sourceType === 'template'
            ? '📦 ' + t('dcl.srcTemplate')
            : sourceType === 'url'
              ? '🌐 ' + t('dcl.srcUrl')
              : '✏️ ' + t('dcl.srcManual')
        }}</span>
      </div>

      <el-input v-model="content" type="textarea" :rows="18" class="compose-yaml" />

      <div v-if="output" class="compose-output">
        <pre>{{ output }}</pre>
      </div>
    </div>

    <template #footer>
      <template v-if="sourceType && content">
        <el-button @click="emit('update:visible', false)">{{ t('common.close') }}</el-button>
        <el-button :loading="loading" @click="action('validate')">{{ t('dcl.validateBtn') }}</el-button>
        <el-button type="danger" :loading="loading" @click="action('down')">{{ t('dcl.stopBtn') }}</el-button>
        <el-button type="primary" :loading="loading" @click="action('up')">{{ t('dcl.deployBtn') }}</el-button>
      </template>
      <el-button v-else @click="emit('update:visible', false)">{{ t('common.cancel') }}</el-button>
    </template>

    <!-- 模板选择抽屉 -->
    <el-drawer v-model="templateDrawer" :title="t('dcl.tplTitle')" size="480px" direction="rtl" append-to-body>
      <div class="tpl-search-bar">
        <el-input v-model="tplSearch" :placeholder="t('dcl.tplSearchPh')" clearable size="default">
          <template #prefix>
            <svg viewBox="0 0 24 24" style="width: 16px; height: 16px; fill: var(--text-muted)">
              <path
                d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z"
              />
            </svg>
          </template>
        </el-input>
      </div>
      <div class="tpl-list">
        <div v-a11y-button v-for="tpl in filteredTpls" :key="tpl.id" class="tpl-card" @click="applyTemplate(tpl)">
          <div class="tpl-icon" :style="{ background: tpl.color + '22', color: tpl.color }">{{ tpl.icon }}</div>
          <div class="tpl-body">
            <div class="tpl-name">{{ tpl.name }}</div>
            <div class="tpl-desc">{{ tpl.description }}</div>
            <div class="tpl-services">
              <span v-for="s in tpl.services" :key="s" class="tpl-svc-tag">{{ s }}</span>
            </div>
          </div>
        </div>
        <div v-if="!filteredTpls.length" class="tpl-empty">{{ t('dcl.tplEmpty') }}</div>
      </div>
    </el-drawer>
  </el-dialog>
</template>

<style scoped src="./DockerComposeLocal.css"></style>
