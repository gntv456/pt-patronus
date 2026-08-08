<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { dockerTemplates, dockerTemplateCategories, type DockerTemplate, type DockerTemplateCategory } from '@/data/dockerTemplates'
import { composeTemplates, composeTemplateCategories, type ComposeTemplate, type ComposeTemplateCategory } from '@/data/composeTemplates'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  'update:visible': [val: boolean]
  select: [tpl: DockerTemplate]
  selectCompose: [tpl: ComposeTemplate]
}>()

const storeMode = ref<'single' | 'compose'>('single')
const activeCategory = ref<DockerTemplateCategory | '全部'>('全部')
const activeComposeCategory = ref<ComposeTemplateCategory | '全部'>('全部')
const searchText = ref('')

watch(() => props.visible, (v) => {
  if (v) {
    storeMode.value = 'single'
    activeCategory.value = '全部'
    activeComposeCategory.value = '全部'
    searchText.value = ''
  }
})

// 单容器模板过滤
const filteredTemplates = computed(() => {
  let list = dockerTemplates
  if (activeCategory.value !== '全部') {
    list = list.filter((t) => t.category === activeCategory.value)
  }
  if (searchText.value.trim()) {
    const q = searchText.value.toLowerCase()
    list = list.filter((t) =>
      t.name.toLowerCase().includes(q) ||
      t.description.toLowerCase().includes(q) ||
      t.category.toLowerCase().includes(q),
    )
  }
  return list
})

// Compose 模板过滤
const filteredCompose = computed(() => {
  let list = composeTemplates
  if (activeComposeCategory.value !== '全部') {
    list = list.filter((t) => t.category === activeComposeCategory.value)
  }
  if (searchText.value.trim()) {
    const q = searchText.value.toLowerCase()
    list = list.filter((t) =>
      t.name.toLowerCase().includes(q) ||
      t.description.toLowerCase().includes(q),
    )
  }
  return list
})

function countForCategory(cat: DockerTemplateCategory | '全部'): number {
  if (cat === '全部') return dockerTemplates.length
  return dockerTemplates.filter((t) => t.category === cat).length
}

function countForComposeCategory(cat: ComposeTemplateCategory | '全部'): number {
  if (cat === '全部') return composeTemplates.length
  return composeTemplates.filter((t) => t.category === cat).length
}

function selectTemplate(tpl: DockerTemplate) {
  emit('select', tpl)
  emit('update:visible', false)
}

function selectComposeTemplate(tpl: ComposeTemplate) {
  emit('selectCompose', tpl)
  emit('update:visible', false)
}

function portSummary(tpl: DockerTemplate): string {
  if (!tpl.ports.length) return '无端口'
  return tpl.ports.map((p) => p.host + ':' + p.container).join(', ')
}

const resultCount = computed(() => storeMode.value === 'single' ? filteredTemplates.value.length : filteredCompose.value.length)
</script>

<template>
  <el-dialog :model-value="visible" @update:model-value="emit('update:visible', $event)"
    title="应用商店" width="940px" top="3vh" class="app-store-dialog" destroy-on-close>
    <div class="app-store-layout">
      <!-- 左侧分类栏 -->
      <aside class="app-store-sidebar">
        <!-- 模式切换 -->
        <div class="store-mode-switch">
          <button class="mode-tab" :class="{ active: storeMode === 'single' }" @click="storeMode = 'single'; activeCategory = '全部'">
            📦 单容器
          </button>
          <button class="mode-tab" :class="{ active: storeMode === 'compose' }" @click="storeMode = 'compose'; activeComposeCategory = '全部'">
            🗂️ Compose 编排
          </button>
        </div>

        <!-- 单容器分类 -->
        <template v-if="storeMode === 'single'">
          <button
            v-for="cat in dockerTemplateCategories"
            :key="cat.key"
            class="cat-item"
            :class="{ active: activeCategory === cat.key }"
            @click="activeCategory = cat.key"
          >
            <span class="cat-icon">{{ cat.icon }}</span>
            <span class="cat-name">{{ cat.key }}</span>
            <span class="cat-count">{{ countForCategory(cat.key) }}</span>
          </button>
        </template>

        <!-- Compose 分类 -->
        <template v-else>
          <button
            v-for="cat in composeTemplateCategories"
            :key="cat.key"
            class="cat-item"
            :class="{ active: activeComposeCategory === cat.key }"
            @click="activeComposeCategory = cat.key"
          >
            <span class="cat-icon">{{ cat.icon }}</span>
            <span class="cat-name">{{ cat.key }}</span>
            <span class="cat-count">{{ countForComposeCategory(cat.key) }}</span>
          </button>
        </template>
      </aside>

      <!-- 右侧应用区 -->
      <div class="app-store-main">
        <!-- 搜索栏 -->
        <div class="store-search-bar">
          <el-input v-model="searchText" placeholder="搜索应用名称或描述…" clearable size="large" class="store-search-input">
            <template #prefix>
              <svg viewBox="0 0 24 24" class="store-search-ico"><path d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z"/></svg>
            </template>
          </el-input>
          <span class="store-result-count">{{ resultCount }} 个应用</span>
        </div>

        <!-- 单容器应用卡片网格 -->
        <div v-if="storeMode === 'single'" class="store-app-grid">
          <div v-for="tpl in filteredTemplates" :key="tpl.id" class="store-app-card">
            <div class="app-card-top">
              <div class="app-icon" :style="{ background: tpl.color + '22', color: tpl.color }">
                <span>{{ tpl.icon }}</span>
              </div>
              <div class="app-info">
                <h3 class="app-name">{{ tpl.name }}</h3>
                <p class="app-desc">{{ tpl.description }}</p>
              </div>
            </div>
            <div class="app-card-meta">
              <span class="app-cat-tag">{{ tpl.category }}</span>
              <span class="app-port" :title="portSummary(tpl)">{{ portSummary(tpl) }}</span>
            </div>
            <button class="app-install-btn" @click="selectTemplate(tpl)">
              <svg viewBox="0 0 24 24"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg>
              安装
            </button>
          </div>
        </div>

        <!-- Compose 编排卡片网格 -->
        <div v-else class="store-app-grid">
          <div v-for="tpl in filteredCompose" :key="tpl.id" class="store-app-card store-app-card--compose">
            <div class="app-card-top">
              <div class="app-icon" :style="{ background: tpl.color + '22', color: tpl.color }">
                <span>{{ tpl.icon }}</span>
              </div>
              <div class="app-info">
                <h3 class="app-name">{{ tpl.name }}</h3>
                <p class="app-desc">{{ tpl.description }}</p>
              </div>
            </div>
            <div class="app-card-meta">
              <span class="compose-services">
                <span v-for="s in tpl.services" :key="s" class="compose-svc-chip">{{ s }}</span>
              </span>
            </div>
            <button class="app-install-btn app-install-btn--compose" @click="selectComposeTemplate(tpl)">
              <svg viewBox="0 0 24 24"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg>
              部署编排
            </button>
          </div>
        </div>

        <!-- 空状态 -->
        <div v-if="!resultCount" class="store-empty">
          <div class="store-empty-icon">🔍</div>
          <p>未找到匹配的应用</p>
          <span>试试其他关键词或切换分类</span>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped src="./dockerAppStore.css"></style>
