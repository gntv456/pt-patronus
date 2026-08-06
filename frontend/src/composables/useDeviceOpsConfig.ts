import { ref } from 'vue'

// 监控目标的「可选配置」(opsConfig) 与 thresholds JSON 的序列化/反序列化。
// thresholds 字段同时承载阈值（磁盘/CPU/内存）和 ops 配置（HTTP 方法/进程名/脚本命令等），
// 抽出为 composable 收敛 DeviceForm.vue 脚本体积。
export function useDeviceOpsConfig() {
  const opsConfig = ref({
    http_method: 'GET',
    http_expected_min: 200,
    http_expected_max: 399,
    response_ms: 0,
    process_name: '',
    process_min_count: 1,
    script_command: '',
  })

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

  return { opsConfig, loadOpsConfig, buildThresholds }
}
