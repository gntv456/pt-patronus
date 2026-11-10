<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, watch } from 'vue'
import {
  dockerTemplates,
  dockerTemplateCategories,
  type DockerTemplate,
  type DockerTemplateCategory,
} from '@/data/dockerTemplates'
import {
  composeTemplates,
  composeTemplateCategories,
  type ComposeTemplate,
  type ComposeTemplateCategory,
} from '@/data/composeTemplates'

const { t } = useI18n()
const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  'update:visible': [val: boolean]
  select: [tpl: DockerTemplate]
  selectCompose: [tpl: ComposeTemplate]
}>()

const ALL = t('das.catAll')
const storeMode = ref<'single' | 'compose'>('single')

function setStoreMode(m: 'single' | 'compose') {
  storeMode.value = m
  if (m === 'single') activeCategory.value = ALL
  else activeComposeCategory.value = ALL
}
const activeCategory = ref<DockerTemplateCategory | string>(ALL)
const activeComposeCategory = ref<ComposeTemplateCategory | string>(ALL)
const searchText = ref('')

watch(
  () => props.visible,
  (v) => {
    if (v) {
      storeMode.value = 'single'
      activeCategory.value = ALL
      activeComposeCategory.value = ALL
      searchText.value = ''
    }
  },
)

// 单容器模板过滤
const filteredTemplates = computed(() => {
  let list = dockerTemplates
  if (activeCategory.value !== ALL) {
    list = list.filter((t) => t.category === activeCategory.value)
  }
  if (searchText.value.trim()) {
    const q = searchText.value.toLowerCase()
    list = list.filter(
      (t) =>
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
  if (activeComposeCategory.value !== ALL) {
    list = list.filter((t) => t.category === activeComposeCategory.value)
  }
  if (searchText.value.trim()) {
    const q = searchText.value.toLowerCase()
    list = list.filter((t) => t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q))
  }
  return list
})

function countForCategory(cat: DockerTemplateCategory | string): number {
  if (cat === ALL) return dockerTemplates.length
  return dockerTemplates.filter((t) => t.category === cat).length
}

function countForComposeCategory(cat: ComposeTemplateCategory | string): number {
  if (cat === ALL) return composeTemplates.length
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
  if (!tpl.ports.length) return t('das.noPorts')
  return tpl.ports.map((p) => p.host + ':' + p.container).join(', ')
}

const resultCount = computed(() =>
  storeMode.value === 'single' ? filteredTemplates.value.length : filteredCompose.value.length,
)
</script>

<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    :title="t('das.title')"
    width="940px"
    top="3vh"
    class="app-store-dialog"
    destroy-on-close
  >
    <div class="app-store-layout">
      <!-- 左侧分类栏 -->
      <aside class="app-store-sidebar">
        <!-- 模式切换 -->
        <div class="store-mode-switch">
          <button class="mode-tab" :class="{ active: storeMode === 'single' }" @click="setStoreMode('single')">
            📦 {{ t('das.modeSingle') }}
          </button>
          <button class="mode-tab" :class="{ active: storeMode === 'compose' }" @click="setStoreMode('compose')">
            🗂️ {{ t('das.modeCompose') }}
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
          <el-input
            v-model="searchText"
            :placeholder="t('das.searchPh')"
            clearable
            size="large"
            class="store-search-input"
          >
            <template #prefix>
              <svg viewBox="0 0 24 24" class="store-search-ico">
                <path
                  d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z"
                />
              </svg>
            </template>
          </el-input>
          <span class="store-result-count">{{ t('das.appCount', { n: resultCount }) }}</span>
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
              <svg viewBox="0 0 24 24"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" /></svg>
              {{ t('das.install') }}
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
              <svg viewBox="0 0 24 24"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" /></svg>
              {{ t('das.deployCompose') }}
            </button>
          </div>
        </div>

        <!-- 空状态 -->
        <div v-if="!resultCount" class="store-empty">
          <div class="store-empty-icon">🔍</div>
          <p>{{ t('das.emptyTitle') }}</p>
          <span>{{ t('das.emptyHint') }}</span>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped src="./dockerAppStore.css"></style>
