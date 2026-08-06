<script setup lang="ts">
import { computed } from 'vue'
import type { OpsConfig, OpsConfigHistory, OpsConfigInput } from '@/api/monitor'
import { configFormatText } from '@/views/nasServerHelpers'

// form 是父级 composable 持有的响应式对象，子组件直接绑定其字段。
const props = defineProps<{
  drawerVisible: boolean
  configs: OpsConfig[]
  history: OpsConfigHistory[]
  historyTarget: OpsConfig | null
  formVisible: boolean
  editing: OpsConfig | null
  form: OpsConfigInput
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
  props.form.format === 'text' ? '任意文本配置' : '{\n  "key": "value"\n}',
)
</script>

<template>
  <el-drawer
    :model-value="drawerVisible"
    title="配置中心"
    size="640px"
    @update:model-value="emit('update:drawerVisible', $event)"
  >
    <div class="cred-toolbar">
      <span>统一管理 KV、文本、JSON 配置，保存时自动生成历史版本。</span>
      <el-button type="primary" size="small" @click="emit('add')">新增配置</el-button>
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
        <pre class="config-preview">{{ c.value || '空配置' }}</pre>
        <div class="cred-flags">
          <span v-if="c.tags">{{ c.tags }}</span>
          <span>{{ new Date(c.updated_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
        </div>
        <p v-if="c.note">{{ c.note }}</p>
        <div class="cred-actions">
          <el-button size="small" @click="emit('edit', c)">编辑</el-button>
          <el-button size="small" @click="emit('showHistory', c)">历史</el-button>
          <el-button size="small" type="danger" plain @click="emit('remove', c)">删除</el-button>
        </div>
      </div>
    </div>
    <div v-else class="cred-empty">
      暂无配置
      <el-button type="primary" size="small" @click="emit('add')">新增第一条</el-button>
    </div>

    <section v-if="historyTarget" class="config-history">
      <div class="section-title">历史版本 · {{ historyTarget.name }}</div>
      <div v-if="history.length" class="history-list">
        <div v-for="h in history" :key="h.id" class="history-item">
          <div class="history-top">
            <b>v{{ h.version }}</b>
            <span>{{ new Date(h.created_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
          </div>
          <pre>{{ h.value || '空配置' }}</pre>
        </div>
      </div>
      <div v-else class="cred-empty">暂无历史记录</div>
    </section>
  </el-drawer>

  <el-dialog
    :model-value="formVisible"
    :title="editing ? '编辑配置' : '新增配置'"
    width="560px"
    destroy-on-close
    @update:model-value="emit('update:formVisible', $event)"
  >
    <el-form label-width="82px">
      <el-form-item label="Key" required>
        <el-input v-model="form.key" placeholder="app.deploy.prod" />
      </el-form-item>
      <el-form-item label="名称" required>
        <el-input v-model="form.name" placeholder="生产发布配置" />
      </el-form-item>
      <el-form-item label="环境">
        <el-input v-model="form.env" placeholder="prod / test / dev" />
      </el-form-item>
      <el-form-item label="格式">
        <el-radio-group v-model="form.format">
          <el-radio-button label="kv">KV</el-radio-button>
          <el-radio-button label="text">文本</el-radio-button>
          <el-radio-button label="json">JSON</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="内容" required>
        <el-input
          v-model="form.value"
          type="textarea"
          :rows="9"
          resize="vertical"
          :placeholder="valuePlaceholder"
        />
      </el-form-item>
      <el-form-item label="标签">
        <el-input v-model="form.tags" placeholder="生产, 发布" />
      </el-form-item>
      <el-form-item label="备注">
        <el-input v-model="form.note" type="textarea" :rows="3" resize="vertical" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:formVisible', false)">取消</el-button>
      <el-button type="primary" @click="emit('save')">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped src="../../views/nas-shared/config.css"></style>
