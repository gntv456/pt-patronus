<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import type { MonitorTarget, TargetInput } from '@/api/monitor'

// DeviceForm — 新增/编辑监控目标对话框。按 type 动态展示连接字段；编辑时密码留空表示不改。
const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ target?: MonitorTarget | null; defaultType?: string }>()
const emit = defineEmits<{ submit: [TargetInput] }>()

function blank(): TargetInput {
  return {
    name: '', type: 'host', protocol: 'local', enabled: true,
    interval_sec: 60, timeout_sec: 15, vendor: '', host: '', port: 0,
    username: '', password: '', private_key: '', rtsp_url: '', thresholds: '',
  }
}
const form = ref<TargetInput>(blank())
const isEdit = computed(() => !!props.target)

watch(visible, (v) => {
  if (!v) return
  if (props.target) {
    form.value = {
      name: props.target.name, type: props.target.type, protocol: props.target.protocol,
      vendor: props.target.vendor, host: props.target.host, port: props.target.port,
      rtsp_url: props.target.rtsp_url, interval_sec: props.target.interval_sec,
      timeout_sec: props.target.timeout_sec, enabled: props.target.enabled,
      thresholds: props.target.thresholds, username: props.target.username || '', password: '', private_key: '',
    }
  } else {
    form.value = blank()
    if (props.defaultType) {
      form.value.type = props.defaultType
      form.value.protocol = props.defaultType === 'nas' ? 'api' : props.defaultType === 'camera' ? 'onvif' : 'local'
    }
  }
})

// 切换类型时重置协议默认值
watch(() => form.value.type, (t) => {
  if (isEdit.value) return
  form.value.protocol = t === 'nas' ? 'api' : t === 'camera' ? 'onvif' : 'local'
  form.value.vendor = t === 'nas' ? 'synology' : ''
})

const showHost = computed(() => !(form.value.type === 'host' && (form.value.protocol === 'local' || form.value.protocol === 'agent')))
// 仅 SSH 远程主机需要私钥（其它协议走密码/token/SNMP）
const showPrivateKey = computed(() => form.value.type === 'host' && form.value.protocol === 'ssh')
const hasPrivateKey = computed(() => !!props.target?.has_private_key)

function submit() {
  if (!form.value.name.trim()) {
    ElMessage.warning('请填写名称')
    return
  }
  const out = { ...form.value }
  if (!showHost.value) out.host = ''
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
        <el-form-item label="厂商">
          <el-select v-model="form.vendor" style="width: 100%">
            <el-option label="群晖 Synology" value="synology" />
            <el-option label="TrueNAS / FreeNAS" value="truenas" />
            <el-option label="通用 SNMP" value="snmp" />
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

      <el-form-item v-if="showHost" label="地址">
        <el-input v-model="form.host" :placeholder="form.type === 'nas' ? 'http://192.168.1.2:5000' : '192.168.1.3'" />
      </el-form-item>
      <el-form-item v-if="showHost" label="端口">
        <el-input-number v-model="form.port" :min="0" :max="65535" controls-position="right" style="width: 100%" />
      </el-form-item>

      <el-form-item v-if="form.type === 'camera' && form.protocol === 'rtsp'" label="RTSP 地址">
        <el-input v-model="form.rtsp_url" placeholder="rtsp://user:pass@192.168.1.10:554/stream" />
      </el-form-item>

      <el-form-item v-if="showHost || form.type === 'camera'" label="用户名">
        <el-input v-model="form.username" :placeholder="form.type === 'nas' && form.vendor === 'snmp' ? 'SNMP community' : '账号'" />
      </el-form-item>
      <el-form-item v-if="showHost || form.type === 'camera'" label="密码">
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
</style>
