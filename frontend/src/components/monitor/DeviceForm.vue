<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import type { MonitorTarget, OpsCredential, TargetInput } from '@/api/monitor'
import { useDeviceOpsConfig } from '@/composables/useDeviceOpsConfig'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// DeviceForm — 新增/编辑监控目标对话框。按 type 动态展示连接字段；编辑时密码留空表示不改。
const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ target?: MonitorTarget | null; defaultType?: string; credentials?: OpsCredential[] }>()
const emit = defineEmits<{ submit: [TargetInput] }>()

function blank(): TargetInput {
  return {
    name: '', type: 'host', protocol: 'local', enabled: true,
    interval_sec: 60, timeout_sec: 15, vendor: '', host: '', port: 0,
    credential_id: 0, username: '', password: '', private_key: '', rtsp_url: '', thresholds: '', disk_paths: '',
  }
}
const form = ref<TargetInput>(blank())
const isEdit = computed(() => !!props.target)
// 可选配置（HTTP 方法/进程名/脚本命令等）与 thresholds 序列化抽出为 composable（useDeviceOpsConfig）。
const { opsConfig, loadOpsConfig, buildThresholds } = useDeviceOpsConfig()

function defaultProtocol(type: string) {
  if (type === 'camera') return 'onvif'
  if (type === 'http') return 'http'
  if (type === 'tcp') return 'tcp'
  if (type === 'process' || type === 'script') return 'ssh'
  return 'local'
}

function inferredProtocol(t: MonitorTarget) {
  if (t.protocol) return t.protocol
  if (t.type === 'http') return 'http'
  if (t.type === 'tcp') return 'tcp'
  if (t.type === 'process' || t.type === 'script') return 'ssh'
  if (t.type === 'host') return t.host ? 'ssh' : 'local'
  if (t.type === 'nas') return t.host ? 'api' : 'local'
  return defaultProtocol(t.type)
}

function normalizeNasFields() {
  if (form.value.type !== 'nas') return
  if (form.value.protocol === 'local' || form.value.protocol === 'agent') {
    form.value.vendor = ''
  } else if (form.value.protocol === 'snmp') {
    form.value.vendor = 'snmp'
  } else if (!form.value.vendor || form.value.vendor === 'snmp') {
    form.value.vendor = 'synology'
  }
}

watch(visible, (v) => {
  if (!v) return
  if (props.target) {
    form.value = {
      name: props.target.name, type: props.target.type, protocol: inferredProtocol(props.target),
      vendor: props.target.vendor, host: props.target.host, port: props.target.port,
      rtsp_url: props.target.rtsp_url, interval_sec: props.target.interval_sec,
      timeout_sec: props.target.timeout_sec, enabled: props.target.enabled,
      thresholds: props.target.thresholds, credential_id: props.target.credential_id || 0,
      disk_paths: props.target.disk_paths || '',
      username: props.target.username || '', password: '', private_key: '',
    }
    loadOpsConfig(props.target.thresholds)
    if (form.value.type === 'nas' && form.value.vendor === 'snmp' && form.value.protocol !== 'local') {
      form.value.protocol = 'snmp'
    }
    normalizeNasFields()
  } else {
    form.value = blank()
    if (props.defaultType) {
      form.value.type = props.defaultType
      form.value.protocol = defaultProtocol(props.defaultType)
      normalizeNasFields()
    }
    loadOpsConfig(form.value.thresholds)
  }
})

// 切换类型时重置协议默认值
watch(() => form.value.type, (t) => {
  if (isEdit.value) return
  form.value.protocol = defaultProtocol(t)
  form.value.vendor = ''
  normalizeNasFields()
  loadOpsConfig(form.value.thresholds)
})

watch(() => form.value.protocol, () => normalizeNasFields())
watch(() => [form.value.type, form.value.protocol], () => {
  if (!canUseCredential.value) form.value.credential_id = 0
})

const isLocal = computed(() => form.value.protocol === 'local')
const isAgent = computed(() => form.value.protocol === 'agent')
const showHost = computed(() => !(isLocal.value || isAgent.value))
const showPort = computed(() => showHost.value && form.value.type !== 'http')
const lockAgentProtocol = computed(() => !!props.target && inferredProtocol(props.target) === 'agent')
const canUseCredential = computed(() => {
  if (form.value.type === 'tcp') return false
  if (form.value.type === 'http' || form.value.type === 'process' || form.value.type === 'script') return true
  return showHost.value || form.value.type === 'camera'
})
const usingCredential = computed(() => canUseCredential.value && !!form.value.credential_id)
const credentialOptions = computed(() => {
  const all = props.credentials || []
  if ((form.value.type === 'host' && form.value.protocol === 'ssh') || form.value.type === 'process' || form.value.type === 'script') return all.filter((c) => c.type === 'ssh' || c.type === 'generic')
  if (form.value.type === 'nas') return all.filter((c) => c.type === 'nas' || c.type === 'generic')
  return all.filter((c) => c.type === 'generic')
})
// 仅 SSH 远程主机需要私钥（其它协议走密码/token/SNMP）
const showPrivateKey = computed(() => !usingCredential.value && ((form.value.type === 'host' && form.value.protocol === 'ssh') || form.value.type === 'process' || form.value.type === 'script'))
const hasPrivateKey = computed(() => !!props.target?.has_private_key)
const showInlineCredential = computed(() => !usingCredential.value && canUseCredential.value)
const hostLabel = computed(() => form.value.type === 'http' ? 'URL' : t('deviceform.address'))
const hostPlaceholder = computed(() => {
  if (form.value.type === 'http') return 'https://example.com/health'
  if (form.value.type === 'tcp') return '192.168.1.10'
  if (form.value.type === 'process' || form.value.type === 'script') return '192.168.1.3'
  return form.value.type === 'nas' ? 'http://192.168.1.2:5000' : '192.168.1.3'
})

function submit() {
  if (!form.value.name.trim()) {
    ElMessage.warning(t('deviceform.nameRequired'))
    return
  }
  if (form.value.type === 'http' && !/^https?:\/\//i.test(form.value.host || '')) {
    ElMessage.warning(t('deviceform.urlRequired'))
    return
  }
  if (form.value.type === 'tcp' && (!(form.value.host || '').trim() || !form.value.port)) {
    ElMessage.warning(t('deviceform.tcpRequired'))
    return
  }
  if (form.value.type === 'process' && !opsConfig.value.process_name.trim()) {
    ElMessage.warning(t('deviceform.processRequired'))
    return
  }
  if (form.value.type === 'script' && !opsConfig.value.script_command.trim()) {
    ElMessage.warning(t('deviceform.scriptRequired'))
    return
  }
  const out = { ...form.value }
  if (!showHost.value) out.host = ''
  if (!showPort.value) out.port = 0
  if (!canUseCredential.value) out.credential_id = 0
  if (out.credential_id) {
    out.username = ''
    out.password = ''
    out.private_key = ''
  }
  if (out.type === 'nas' && (out.protocol === 'local' || out.protocol === 'agent')) out.vendor = ''
  if (out.type === 'nas' && out.protocol === 'snmp') out.vendor = 'snmp'
  if (out.type === 'process' || out.type === 'script') out.protocol = 'ssh'
  out.thresholds = buildThresholds(out.thresholds, out.type)
  emit('submit', out)
  visible.value = false
}
</script>


<template>
  <el-dialog v-model="visible" :title="isEdit ? t('deviceform.edit') : t('deviceform.add')" width="480px">
    <el-form label-width="88px" label-position="right">
      <el-form-item :label="t('common.name')" required>
        <el-input v-model="form.name" :placeholder="t('deviceform.namePh')" />
      </el-form-item>
      <el-form-item :label="t('cloud.type')">
        <el-select v-model="form.type" :disabled="isEdit" style="width: 100%">
          <el-option :label="t('deviceform.type.host')" value="host" />
          <el-option label="NAS" value="nas" />
          <el-option :label="t('deviceform.type.camera')" value="camera" />
          <el-option :label="t('deviceform.type.http')" value="http" />
          <el-option :label="t('deviceform.type.tcp')" value="tcp" />
          <el-option :label="t('deviceform.type.process')" value="process" />
          <el-option :label="t('deviceform.type.script')" value="script" />
        </el-select>
      </el-form-item>

      <template v-if="form.type === 'host'">
        <el-form-item :label="t('deviceform.protocol')">
          <el-select v-model="form.protocol" :disabled="lockAgentProtocol" style="width: 100%">
            <el-option :label="t('deviceform.proto.local')" value="local" />
            <el-option :label="t('deviceform.proto.ssh')" value="ssh" />
            <el-option :label="t('deviceform.proto.agent')" value="agent" />
          </el-select>
        </el-form-item>
      </template>
      <template v-if="form.type === 'nas'">
        <el-form-item :label="t('deviceform.protocol')">
          <el-select v-model="form.protocol" :disabled="lockAgentProtocol" style="width: 100%">
            <el-option :label="t('deviceform.proto.local')" value="local" />
            <el-option :label="t('deviceform.proto.agentNas')" value="agent" />
            <el-option :label="t('deviceform.proto.api')" value="api" />
            <el-option :label="t('deviceform.proto.snmp')" value="snmp" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.protocol === 'api'" :label="t('deviceform.vendor')">
          <el-select v-model="form.vendor" style="width: 100%">
            <el-option :label="t('deviceform.vendorName.synology')" value="synology" />
            <el-option :label="t('deviceform.vendorName.truenas')" value="truenas" />
          </el-select>
        </el-form-item>
      </template>
      <template v-if="form.type === 'camera'">
        <el-form-item :label="t('deviceform.protocol')">
          <el-select v-model="form.protocol" style="width: 100%">
            <el-option label="ONVIF" value="onvif" />
            <el-option :label="t('deviceform.proto.rtsp')" value="rtsp" />
          </el-select>
        </el-form-item>
      </template>

      <el-form-item v-if="isLocal && (form.type === 'host' || form.type === 'nas')" :label="t('deviceform.hostDiskPaths')">
        <el-input v-model="form.disk_paths" type="textarea" :rows="2"
          :placeholder="t('deviceform.hostDiskPh')" />
        <div style="font-size: var(--fs-xs); color: var(--el-text-color-secondary); line-height: 1.5; margin-top: 4px">
          {{ t('deviceform.hostDiskHint') }}
        </div>
      </el-form-item>

      <el-form-item v-if="showHost" :label="hostLabel">
        <el-input v-model="form.host" :placeholder="hostPlaceholder" />
      </el-form-item>
      <el-form-item v-if="showPort" :label="t('deviceform.port')">
        <el-input-number v-model="form.port" :min="0" :max="65535" controls-position="right" style="width: 100%" />
      </el-form-item>

      <el-form-item v-if="form.type === 'camera' && form.protocol === 'rtsp'" :label="t('deviceform.rtspUrl')">
        <el-input v-model="form.rtsp_url" placeholder="rtsp://user:pass@192.168.1.10:554/stream" />
      </el-form-item>

      <el-form-item v-if="canUseCredential" :label="t('deviceform.credential')">
        <el-select v-model="form.credential_id" clearable :placeholder="t('deviceform.noCredential')" style="width: 100%">
          <el-option :value="0" :label="t('deviceform.noCredential')" />
          <el-option v-for="c in credentialOptions" :key="c.id" :label="c.name" :value="c.id">
            <span>{{ c.name }}</span>
            <span class="cred-sub">{{ c.username || t('deviceform.noUsername') }}{{ c.has_private_key ? ' · ' + t('deviceform.privateKey') : '' }}{{ c.has_password ? ' · ' + t('deviceform.password') : '' }}</span>
          </el-option>
        </el-select>
      </el-form-item>

      <el-form-item v-if="showInlineCredential" :label="t('deviceform.username')">
        <el-input v-model="form.username" :placeholder="form.type === 'nas' && form.vendor === 'snmp' ? 'SNMP community' : t('deviceform.account')" />
      </el-form-item>
      <el-form-item v-if="showInlineCredential" :label="t('deviceform.password')">
        <el-input v-model="form.password" type="password" show-password :placeholder="isEdit ? t('deviceform.keepIfEmpty') : t('deviceform.secretPh')" />
      </el-form-item>
      <el-form-item v-if="showPrivateKey" :label="t('deviceform.privateKey')">
        <el-input
          v-model="form.private_key"
          type="textarea"
          :rows="6"
          resize="vertical"
          :placeholder="hasPrivateKey ? t('deviceform.keyKeepPh') : t('deviceform.keyPh')"
        />
      </el-form-item>

      <template v-if="form.type === 'http'">
        <el-form-item :label="t('deviceform.httpMethod')">
          <el-select v-model="opsConfig.http_method" style="width: 100%">
            <el-option label="GET" value="GET" />
            <el-option label="HEAD" value="HEAD" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('deviceform.statusCode')">
          <div class="range-row">
            <el-input-number v-model="opsConfig.http_expected_min" :min="100" :max="599" controls-position="right" />
            <span>{{ t('deviceform.to') }}</span>
            <el-input-number v-model="opsConfig.http_expected_max" :min="100" :max="599" controls-position="right" />
          </div>
        </el-form-item>
        <el-form-item :label="t('deviceform.respThreshold')">
          <el-input-number v-model="opsConfig.response_ms" :min="0" :step="100" controls-position="right" style="width: 100%" />
          <span class="hint">{{ t('deviceform.msHint') }}</span>
        </el-form-item>
      </template>

      <template v-if="form.type === 'tcp'">
        <el-form-item :label="t('deviceform.respThreshold')">
          <el-input-number v-model="opsConfig.response_ms" :min="0" :step="100" controls-position="right" style="width: 100%" />
          <span class="hint">{{ t('deviceform.msHint') }}</span>
        </el-form-item>
      </template>

      <template v-if="form.type === 'process'">
        <el-form-item :label="t('deviceform.processName')" required>
          <el-input v-model="opsConfig.process_name" placeholder="nginx / qbittorrent-nox / ptpatronus" />
        </el-form-item>
        <el-form-item :label="t('deviceform.minCount')">
          <el-input-number v-model="opsConfig.process_min_count" :min="1" controls-position="right" style="width: 100%" />
        </el-form-item>
      </template>

      <template v-if="form.type === 'script'">
        <el-form-item :label="t('deviceform.scriptCmd')" required>
          <el-input v-model="opsConfig.script_command" type="textarea" :rows="4" resize="vertical" placeholder="curl -fsS http://127.0.0.1:8080/health" />
        </el-form-item>
        <el-form-item :label="t('deviceform.timeThreshold')">
          <el-input-number v-model="opsConfig.response_ms" :min="0" :step="100" controls-position="right" style="width: 100%" />
          <span class="hint">{{ t('deviceform.msHint') }}</span>
        </el-form-item>
      </template>

      <el-form-item :label="t('deviceform.interval')">
        <el-input-number v-model="form.interval_sec" :min="10" :step="10" controls-position="right" style="width: 100%" />
        <span class="hint">{{ t('deviceform.sec') }}</span>
      </el-form-item>
      <el-form-item :label="t('deviceform.timeout')">
        <el-input-number v-model="form.timeout_sec" :min="3" controls-position="right" style="width: 100%" />
        <span class="hint">{{ t('deviceform.timeoutHint') }}</span>
      </el-form-item>
      <el-form-item :label="t('common.enabled')">
        <el-switch v-model="form.enabled" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" @click="submit">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint {
  margin-left: 8px;
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.cred-sub {
  float: right;
  color: var(--text-muted);
  font-size: var(--fs-xs);
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.range-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: var(--sp-2);
  width: 100%;
}
.range-row :deep(.el-input-number) {
  width: 100%;
}
</style>
