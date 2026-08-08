<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'
import { composeTemplates, type ComposeTemplate } from '@/data/composeTemplates'

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

watch(() => props.visible, (v) => {
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
})

const filteredTpls = computed(() => {
  if (!tplSearch.value.trim()) return composeTemplates
  const q = tplSearch.value.toLowerCase()
  return composeTemplates.filter((t) =>
    t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q),
  )
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
  ElMessage.success('已加载模板：' + tpl.name)
}

async function fetchFromUrl() {
  if (!urlInput.value.trim()) { ElMessage.warning('请输入 URL'); return }
  urlLoading.value = true
  try {
    const resp = await fetch(urlInput.value.trim())
    if (!resp.ok) throw new Error('HTTP ' + resp.status)
    const text = await resp.text()
    if (!text.includes('services:') && !text.includes('version:')) {
      ElMessage.warning('内容看起来不像 compose 文件，请检查 URL')
    }
    content.value = text
    sourceType.value = 'url'
    ElMessage.success('已拉取 compose 文件')
  } catch (e: any) {
    ElMessage.error('拉取失败：' + e.message + '（可能是 CORS 限制，请复制内容后选手动编写）')
  } finally {
    urlLoading.value = false
  }
}

async function action(act: 'up' | 'down' | 'validate') {
  if (!content.value.trim()) { ElMessage.warning('请输入 compose 内容'); return }
  loading.value = true
  output.value = ''
  try {
    const fn = act === 'up' ? dockerApi.localComposeUp : act === 'down' ? dockerApi.localComposeDown : dockerApi.localComposeValidate
    const res = await fn(content.value, name.value.trim() || undefined)
    output.value = res.output || (res.ok ? '成功' : '失败')
    if (res.ok && act === 'up') { ElMessage.success('部署成功'); emit('done') }
    if (res.ok && act === 'down') { ElMessage.success('已停止'); emit('done') }
    if (res.ok && act === 'validate') ElMessage.success('配置校验通过')
  } catch (e: any) { output.value = e.message }
  finally { loading.value = false }
}
</script>

<template>
  <el-dialog :model-value="visible" @update:model-value="emit('update:visible', $event)"
    title="新建 Compose 项目" width="820px" top="5vh" destroy-on-close class="compose-create-dialog">
    <!-- 步骤一：选择来源 -->
    <div v-if="!sourceType" class="source-picker">
      <p class="source-hint">请选择 Compose 文件来源方式</p>
      <div class="source-cards">
        <div class="source-card" @click="selectSource('template')">
          <div class="source-card-icon" style="background: rgba(108,92,231,.12); color: #6c5ce7">📋</div>
          <div class="source-card-body">
            <div class="source-card-title">从模板创建</div>
            <div class="source-card-desc">选择预置的应用模板（WordPress、Jellyfin 套件等），一键填充后微调部署</div>
          </div>
          <svg class="source-arrow" viewBox="0 0 24 24"><path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" stroke-width="2"/></svg>
        </div>

        <div class="source-card" @click="selectSource('manual')">
          <div class="source-card-icon" style="background: rgba(24,160,88,.12); color: #18a058">✏️</div>
          <div class="source-card-body">
            <div class="source-card-title">手动编写</div>
            <div class="source-card-desc">直接在编辑器中编写或粘贴 docker-compose.yml 内容</div>
          </div>
          <svg class="source-arrow" viewBox="0 0 24 24"><path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" stroke-width="2"/></svg>
        </div>

        <div class="source-card" @click="selectSource('url')">
          <div class="source-card-icon" style="background: rgba(0,150,57,.12); color: #009639">🌐</div>
          <div class="source-card-body">
            <div class="source-card-title">在线拉取</div>
            <div class="source-card-desc">从 URL 拉取 docker-compose.yml（如 GitHub Raw 链接），填入编辑器</div>
          </div>
          <svg class="source-arrow" viewBox="0 0 24 24"><path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" stroke-width="2"/></svg>
        </div>
      </div>
    </div>

    <!-- 在线拉取输入 -->
    <div v-if="sourceType === 'url' && !content" class="url-fetch-area">
      <el-button text @click="backToSource" class="back-btn">← 返回选择来源</el-button>
      <el-input v-model="urlInput" placeholder="https://raw.githubusercontent.com/.../docker-compose.yml" size="large">
        <template #prepend>URL</template>
      </el-input>
      <el-button type="primary" size="large" :loading="urlLoading" @click="fetchFromUrl" style="margin-top: 12px">拉取文件</el-button>
      <p class="url-tip">💡 支持 GitHub Raw、Gitee 等直链。若遇到跨域限制，请复制文件内容后选择「手动编写」。</p>
    </div>

    <!-- 步骤二：编辑区（模板/手动/URL拉取成功后） -->
    <div v-if="sourceType && content" class="compose-editor-layout">
      <div class="editor-topbar">
        <el-button text @click="backToSource" class="back-btn">← 返回</el-button>
        <el-input v-model="name" placeholder="项目名（可选）" style="width: 220px" clearable size="small">
          <template #prefix><span class="field-label">项目名</span></template>
        </el-input>
        <el-button size="small" @click="templateDrawer = true">更换模板</el-button>
        <span class="source-tag">{{ sourceType === 'template' ? '📦 模板' : sourceType === 'url' ? '🌐 在线拉取' : '✏️ 手动编写' }}</span>
      </div>

      <el-input v-model="content" type="textarea" :rows="18" class="compose-yaml" />

      <div v-if="output" class="compose-output"><pre>{{ output }}</pre></div>
    </div>

    <template #footer>
      <template v-if="sourceType && content">
        <el-button @click="emit('update:visible', false)">关闭</el-button>
        <el-button :loading="loading" @click="action('validate')">验证配置</el-button>
        <el-button type="danger" :loading="loading" @click="action('down')">停止</el-button>
        <el-button type="primary" :loading="loading" @click="action('up')">部署</el-button>
      </template>
      <el-button v-else @click="emit('update:visible', false)">取消</el-button>
    </template>

    <!-- 模板选择抽屉 -->
    <el-drawer v-model="templateDrawer" title="选择 Compose 模板" size="480px" direction="rtl" append-to-body>
      <div class="tpl-search-bar">
        <el-input v-model="tplSearch" placeholder="搜索模板…" clearable size="default">
          <template #prefix>
            <svg viewBox="0 0 24 24" style="width:16px;height:16px;fill:var(--text-muted)"><path d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z"/></svg>
          </template>
        </el-input>
      </div>
      <div class="tpl-list">
        <div v-for="tpl in filteredTpls" :key="tpl.id" class="tpl-card" @click="applyTemplate(tpl)">
          <div class="tpl-icon" :style="{ background: tpl.color + '22', color: tpl.color }">{{ tpl.icon }}</div>
          <div class="tpl-body">
            <div class="tpl-name">{{ tpl.name }}</div>
            <div class="tpl-desc">{{ tpl.description }}</div>
            <div class="tpl-services">
              <span v-for="s in tpl.services" :key="s" class="tpl-svc-tag">{{ s }}</span>
            </div>
          </div>
        </div>
        <div v-if="!filteredTpls.length" class="tpl-empty">未找到匹配的模板</div>
      </div>
    </el-drawer>
  </el-dialog>
</template>

<style scoped>
/* 来源选择 */
.source-picker { padding: 8px 4px; }
.source-hint { text-align: center; font-size: 15px; color: var(--text-muted); margin: 8px 0 24px; }
.source-cards { display: flex; flex-direction: column; gap: 14px; max-width: 560px; margin: 0 auto; }
.source-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  border-radius: var(--radius-lg, 12px);
  border: 2px solid var(--divider);
  background: var(--surface-1);
  cursor: pointer;
  transition: all 0.18s;
}
.source-card:hover {
  border-color: var(--pt-primary);
  box-shadow: var(--surface-shadow-hover, 0 4px 16px rgba(0,0,0,.08));
  transform: translateY(-2px);
}
.source-card-icon {
  flex: none;
  width: 56px; height: 56px;
  border-radius: var(--radius-md, 10px);
  display: flex; align-items: center; justify-content: center;
  font-size: 28px;
}
.source-card-body { flex: 1; }
.source-card-title { font-size: 16px; font-weight: 700; color: var(--text-strong); margin-bottom: 4px; }
.source-card-desc { font-size: 13px; color: var(--text-muted); line-height: 1.5; }
.source-arrow { width: 20px; height: 20px; flex: none; color: var(--text-muted); }

/* URL 拉取 */
.url-fetch-area { max-width: 600px; margin: 16px auto; }
.url-tip { font-size: 12px; color: var(--text-muted); margin-top: 16px; line-height: 1.6; }

/* 编辑区 */
.compose-editor-layout { display: flex; flex-direction: column; gap: 12px; }
.editor-topbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.back-btn { color: var(--text-muted) !important; padding: 0 8px !important; }
.back-btn:hover { color: var(--pt-primary) !important; }
.field-label { font-size: 12px; color: var(--text-muted); white-space: nowrap; }
.source-tag {
  margin-left: auto;
  font-size: 12px;
  padding: 3px 10px;
  border-radius: var(--radius-pill, 999px);
  background: var(--surface-muted);
  color: var(--text-muted);
}
.compose-yaml :deep(textarea) {
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
}
.compose-output {
  background: var(--surface-muted);
  border-radius: var(--radius-sm, 6px);
  padding: 10px 12px;
  max-height: 200px;
  overflow: auto;
}
.compose-output pre { margin: 0; font-size: 12px; font-family: monospace; white-space: pre-wrap; word-break: break-all; }

/* 模板抽屉 */
.tpl-search-bar { margin-bottom: 12px; }
.tpl-list { display: flex; flex-direction: column; gap: 10px; }
.tpl-card {
  display: flex;
  gap: 12px;
  padding: 12px;
  border-radius: var(--radius-md, 8px);
  border: 1px solid var(--divider);
  background: var(--surface-1);
  cursor: pointer;
  transition: all 0.15s;
}
.tpl-card:hover {
  border-color: var(--pt-primary);
  box-shadow: var(--surface-shadow-hover);
  transform: translateX(-2px);
}
.tpl-icon {
  flex: none;
  width: 42px; height: 42px;
  border-radius: var(--radius-md, 8px);
  display: flex; align-items: center; justify-content: center;
  font-size: 22px;
}
.tpl-body { flex: 1; min-width: 0; }
.tpl-name { font-size: 14px; font-weight: 700; color: var(--text-strong); margin-bottom: 2px; }
.tpl-desc { font-size: 12px; color: var(--text-muted); line-height: 1.4; margin-bottom: 6px; }
.tpl-services { display: flex; flex-wrap: wrap; gap: 4px; }
.tpl-svc-tag {
  font-size: 11px;
  font-family: monospace;
  padding: 1px 6px;
  border-radius: var(--radius-xs, 4px);
  background: var(--surface-muted);
  color: var(--text-muted);
}
.tpl-empty { text-align: center; padding: 32px; color: var(--text-muted); font-size: 13px; }
</style>
