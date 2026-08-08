<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'
import { dockerTemplates, type DockerTemplate } from '@/data/dockerTemplates'

const props = defineProps<{ visible: boolean; images?: { Repository: string; Tag: string }[]; initialTemplate?: DockerTemplate | null }>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; created: [] }>()

const activeStep = ref(0)
const loading = ref(false)
const showTemplates = ref(true)
const selectedTemplate = ref<string>('')

interface PortRow { host: number | string; container: number | string; protocol: 'tcp' | 'udp' }
interface VolumeRow { host: string; container: string; mode: 'rw' | 'ro' }
interface EnvRow { key: string; value: string }

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

const steps = ['选择镜像', '基础配置', '网络端口', '存储卷', '环境变量', '确认创建']
const networkOptions = [
  { value: 'bridge', label: 'Bridge（默认桥接）' },
  { value: 'host', label: 'Host（主机网络）' },
  { value: 'none', label: 'None（无网络）' },
]
const restartOptions = [
  { value: 'no', label: '不自动重启' },
  { value: 'always', label: '总是重启' },
  { value: 'unless-stopped', label: '除非手动停止（推荐）' },
  { value: 'on-failure', label: '仅失败时重启' },
]

const imageSuggestions = computed(() => {
  if (!props.images) return []
  return props.images.map((i) => `${i.Repository}:${i.Tag}`).filter((s) => !s.includes('<none>'))
})

function resetForm() {
  activeStep.value = 0
  showTemplates.value = true
  selectedTemplate.value = ''
  form.value = {
    image: '', name: '', cmd: '', cpuLimit: 0, memoryLimitMB: 0, privileged: false,
    networkMode: 'bridge', restartPolicy: 'unless-stopped', autoStart: true,
    ports: [], volumes: [], env: [],
  }
}

watch(() => props.visible, (v) => {
    if (v) {
      resetForm()
      // 如果从应用商店传入模板，自动应用
      if (props.initialTemplate) {
        nextTick(() => {
          applyTemplate(props.initialTemplate!)
        })
      }
    }
  })

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

function addPort() { form.value.ports.push({ host: '', container: '', protocol: 'tcp' }) }
function removePort(i: number) { form.value.ports.splice(i, 1) }
function addVolume() { form.value.volumes.push({ host: '', container: '', mode: 'rw' }) }
function removeVolume(i: number) { form.value.volumes.splice(i, 1) }
function addEnv() { form.value.env.push({ key: '', value: '' }) }
function removeEnv(i: number) { form.value.env.splice(i, 1) }

function next() { if (activeStep.value < steps.length - 1) activeStep.value++ }
function prev() { if (activeStep.value > 0) activeStep.value-- }

const canNextImage = computed(() => form.value.image.trim().length > 0)

async function submit() {
  if (!form.value.image.trim()) { ElMessage.warning('请输入镜像名'); return }
  loading.value = true
  try {
    const env: Record<string, string> = {}
    form.value.env.filter((e) => e.key.trim()).forEach((e) => { env[e.key.trim()] = e.value })
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
    if (res.ok) { ElMessage.success('容器已创建'); emit('update:visible', false); emit('created') }
    else ElMessage.error(res.error || '创建失败')
  } catch (e: any) { ElMessage.error(e.message) }
  finally { loading.value = false }
}
</script>

<template>
  <el-dialog :model-value="visible" @update:model-value="emit('update:visible', $event)"
    title="创建容器" width="720px" top="5vh" class="create-wizard-dialog">
    <!-- 步骤指示器 -->
    <el-steps :active="activeStep" finish-status="success" align-center class="wizard-steps">
      <el-step v-for="(s, i) in steps" :key="i" :title="s" />
    </el-steps>

    <div class="wizard-body">
      <!-- 步骤 0：选择镜像 -->
      <div v-if="activeStep === 0" class="step-panel">
        <div v-if="showTemplates" class="template-section">
          <div class="section-title">从应用模板创建 <span class="section-hint">（推荐小白用户）</span></div>
          <div class="template-grid">
            <div v-for="tpl in dockerTemplates" :key="tpl.id" class="template-card"
              :class="{ selected: selectedTemplate === tpl.id }" @click="applyTemplate(tpl)">
              <span class="tpl-icon" :style="{ background: tpl.color + '20', color: tpl.color }">{{ tpl.icon }}</span>
              <div class="tpl-info">
                <span class="tpl-name">{{ tpl.name }}</span>
                <span class="tpl-desc">{{ tpl.description }}</span>
              </div>
            </div>
          </div>
          <el-divider>或手动输入</el-divider>
        </div>

        <el-form label-width="90px">
          <el-form-item label="镜像名" required>
            <el-autocomplete v-model="form.image" :fetch-suggestions="(q: string, cb: any) => cb(imageSuggestions.filter(s => s.includes(q)).map(v => ({ value: v })))"
              placeholder="nginx:latest 或 linuxserver/qbittorrent:latest" style="width: 100%" />
          </el-form-item>
          <el-form-item label="容器名称">
            <el-input v-model="form.name" placeholder="my-container（可选，留空自动生成）" />
          </el-form-item>
        </el-form>
      </div>

      <!-- 步骤 1：基础配置 -->
      <div v-if="activeStep === 1" class="step-panel">
        <el-form label-width="110px">
          <el-form-item label="启动命令">
            <el-input v-model="form.cmd" placeholder="覆盖默认命令（可选，如 nginx -g 'daemon off;'）" />
          </el-form-item>
          <el-form-item label="CPU 限制">
            <el-input-number v-model="form.cpuLimit" :min="0" :max="64" :step="0.5" :precision="1" />
            <span class="field-hint">核数，0 = 不限制</span>
          </el-form-item>
          <el-form-item label="内存限制">
            <el-input-number v-model="form.memoryLimitMB" :min="0" :step="256" />
            <span class="field-hint">MB，0 = 不限制</span>
          </el-form-item>
          <el-form-item label="重启策略">
            <el-select v-model="form.restartPolicy" style="width: 280px">
              <el-option v-for="o in restartOptions" :key="o.value" :value="o.value" :label="o.label" />
            </el-select>
          </el-form-item>
          <el-form-item label="特权模式">
            <el-switch v-model="form.privileged" />
            <span class="field-hint field-warn">⚠ 特权模式赋予容器 root 权限，请谨慎使用</span>
          </el-form-item>
          <el-form-item label="创建后启动">
            <el-switch v-model="form.autoStart" />
          </el-form-item>
        </el-form>
      </div>

      <!-- 步骤 2：网络端口 -->
      <div v-if="activeStep === 2" class="step-panel">
        <el-form label-width="110px">
          <el-form-item label="网络模式">
            <el-select v-model="form.networkMode" style="width: 280px">
              <el-option v-for="o in networkOptions" :key="o.value" :value="o.value" :label="o.label" />
            </el-select>
            <span v-if="form.networkMode === 'host'" class="field-hint">⚠ Host 模式下端口映射无效</span>
          </el-form-item>
          <el-form-item v-if="form.networkMode !== 'host'" label="端口映射">
            <div class="dynamic-rows">
              <div v-for="(p, i) in form.ports" :key="i" class="dyn-row">
                <el-input v-model="p.host" placeholder="主机端口" class="dyn-input-sm" />
                <span class="dyn-arrow">→</span>
                <el-input v-model="p.container" placeholder="容器端口" class="dyn-input-sm" />
                <el-select v-model="p.protocol" class="dyn-input-proto">
                  <el-option value="tcp" label="TCP" />
                  <el-option value="udp" label="UDP" />
                </el-select>
                <button class="dyn-remove" @click="removePort(i)">✕</button>
              </div>
              <button class="dyn-add" @click="addPort">+ 添加端口映射</button>
            </div>
          </el-form-item>
        </el-form>
      </div>

      <!-- 步骤 3：存储卷 -->
      <div v-if="activeStep === 3" class="step-panel">
        <el-form label-width="110px">
          <el-form-item label="卷挂载">
            <div class="dynamic-rows">
              <div v-for="(v, i) in form.volumes" :key="i" class="dyn-row">
                <el-input v-model="v.host" placeholder="宿主机路径 /opt/app/config" class="dyn-input-lg" />
                <span class="dyn-arrow">→</span>
                <el-input v-model="v.container" placeholder="容器路径 /config" class="dyn-input-md" />
                <el-select v-model="v.mode" class="dyn-input-mode">
                  <el-option value="rw" label="读写" />
                  <el-option value="ro" label="只读" />
                </el-select>
                <button class="dyn-remove" @click="removeVolume(i)">✕</button>
              </div>
              <button class="dyn-add" @click="addVolume">+ 添加卷挂载</button>
            </div>
          </el-form-item>
        </el-form>
      </div>

      <!-- 步骤 4：环境变量 -->
      <div v-if="activeStep === 4" class="step-panel">
        <el-form label-width="110px">
          <el-form-item label="环境变量">
            <div class="dynamic-rows">
              <div v-for="(e, i) in form.env" :key="i" class="dyn-row">
                <el-input v-model="e.key" placeholder="变量名 KEY" class="dyn-input-md" />
                <span class="dyn-arrow">=</span>
                <el-input v-model="e.value" placeholder="值 value" class="dyn-input-lg" />
                <button class="dyn-remove" @click="removeEnv(i)">✕</button>
              </div>
              <button class="dyn-add" @click="addEnv">+ 添加环境变量</button>
            </div>
          </el-form-item>
        </el-form>
      </div>

      <!-- 步骤 5：确认创建 -->
      <div v-if="activeStep === 5" class="step-panel">
        <div class="confirm-section">
          <div class="confirm-row"><span class="ck">镜像</span><code>{{ form.image }}</code></div>
          <div class="confirm-row"><span class="ck">名称</span><span>{{ form.name || '(自动)' }}</span></div>
          <div class="confirm-row"><span class="ck">CPU</span><span>{{ form.cpuLimit ? form.cpuLimit + ' 核' : '不限制' }}</span></div>
          <div class="confirm-row"><span class="ck">内存</span><span>{{ form.memoryLimitMB ? form.memoryLimitMB + ' MB' : '不限制' }}</span></div>
          <div class="confirm-row"><span class="ck">网络</span><span>{{ form.networkMode }}</span></div>
          <div class="confirm-row"><span class="ck">重启策略</span><span>{{ restartOptions.find(o => o.value === form.restartPolicy)?.label }}</span></div>
          <div class="confirm-row"><span class="ck">特权</span><span>{{ form.privileged ? '⚠ 是' : '否' }}</span></div>
          <div v-if="form.cmd" class="confirm-row"><span class="ck">命令</span><code>{{ form.cmd }}</code></div>
          <div v-if="form.ports.length" class="confirm-row"><span class="ck">端口</span>
            <span v-for="(p, i) in form.ports" :key="i" class="confirm-chip">{{ p.host }}→{{ p.container }}/{{ p.protocol }}</span>
          </div>
          <div v-if="form.volumes.length" class="confirm-row"><span class="ck">卷</span>
            <span v-for="(v, i) in form.volumes" :key="i" class="confirm-chip">{{ v.host }}→{{ v.container }}:{{ v.mode }}</span>
          </div>
          <div v-if="form.env.length" class="confirm-row"><span class="ck">环境</span>
            <span v-for="(e, i) in form.env" :key="i" class="confirm-chip">{{ e.key }}={{ e.value }}</span>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="emit('update:visible', false)">取消</el-button>
      <el-button v-if="activeStep > 0" @click="prev">上一步</el-button>
      <el-button v-if="activeStep < steps.length - 1" type="primary" :disabled="activeStep === 0 && !canNextImage" @click="next">下一步</el-button>
      <el-button v-if="activeStep === steps.length - 1" type="primary" :loading="loading" @click="submit">创建容器</el-button>
    </template>
  </el-dialog>
</template>

<style scoped src="./dockerWizard.css"></style>
