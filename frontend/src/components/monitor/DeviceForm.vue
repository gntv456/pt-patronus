<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import type { MonitorTarget, OpsCredential, TargetInput } from '@/api/monitor'

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
const opsConfig = ref({
  http_method: 'GET',
  http_expected_min: 200,
  http_expected_max: 399,
  response_ms: 0,
  process_name: '',
  process_min_count: 1,
  script_command: '',
})

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
  if (form.value.protocol === 'local') {
    form.value.vendor = ''
  } else if (form.value.protocol === 'snmp') {
    form.value.vendor = 'snmp'
  } else if (!form.value.vendor || form.value.vendor === 'snmp') {
    form.value.vendor = 'synology'
  }
}

function parseThresholds(raw?: string): Record<string, unknown> {
  if (!raw) return {}
  try {
    const v = JSON.parse(raw)
    return v && typeof v === 'object' ? v : {}
  } catch {
    return {}
  }
}

function loadOpsConfig(raw?: string) {
  const cfg = parseThresholds(raw)
  opsConfig.value = {
    http_method: String(cfg.http_method || 'GET').toUpperCase(),
    http_expected_min: Number(cfg.http_expected_min || 200),
    http_expected_max: Number(cfg.http_expected_max || 399),
    response_ms: Number(cfg.response_ms || 0),
    process_name: String(cfg.process_name || ''),
    process_min_count: Number(cfg.process_min_count || 1),
    script_command: String(cfg.script_command || ''),
  }
}

function buildThresholds(raw: string | undefined, type: string): string {
  const cfg = parseThresholds(raw)
  for (const key of [
    'http_method',
    'http_expected_min',
    'http_expected_max',
    'response_ms',
    'process_name',
    'process_min_count',
    'script_command',
  ]) {
    delete cfg[key]
  }
  if (type === 'http') {
    cfg.http_method = opsConfig.value.http_method || 'GET'
    cfg.http_expected_min = Number(opsConfig.value.http_expected_min || 200)
    cfg.http_expected_max = Number(opsConfig.value.http_expected_max || 399)
    if (Number(opsConfig.value.response_ms) > 0) cfg.response_ms = Number(opsConfig.value.response_ms)
  } else if (type === 'tcp') {
    if (Number(opsConfig.value.response_ms) > 0) cfg.response_ms = Number(opsConfig.value.response_ms)
  } else if (type === 'process') {
    cfg.process_name = opsConfig.value.process_name.trim()
    cfg.process_min_count = Number(opsConfig.value.process_min_count || 1)
  } else if (type === 'script') {
    cfg.script_command = opsConfig.value.script_command.trim()
    if (Number(opsConfig.value.response_ms) > 0) cfg.response_ms = Number(opsConfig.value.response_ms)
  }
  return Object.keys(cfg).length ? JSON.stringify(cfg) : ''
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
const showHost = computed(() => !(isLocal.value || (form.value.type === 'host' && form.value.protocol === 'agent')))
const showPort = computed(() => showHost.value && form.value.type !== 'http')
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
const hostLabel = computed(() => form.value.type === 'http' ? 'URL' : '地址')
const hostPlaceholder = computed(() => {
  if (form.value.type === 'http') return 'https://example.com/health'
  if (form.value.type === 'tcp') return '192.168.1.10'
  if (form.value.type === 'process' || form.value.type === 'script') return '192.168.1.3'
  return form.value.type === 'nas' ? 'http://192.168.1.2:5000' : '192.168.1.3'
})

function submit() {
  if (!form.value.name.trim()) {
    ElMessage.warning('请填写名称')
    return
  }
  if (form.value.type === 'http' && !/^https?:\/\//i.test(form.value.host || '')) {
    ElMessage.warning('请填写 http/https 完整 URL')
    return
  }
  if (form.value.type === 'tcp' && (!(form.value.host || '').trim() || !form.value.port)) {
    ElMessage.warning('请填写 TCP 地址和端口')
    return
  }
  if (form.value.type === 'process' && !opsConfig.value.process_name.trim()) {
    ElMessage.warning('请填写进程名称')
    return
  }
  if (form.value.type === 'script' && !opsConfig.value.script_command.trim()) {
    ElMessage.warning('请填写脚本命令')
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
  if (out.type === 'nas' && out.protocol === 'local') out.vendor = ''
  if (out.type === 'nas' && out.protocol === 'snmp') out.vendor = 'snmp'
  if (out.type === 'process' || out.type === 'script') out.protocol = 'ssh'
  out.thresholds = buildThresholds(out.thresholds, out.type)
  emit('submit', out)
  visible.value = false
}
</script>

<template>
  <el-dialog v-model="visible" :title="isEdit ? '编辑设备' : '添加设备'" width="480px">
    <el-form label-width="88px" label-position="right">
      <el-form-item label="名称" required>
        <el-input v-model="form.name" placeholder="如：客厅 NAS / 主服务器" />
      </el-form-item>
      <el-form-item label="类型">
        <el-select v-model="form.type" :disabled="isEdit" style="width: 100%">
          <el-option label="服务器" value="host" />
          <el-option label="NAS" value="nas" />
          <el-option label="摄像头" value="camera" />
          <el-option label="网站监控" value="http" />
          <el-option label="端口监控" value="tcp" />
          <el-option label="进程监控" value="process" />
          <el-option label="脚本监控" value="script" />
        </el-select>
      </el-form-item>

      <template v-if="form.type === 'host'">
        <el-form-item label="协议">
          <el-select v-model="form.protocol" style="width: 100%">
            <el-option label="本机（gopsutil）" value="local" />
            <el-option label="SSH 远程" value="ssh" />
            <el-option label="Agent 上报" value="agent" />
          </el-select>
        </el-form-item>
      </template>
      <template v-if="form.type === 'nas'">
        <el-form-item label="协议">
          <el-select v-model="form.protocol" style="width: 100%">
            <el-option label="本机（gopsutil）" value="local" />
            <el-option label="厂商 API" value="api" />
            <el-option label="通用 SNMP" value="snmp" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.protocol === 'api'" label="厂商">
          <el-select v-model="form.vendor" style="width: 100%">
            <el-option label="群晖 Synology" value="synology" />
            <el-option label="TrueNAS / FreeNAS" value="truenas" />
          </el-select>
        </el-form-item>
      </template>
      <template v-if="form.type === 'camera'">
        <el-form-item label="协议">
          <el-select v-model="form.protocol" style="width: 100%">
            <el-option label="ONVIF" value="onvif" />
            <el-option label="RTSP 直连" value="rtsp" />
          </el-select>
        </el-form-item>
      </template>

      <el-form-item v-if="isLocal && form.type === 'host'" label="宿主磁盘路径">
        <el-input v-model="form.disk_paths" type="textarea" :rows="2"
          placeholder="每行一个容器内路径，如 /mnt/nas&#10;需先在 docker-compose 挂载该目录（-v /mnt:/mnt:ro）" />
        <div style="font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.5; margin-top: 4px">
          Docker 部署时容器看不到宿主磁盘：把宿主目录挂进容器（compose 加 <code>-v /mnt:/mnt:ro</code>）再在此填对应路径，采集穿透 bind mount 读真实空间。
        </div>
      </el-form-item>

      <el-form-item v-if="showHost" :label="hostLabel">
        <el-input v-model="form.host" :placeholder="hostPlaceholder" />
      </el-form-item>
      <el-form-item v-if="showPort" label="端口">
        <el-input-number v-model="form.port" :min="0" :max="65535" controls-position="right" style="width: 100%" />
      </el-form-item>

      <el-form-item v-if="form.type === 'camera' && form.protocol === 'rtsp'" label="RTSP 地址">
        <el-input v-model="form.rtsp_url" placeholder="rtsp://user:pass@192.168.1.10:554/stream" />
      </el-form-item>

      <el-form-item v-if="canUseCredential" label="统一凭据">
        <el-select v-model="form.credential_id" clearable placeholder="不使用统一凭据" style="width: 100%">
          <el-option :value="0" label="不使用统一凭据" />
          <el-option v-for="c in credentialOptions" :key="c.id" :label="c.name" :value="c.id">
            <span>{{ c.name }}</span>
            <span class="cred-sub">{{ c.username || '无用户名' }}{{ c.has_private_key ? ' · 私钥' : '' }}{{ c.has_password ? ' · 密码' : '' }}</span>
          </el-option>
        </el-select>
      </el-form-item>

      <el-form-item v-if="showInlineCredential" label="用户名">
        <el-input v-model="form.username" :placeholder="form.type === 'nas' && form.vendor === 'snmp' ? 'SNMP community' : '账号'" />
      </el-form-item>
      <el-form-item v-if="showInlineCredential" label="密码">
        <el-input v-model="form.password" type="password" show-password :placeholder="isEdit ? '留空不改' : '密码 / Token / API Key'" />
      </el-form-item>
      <el-form-item v-if="showPrivateKey" label="私钥">
        <el-input
          v-model="form.private_key"
          type="textarea"
          :rows="6"
          resize="vertical"
          :placeholder="hasPrivateKey ? '已配置私钥，留空则保留；粘贴新私钥则覆盖' : '粘贴 SSH 私钥（PEM，-----BEGIN ... KEY-----）；留空则用密码认证'"
        />
      </el-form-item>

      <template v-if="form.type === 'http'">
        <el-form-item label="请求方法">
          <el-select v-model="opsConfig.http_method" style="width: 100%">
            <el-option label="GET" value="GET" />
            <el-option label="HEAD" value="HEAD" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态码">
          <div class="range-row">
            <el-input-number v-model="opsConfig.http_expected_min" :min="100" :max="599" controls-position="right" />
            <span>至</span>
            <el-input-number v-model="opsConfig.http_expected_max" :min="100" :max="599" controls-position="right" />
          </div>
        </el-form-item>
        <el-form-item label="响应阈值">
          <el-input-number v-model="opsConfig.response_ms" :min="0" :step="100" controls-position="right" style="width: 100%" />
          <span class="hint">毫秒，0 表示不告警</span>
        </el-form-item>
      </template>

      <template v-if="form.type === 'tcp'">
        <el-form-item label="响应阈值">
          <el-input-number v-model="opsConfig.response_ms" :min="0" :step="100" controls-position="right" style="width: 100%" />
          <span class="hint">毫秒，0 表示不告警</span>
        </el-form-item>
      </template>

      <template v-if="form.type === 'process'">
        <el-form-item label="进程名称" required>
          <el-input v-model="opsConfig.process_name" placeholder="nginx / qbittorrent-nox / ptpatronus" />
        </el-form-item>
        <el-form-item label="最少数量">
          <el-input-number v-model="opsConfig.process_min_count" :min="1" controls-position="right" style="width: 100%" />
        </el-form-item>
      </template>

      <template v-if="form.type === 'script'">
        <el-form-item label="脚本命令" required>
          <el-input v-model="opsConfig.script_command" type="textarea" :rows="4" resize="vertical" placeholder="curl -fsS http://127.0.0.1:8080/health" />
        </el-form-item>
        <el-form-item label="耗时阈值">
          <el-input-number v-model="opsConfig.response_ms" :min="0" :step="100" controls-position="right" style="width: 100%" />
          <span class="hint">毫秒，0 表示不告警</span>
        </el-form-item>
      </template>

      <el-form-item label="采集间隔">
        <el-input-number v-model="form.interval_sec" :min="10" :step="10" controls-position="right" style="width: 100%" />
        <span class="hint">秒</span>
      </el-form-item>
      <el-form-item label="超时">
        <el-input-number v-model="form.timeout_sec" :min="3" controls-position="right" style="width: 100%" />
        <span class="hint">秒（SSH 建议 ≥3）</span>
      </el-form-item>
      <el-form-item label="启用">
        <el-switch v-model="form.enabled" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint {
  margin-left: 8px;
  font-size: 12px;
  color: var(--text-muted);
}
.cred-sub {
  float: right;
  color: var(--text-muted);
  font-size: 12px;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.range-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  width: 100%;
}
.range-row :deep(.el-input-number) {
  width: 100%;
}
</style>
