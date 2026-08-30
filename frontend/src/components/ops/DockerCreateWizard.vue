<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'
import { type DockerTemplate } from '@/data/dockerTemplates'
import DockerWizardSteps from './DockerWizardSteps.vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  images?: { Repository: string; Tag: string }[]
  initialTemplate?: DockerTemplate | null
}>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; created: [] }>()

const activeStep = ref(0)
const loading = ref(false)
const showTemplates = ref(true)
const selectedTemplate = ref<string>('')

interface PortRow {
  host: number | string
  container: number | string
  protocol: 'tcp' | 'udp'
}
interface VolumeRow {
  host: string
  container: string
  mode: 'rw' | 'ro'
}
interface EnvRow {
  key: string
  value: string
}

const form = ref({
  image: '',
  name: '',
  cmd: '',
  cpuLimit: 0,
  memoryLimitMB: 0,
  privileged: false,
  networkMode: 'bridge',
  restartPolicy: 'unless-stopped',
  autoStart: true,
  ports: [] as PortRow[],
  volumes: [] as VolumeRow[],
  env: [] as EnvRow[],
})

const steps = computed(() => [
  t('dw.step.image'),
  t('dw.step.basic'),
  t('dw.step.net'),
  t('dw.step.vol'),
  t('dw.step.env'),
  t('dw.step.confirm'),
])
// confirm 面板展示重启策略标签用；表单选项与行操作在 DockerWizardSteps.vue
const restartOptions = computed(() => [
  { value: 'no', label: t('dw.restart.no') },
  { value: 'always', label: t('dw.restart.always') },
  { value: 'unless-stopped', label: t('dw.restart.unless') },
  { value: 'on-failure', label: t('dw.restart.onFailure') },
])

function resetForm() {
  activeStep.value = 0
  showTemplates.value = true
  selectedTemplate.value = ''
  form.value = {
    image: '',
    name: '',
    cmd: '',
    cpuLimit: 0,
    memoryLimitMB: 0,
    privileged: false,
    networkMode: 'bridge',
    restartPolicy: 'unless-stopped',
    autoStart: true,
    ports: [],
    volumes: [],
    env: [],
  }
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      resetForm()
      // 如果从应用商店传入模板，自动应用
      if (props.initialTemplate) {
        nextTick(() => {
          applyTemplate(props.initialTemplate!)
        })
      }
    }
  },
)

function applyTemplate(tpl: DockerTemplate) {
  selectedTemplate.value = tpl.id
  showTemplates.value = false
  form.value.image = tpl.image
  form.value.name = tpl.id
  form.value.cpuLimit = tpl.cpuLimit
  form.value.memoryLimitMB = tpl.memoryLimitMB
  form.value.privileged = tpl.privileged
  form.value.networkMode = tpl.networkMode
  form.value.restartPolicy = tpl.restartPolicy
  form.value.ports = tpl.ports.map((p) => ({ host: p.host, container: p.container, protocol: p.protocol }))
  form.value.volumes = tpl.volumes.map((v) => ({ host: v.host, container: v.container, mode: v.mode }))
  form.value.env = tpl.env.map((e) => ({ key: e.key, value: e.value }))
  activeStep.value = 1
}

function next() {
  if (activeStep.value < steps.value.length - 1) activeStep.value++
}
function prev() {
  if (activeStep.value > 0) activeStep.value--
}

const canNextImage = computed(() => form.value.image.trim().length > 0)

async function submit() {
  if (!form.value.image.trim()) {
    ElMessage.warning(t('dw.imageRequired'))
    return
  }
  loading.value = true
  try {
    const env: Record<string, string> = {}
    form.value.env
      .filter((e) => e.key.trim())
      .forEach((e) => {
        env[e.key.trim()] = e.value
      })
    const ports = form.value.ports
      .filter((p) => p.container !== '' && p.container !== null)
      .map((p) => ({
        container: parseInt(String(p.container)) || 0,
        host: parseInt(String(p.host)) || 0,
        protocol: p.protocol || 'tcp',
      }))
    const binds = form.value.volumes
      .filter((v) => v.host.trim() && v.container.trim())
      .map((v) => `${v.host}:${v.container}:${v.mode}`)
    const cmd = form.value.cmd.trim() ? form.value.cmd.trim().split(/\s+/) : undefined
    const res = await dockerApi.localCreate({
      image: form.value.image.trim(),
      name: form.value.name.trim() || undefined,
      cmd,
      env,
      ports,
      binds,
      restart_policy: form.value.restartPolicy,
      auto_start: form.value.autoStart,
      cpu_limit: form.value.cpuLimit || undefined,
      memory_limit_mb: form.value.memoryLimitMB || undefined,
      privileged: form.value.privileged || undefined,
      network_mode: form.value.networkMode !== 'bridge' ? form.value.networkMode : undefined,
    })
    if (res.ok) {
      ElMessage.success(t('dw.created'))
      emit('update:visible', false)
      emit('created')
    } else ElMessage.error(res.error || t('dw.createFailed'))
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    :title="t('dw.title')"
    width="720px"
    top="5vh"
    class="create-wizard-dialog"
  >
    <!-- 步骤指示器 -->
    <el-steps :active="activeStep" finish-status="success" align-center class="wizard-steps">
      <el-step v-for="(s, i) in steps" :key="i" :title="s" />
    </el-steps>

    <DockerWizardSteps
      :active-step="activeStep"
      :form="form"
      :images="images"
      :selected-template="selectedTemplate"
      :show-templates="showTemplates"
      @apply-template="applyTemplate"
    />

    <!-- 步骤 5：确认创建 -->
    <div v-if="activeStep === 5" class="step-panel">
      <div class="confirm-section">
        <div class="confirm-row">
          <span class="ck">{{ t('opsdocker.images') }}</span
          ><code>{{ form.image }}</code>
        </div>
        <div class="confirm-row">
          <span class="ck">{{ t('common.name') }}</span
          ><span>{{ form.name || t('dw.auto') }}</span>
        </div>
        <div class="confirm-row">
          <span class="ck">CPU</span
          ><span>{{ form.cpuLimit ? t('dw.cores', { n: form.cpuLimit }) : t('dw.unlimited') }}</span>
        </div>
        <div class="confirm-row">
          <span class="ck">{{ t('opsdocker.mem') }}</span
          ><span>{{ form.memoryLimitMB ? form.memoryLimitMB + ' MB' : t('dw.unlimited') }}</span>
        </div>
        <div class="confirm-row">
          <span class="ck">{{ t('opsdocker.networks') }}</span
          ><span>{{ form.networkMode }}</span>
        </div>
        <div class="confirm-row">
          <span class="ck">{{ t('dw.restartPolicy') }}</span
          ><span>{{ restartOptions.find((o) => o.value === form.restartPolicy)?.label }}</span>
        </div>
        <div class="confirm-row">
          <span class="ck">{{ t('dw.privileged') }}</span
          ><span>{{ form.privileged ? '⚠ ' + t('common.yes') : t('common.no') }}</span>
        </div>
        <div v-if="form.cmd" class="confirm-row">
          <span class="ck">{{ t('dw.cmd') }}</span
          ><code>{{ form.cmd }}</code>
        </div>
        <div v-if="form.ports.length" class="confirm-row">
          <span class="ck">{{ t('opsdocker.ports') }}</span>
          <span v-for="(p, i) in form.ports" :key="i" class="confirm-chip"
            >{{ p.host }}→{{ p.container }}/{{ p.protocol }}</span
          >
        </div>
        <div v-if="form.volumes.length" class="confirm-row">
          <span class="ck">{{ t('opsdocker.volumes') }}</span>
          <span v-for="(v, i) in form.volumes" :key="i" class="confirm-chip"
            >{{ v.host }}→{{ v.container }}:{{ v.mode }}</span
          >
        </div>
        <div v-if="form.env.length" class="confirm-row">
          <span class="ck">{{ t('dw.envVars') }}</span>
          <span v-for="(e, i) in form.env" :key="i" class="confirm-chip">{{ e.key }}={{ e.value }}</span>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="emit('update:visible', false)">{{ t('common.cancel') }}</el-button>
      <el-button v-if="activeStep > 0" @click="prev">{{ t('dw.prev') }}</el-button>
      <el-button
        v-if="activeStep < steps.length - 1"
        type="primary"
        :disabled="activeStep === 0 && !canNextImage"
        @click="next"
        >{{ t('dw.next') }}</el-button
      >
      <el-button v-if="activeStep === steps.length - 1" type="primary" :loading="loading" @click="submit">{{
        t('dw.createGo')
      }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped src="./dockerWizard.css"></style>
