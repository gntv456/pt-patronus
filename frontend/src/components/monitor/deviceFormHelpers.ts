// DeviceForm 的纯工具函数：空白表单/协议推断/NAS 字段规整（从 DeviceForm.vue 拆出）。
import type { MonitorTarget, TargetInput } from '@/api/monitor'

export function blank(): TargetInput {
  return {
    name: '',
    type: 'host',
    protocol: 'local',
    enabled: true,
    interval_sec: 60,
    timeout_sec: 15,
    vendor: '',
    host: '',
    port: 0,
    credential_id: 0,
    username: '',
    password: '',
    private_key: '',
    rtsp_url: '',
    thresholds: '',
    disk_paths: '',
  }
}

export function defaultProtocol(type: string) {
  if (type === 'camera') return 'onvif'
  if (type === 'http') return 'http'
  if (type === 'tcp') return 'tcp'
  if (type === 'process' || type === 'script') return 'ssh'
  return 'local'
}

export function inferredProtocol(t: MonitorTarget) {
  if (t.protocol) return t.protocol
  if (t.type === 'http') return 'http'
  if (t.type === 'tcp') return 'tcp'
  if (t.type === 'process' || t.type === 'script') return 'ssh'
  if (t.type === 'host') return t.host ? 'ssh' : 'local'
  if (t.type === 'nas') return t.host ? 'api' : 'local'
  return defaultProtocol(t.type)
}

export function normalizeNasFields(form: { value: TargetInput }) {
  if (form.value.type !== 'nas') return
  if (form.value.protocol === 'local' || form.value.protocol === 'agent') {
    form.value.vendor = ''
  } else if (form.value.protocol === 'snmp') {
    form.value.vendor = 'snmp'
  } else if (!form.value.vendor || form.value.vendor === 'snmp') {
    form.value.vendor = 'synology'
  }
}
