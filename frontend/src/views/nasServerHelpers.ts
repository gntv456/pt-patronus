// NasServer.vue 抽出的纯函数 / 数据常量 / 共享类型（零组件状态耦合）。
// 这些不引用任何 ref/computed，仅依输入求值，便于单测与复用。
import type { CommandRun, CommandRunTarget, MonitorTarget } from '@/api/monitor'

export type ExecStatus = 'running' | 'success' | 'error'

export type ExecLog = {
  id: number
  targetId: number
  targetName: string
  command: string
  status: ExecStatus
  time: string
  stdout: string
  stderr: string
  error: string
  durationMs: number
}

export type CommandRisk = { level: 'low' | 'medium' | 'high'; reason: string }

// 高危命令规则（命中即高风险，需“我确认”二次确认）。
export const highRiskRules = [
  { reason: '删除根目录或系统目录', re: /\brm\s+-[^\n;|&]*r[^\n;|&]*f[^\n;|&]*\s+\/(?:\s|$|[;&|])/i },
  { reason: '格式化或擦除磁盘', re: /\b(mkfs|wipefs)\b/i },
  { reason: '向磁盘设备写入原始数据', re: /\bdd\s+.*\bof=\/dev\//i },
  { reason: '电源操作', re: /\b(shutdown|reboot|poweroff|halt)\b/i },
  { reason: '递归放开根目录权限', re: /\bchmod\s+-R\s+777\s+\//i },
  { reason: '递归修改根目录属主', re: /\bchown\s+-R\s+\S+\s+\//i },
]

// 中危命令规则（需普通确认）。
export const mediumRiskRules = [
  { reason: '递归强制删除', re: /\brm\s+-[^\n;|&]*r[^\n;|&]*f/i },
  { reason: '停止或重启服务', re: /\bsystemctl\s+(stop|restart)\b/i },
  { reason: '清理容器或镜像', re: /\bdocker\s+(rm|rmi|system\s+prune)\b/i },
  { reason: '数据库破坏性操作', re: /\b(drop\s+database|truncate\s+table)\b/i },
]

// 快捷命令（执行面板一键填充）。
export const quickCommands = [
  { label: '系统负载', command: 'uptime && free -h' },
  { label: '磁盘空间', command: 'df -h' },
  { label: 'Docker', command: 'docker ps --format "table {{.Names}}\\t{{.Status}}\\t{{.Ports}}"' },
  { label: '服务状态', command: 'systemctl status ptpatronus --no-pager' },
]

// 资产/监控 Tab 定义（name 用 as const 保留字面量类型供 computed 过滤）。
export const tabs = [
  { name: 'host' as const, label: '服务器', icon: 'server', color: ['#0f2027', '#2c5364'] },
  { name: 'nas' as const, label: 'NAS', icon: 'nas', color: ['#f7971e', '#ffd200'] },
  { name: 'camera' as const, label: '摄像头', icon: 'camera', color: ['#ff5f6d', '#ffc371'] },
  { name: 'http' as const, label: '网站', icon: 'sites', color: ['#00b894', '#0984e3'] },
  { name: 'tcp' as const, label: '端口', icon: 'settings', color: ['#34495e', '#16a085'] },
  { name: 'process' as const, label: '进程', icon: 'workflow', color: ['#8e44ad', '#3498db'] },
  { name: 'script' as const, label: '脚本', icon: 'history', color: ['#d35400', '#f1c40f'] },
]

// shQuote 单引号安全转义（部署脚本拼参用）。
export function shQuote(v: string) {
  return `'${String(v || '').replace(/'/g, `'\\''`)}'`
}

// basenameOf 取路径末段（默认 app）。
export function basenameOf(path: string) {
  return (
    (path || '/opt/app')
      .replace(/[\\/]+$/, '')
      .split(/[\\/]/)
      .pop() || 'app'
  )
}

export function nowTime() {
  return new Date().toLocaleTimeString('zh-CN', { hour12: false })
}

export function runTimeText(value?: string | null) {
  return value ? new Date(value).toLocaleTimeString('zh-CN', { hour12: false }) : nowTime()
}

export function runDateText(value?: string | null) {
  return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-'
}

export function runStatusText(status: string) {
  return { success: '成功', partial: '部分失败', failed: '失败', running: '运行中' }[status] || status
}

export function runStatusType(status: string) {
  if (status === 'success') return 'success'
  if (status === 'running') return 'info'
  if (status === 'partial') return 'warning'
  return 'danger'
}

export function runSourceText(source: string) {
  return { batch: '批量执行', deploy: '发布部署', manual: '手动', terminal: '终端' }[source] || source
}

export function logStatus(status: string): ExecStatus {
  if (status === 'success') return 'success'
  if (status === 'running') return 'running'
  return 'error'
}

// commandRisk 按规则集判定命令风险等级（high > medium > low）。
export function commandRisk(command: string): CommandRisk {
  for (const rule of highRiskRules) {
    if (rule.re.test(command)) return { level: 'high', reason: rule.reason }
  }
  for (const rule of mediumRiskRules) {
    if (rule.re.test(command)) return { level: 'medium', reason: rule.reason }
  }
  return { level: 'low', reason: '' }
}

export function credentialTypeText(type: string) {
  return { ssh: 'SSH', nas: 'NAS', generic: '通用' }[type] || type
}

export function configFormatText(format: string) {
  return { kv: 'KV', text: '文本', json: 'JSON' }[format] || format
}

// isSSHHost 判定是否 SSH 可控主机（终端/批量执行前置）。
export function isSSHHost(t: MonitorTarget) {
  return t.type === 'host' && (t.protocol === 'ssh' || (!t.protocol && !!t.host))
}

export function targetAddr(t: MonitorTarget) {
  const port = t.port || 22
  return `${t.host}:${port}`
}

export function targetState(t: MonitorTarget) {
  if (t.last_online === true) return '在线'
  if (t.last_online === false) return '离线'
  return '未检测'
}

// execLogsFromRun 把一次 CommandRun 的各目标结果展开为执行日志行。
export function execLogsFromRun(run: CommandRun): ExecLog[] {
  return (run.targets || []).map((target: CommandRunTarget) => ({
    id: target.id,
    targetId: target.target_id,
    targetName: target.target_name,
    command: run.command,
    status: logStatus(target.status),
    time: runTimeText(target.finished_at || target.started_at),
    stdout: target.stdout || '',
    stderr: target.stderr || '',
    error: target.error || '',
    durationMs: target.duration_ms || 0,
  }))
}

// 页面顶层类型（从 NasServer.vue 移入）。
export type MonitorTab = 'host' | 'nas' | 'camera' | 'http' | 'tcp' | 'process' | 'script'
export type Workspace = 'asset' | 'execute' | 'monitor' | 'deploy'
