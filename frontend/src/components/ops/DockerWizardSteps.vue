<script setup lang="ts">
// 创建向导步骤 0-4 的表单面板（镜像/基础/端口/卷/环境变量）。
// 从 DockerCreateWizard.vue 迁出；行操作直接改 props 传入的数组（push/splice 均可）。
// form 是宿主的响应式对象引用：v-model 绑定其字段属"经引用改深层属性"，
// 与改 props 本身的 lint 反模式不同，这里整文件豁免。
/* eslint-disable vue/no-mutating-props */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { dockerTemplates, type DockerTemplate } from '@/data/dockerTemplates'

const { t } = useI18n()

const props = defineProps<{
  activeStep: number
  form: {
    image: string
    name: string
    cmd: string
    cpuLimit: number
    memoryLimitMB: number
    privileged: boolean
    networkMode: string
    restartPolicy: string
    autoStart: boolean
    ports: Array<{ host: number | string; container: number | string; protocol: 'tcp' | 'udp' }>
    volumes: Array<{ host: string; container: string; mode: 'rw' | 'ro' }>
    env: Array<{ key: string; value: string }>
  }
  images?: { Repository: string; Tag: string }[]
  selectedTemplate: string
  showTemplates: boolean
}>()

const emit = defineEmits<{
  applyTemplate: [tpl: DockerTemplate]
}>()

const networkOptions = computed(() => [
  { value: 'bridge', label: t('dw.net.bridge') },
  { value: 'host', label: t('dw.net.host') },
  { value: 'none', label: t('dw.net.none') },
])
const restartOptions = computed(() => [
  { value: 'no', label: t('dw.restart.no') },
  { value: 'always', label: t('dw.restart.always') },
  { value: 'unless-stopped', label: t('dw.restart.unless') },
  { value: 'on-failure', label: t('dw.restart.onFailure') },
])

const imageSuggestions = computed(() => {
  if (!props.images) return []
  return props.images.map((i) => `${i.Repository}:${i.Tag}`).filter((s) => !s.includes('<none>'))
})

function addPort() {
  props.form.ports.push({ host: '', container: '', protocol: 'tcp' })
}
function removePort(i: number) {
  props.form.ports.splice(i, 1)
}
function addVolume() {
  props.form.volumes.push({ host: '', container: '', mode: 'rw' })
}
function removeVolume(i: number) {
  props.form.volumes.splice(i, 1)
}
function addEnv() {
  props.form.env.push({ key: '', value: '' })
}
function removeEnv(i: number) {
  props.form.env.splice(i, 1)
}
</script>

<template>
  <div class="wizard-body">
    <!-- 步骤 0：选择镜像 -->
    <div v-if="activeStep === 0" class="step-panel">
      <div v-if="showTemplates" class="template-section">
        <div class="section-title">
          {{ t('dw.fromTemplate') }} <span class="section-hint">{{ t('dw.templateHint') }}</span>
        </div>
        <div class="template-grid">
          <div
            v-a11y-button
            v-for="tpl in dockerTemplates"
            :key="tpl.id"
            class="template-card"
            :class="{ selected: selectedTemplate === tpl.id }"
            @click="emit('applyTemplate', tpl)"
          >
            <span class="tpl-icon" :style="{ background: tpl.color + '20', color: tpl.color }">{{ tpl.icon }}</span>
            <div class="tpl-info">
              <span class="tpl-name">{{ tpl.name }}</span>
              <span class="tpl-desc">{{ tpl.description }}</span>
            </div>
          </div>
        </div>
        <el-divider>{{ t('dw.orManual') }}</el-divider>
      </div>

      <el-form label-width="90px">
        <el-form-item :label="t('dw.imageName')" required>
          <el-autocomplete
            v-model="form.image"
            :fetch-suggestions="
              (q: string, cb: any) => cb(imageSuggestions.filter((s) => s.includes(q)).map((v) => ({ value: v })))
            "
            :placeholder="t('dw.imagePh')"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item :label="t('dw.containerName')">
          <el-input v-model="form.name" :placeholder="t('dw.namePh')" />
        </el-form-item>
      </el-form>
    </div>

    <!-- 步骤 1：基础配置 -->
    <div v-if="activeStep === 1" class="step-panel">
      <el-form label-width="110px">
        <el-form-item :label="t('dw.cmd')">
          <el-input v-model="form.cmd" :placeholder="t('dw.cmdPh')" />
        </el-form-item>
        <el-form-item :label="t('dw.cpuLimit')">
          <el-input-number v-model="form.cpuLimit" :min="0" :max="64" :step="0.5" :precision="1" />
          <span class="field-hint">{{ t('dw.cpuHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('dw.memLimit')">
          <el-input-number v-model="form.memoryLimitMB" :min="0" :step="256" />
          <span class="field-hint">{{ t('dw.memHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('dw.restartPolicy')">
          <el-select v-model="form.restartPolicy" style="width: 280px">
            <el-option v-for="o in restartOptions" :key="o.value" :value="o.value" :label="o.label" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('dw.privileged')">
          <el-switch v-model="form.privileged" />
          <span class="field-hint field-warn">⚠ {{ t('dw.privWarn') }}</span>
        </el-form-item>
        <el-form-item :label="t('dw.autoStart')">
          <el-switch v-model="form.autoStart" />
        </el-form-item>
      </el-form>
    </div>

    <!-- 步骤 2：网络端口 -->
    <div v-if="activeStep === 2" class="step-panel">
      <el-form label-width="110px">
        <el-form-item :label="t('dw.netMode')">
          <el-select v-model="form.networkMode" style="width: 280px">
            <el-option v-for="o in networkOptions" :key="o.value" :value="o.value" :label="o.label" />
          </el-select>
          <span v-if="form.networkMode === 'host'" class="field-hint">⚠ {{ t('dw.hostPortWarn') }}</span>
        </el-form-item>
        <el-form-item v-if="form.networkMode !== 'host'" :label="t('dw.portMap')">
          <div class="dynamic-rows">
            <div v-for="(p, i) in form.ports" :key="i" class="dyn-row">
              <el-input v-model="p.host" :placeholder="t('dw.hostPort')" class="dyn-input-sm" />
              <span class="dyn-arrow">→</span>
              <el-input v-model="p.container" :placeholder="t('dw.containerPort')" class="dyn-input-sm" />
              <el-select v-model="p.protocol" class="dyn-input-proto">
                <el-option value="tcp" label="TCP" />
                <el-option value="udp" label="UDP" />
              </el-select>
              <button class="dyn-remove" :aria-label="t('dw.removePort')" @click="removePort(i)">✕</button>
            </div>
            <button class="dyn-add" @click="addPort">+ {{ t('dw.addPort') }}</button>
          </div>
        </el-form-item>
      </el-form>
    </div>

    <!-- 步骤 3：存储卷 -->
    <div v-if="activeStep === 3" class="step-panel">
      <el-form label-width="110px">
        <el-form-item :label="t('dw.volMount')">
          <div class="dynamic-rows">
            <div v-for="(v, i) in form.volumes" :key="i" class="dyn-row">
              <el-input v-model="v.host" :placeholder="t('dw.hostPathPh')" class="dyn-input-lg" />
              <span class="dyn-arrow">→</span>
              <el-input v-model="v.container" :placeholder="t('dw.containerPathPh')" class="dyn-input-md" />
              <el-select v-model="v.mode" class="dyn-input-mode">
                <el-option value="rw" :label="t('dw.rw')" />
                <el-option value="ro" :label="t('dw.ro')" />
              </el-select>
              <button class="dyn-remove" :aria-label="t('dw.removeVol')" @click="removeVolume(i)">✕</button>
            </div>
            <button class="dyn-add" @click="addVolume">+ {{ t('dw.addVol') }}</button>
          </div>
        </el-form-item>
      </el-form>
    </div>

    <!-- 步骤 4：环境变量 -->
    <div v-if="activeStep === 4" class="step-panel">
      <el-form label-width="110px">
        <el-form-item :label="t('dw.envVars')">
          <div class="dynamic-rows">
            <div v-for="(e, i) in form.env" :key="i" class="dyn-row">
              <el-input v-model="e.key" :placeholder="t('dw.keyPh')" class="dyn-input-md" />
              <span class="dyn-arrow">=</span>
              <el-input v-model="e.value" :placeholder="t('dw.valuePh')" class="dyn-input-lg" />
              <button class="dyn-remove" :aria-label="t('dw.removeEnv')" @click="removeEnv(i)">✕</button>
            </div>
            <button class="dyn-add" @click="addEnv">+ {{ t('dw.addEnv') }}</button>
          </div>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>
