<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import type { OpsConfig, OpsConfigHistory, OpsConfigInput } from '@/api/monitor'
import { configFormatText } from '@/views/nasServerHelpers'

// form 用 defineModel 双向绑定（父级 v-model:form）：表单字段修改经 update:form 回传。
const { t } = useI18n()

const form = defineModel<OpsConfigInput>('form', { required: true })

const props = defineProps<{
  drawerVisible: boolean
  configs: OpsConfig[]
  history: OpsConfigHistory[]
  historyTarget: OpsConfig | null
  formVisible: boolean
  editing: OpsConfig | null
}>()

const emit = defineEmits<{
  (e: 'update:drawerVisible', v: boolean): void
  (e: 'update:formVisible', v: boolean): void
  (e: 'add'): void
  (e: 'edit', c: OpsConfig): void
  (e: 'remove', c: OpsConfig): void
  (e: 'showHistory', c: OpsConfig): void
  (e: 'save'): void
}>()

const valuePlaceholder = computed(() =>
  form.value.format === 'text' ? t('ncf.anyText') : '{\n  "key": "value"\n}',
)
</script>

<template>
  <el-drawer
    :model-value="drawerVisible"
    :title="t('ncf.title')"
    size="640px"
    @update:model-value="emit('update:drawerVisible', $event)"
  >
    <div class="cred-toolbar">
      <span>{{ t('ncf.subtitle') }}</span>
      <el-button type="primary" size="small" @click="emit('add')">{{ t('ncf.add') }}</el-button>
    </div>
    <div v-if="configs.length" class="cred-list">
      <div v-for="c in configs" :key="c.id" class="cred-card config-card">
        <div class="cred-main">
          <div>
            <b>{{ c.name }}</b>
            <span>{{ c.key }} · {{ c.env || 'default' }}</span>
          </div>
          <el-tag size="small" type="info">v{{ c.version }} · {{ configFormatText(c.format) }}</el-tag>
        </div>
        <pre class="config-preview">{{ c.value || t('ncf.emptyValue') }}</pre>
        <div class="cred-flags">
          <span v-if="c.tags">{{ c.tags }}</span>
          <span>{{ new Date(c.updated_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
        </div>
        <p v-if="c.note">{{ c.note }}</p>
        <div class="cred-actions">
          <el-button size="small" @click="emit('edit', c)">{{ t('common.edit') }}</el-button>
          <el-button size="small" @click="emit('showHistory', c)">{{ t('ncf.history') }}</el-button>
          <el-button size="small" type="danger" plain @click="emit('remove', c)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
    </div>
    <div v-else class="cred-empty">
      {{ t('ncf.empty') }}
      <el-button type="primary" size="small" @click="emit('add')">{{ t('ncf.addFirst') }}</el-button>
    </div>

    <section v-if="historyTarget" class="config-history">
      <div class="section-title">{{ t('ncf.historyOf', { name: historyTarget.name }) }}</div>
      <div v-if="history.length" class="history-list">
        <div v-for="h in history" :key="h.id" class="history-item">
          <div class="history-top">
            <b>v{{ h.version }}</b>
            <span>{{ new Date(h.created_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
          </div>
          <pre>{{ h.value || t('ncf.emptyValue') }}</pre>
        </div>
      </div>
      <div v-else class="cred-empty">{{ t('ncf.noHistory') }}</div>
    </section>
  </el-drawer>

  <el-dialog
    :model-value="formVisible"
    :title="editing ? t('ncf.editTitle') : t('ncf.add')"
    width="560px"
    destroy-on-close
    @update:model-value="emit('update:formVisible', $event)"
  >
    <el-form label-width="82px">
      <el-form-item label="Key" required>
        <el-input v-model="form.key" placeholder="app.deploy.prod" />
      </el-form-item>
      <el-form-item :label="t('common.name')" required>
        <el-input v-model="form.name" :placeholder="t('ncf.namePh')" />
      </el-form-item>
      <el-form-item :label="t('ncf.env')">
        <el-input v-model="form.env" placeholder="prod / test / dev" />
      </el-form-item>
      <el-form-item :label="t('ncf.format')">
        <el-radio-group v-model="form.format">
          <el-radio-button label="kv">KV</el-radio-button>
          <el-radio-button label="text">{{ t('ncf.fmtText') }}</el-radio-button>
          <el-radio-button label="json">JSON</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item :label="t('ncf.content')" required>
        <el-input
          v-model="form.value"
          type="textarea"
          :rows="9"
          resize="vertical"
          :placeholder="valuePlaceholder"
        />
      </el-form-item>
      <el-form-item :label="t('ncd.tags')">
        <el-input v-model="form.tags" :placeholder="t('ncf.tagsPh')" />
      </el-form-item>
      <el-form-item :label="t('ncd.note')">
        <el-input v-model="form.note" type="textarea" :rows="3" resize="vertical" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:formVisible', false)">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" @click="emit('save')">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped src="../../views/nas-shared/config.css"></style>
