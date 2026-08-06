// NasServer 发布部署 —— 从 NasServer.vue 抽出的部署簇。
// 耦合最重:读 config 的 opsConfigs、驱动的 exec 的执行能力、组件的 sshTargets/选中目标/面板滚动/工作区切换,
// 故用依赖对象显式注入。deployForm/Visible/ConfigId 及 deployProfiles/runs/steps 由本 composable 持有。
import { ref, computed, nextTick, type ComputedRef, type Ref } from 'vue'
import { ElMessage } from 'element-plus'
import { opsConfigApi, type CommandRun, type MonitorTarget, type OpsConfig, type OpsConfigInput } from '@/api/monitor'
import { basenameOf, shQuote } from './nasServerHelpers'

type DeployForm = {
  targetIds: number[]
  repoUrl: string
  branch: string
  projectDir: string
  buildCmd: string
  serviceName: string
  backupDir: string
  grayFirst: boolean
}

export function useDeploy(deps: {
  sshTargets: ComputedRef<MonitorTarget[]>
  selectedExecTargets: ComputedRef<MonitorTarget[]>
  opsConfigs: Ref<OpsConfig[]>
  commandRuns: Ref<CommandRun[]>
  execTargetIds: Ref<number[]>
  execCommand: Ref<string>
  execTimeout: Ref<number>
  runBatchCommand: (source?: string, note?: string) => Promise<void>
  focusExecPanel: () => void
  enterDeployWorkspace: () => void
}) {
  const deployVisible = ref(false)
  const deployConfigId = ref<number | undefined>(undefined)
  const deployForm = ref<DeployForm>({
    targetIds: [],
    repoUrl: '',
    branch: 'main',
    projectDir: '/opt/app',
    buildCmd: '',
    serviceName: '',
    backupDir: '/opt/backups',
    grayFirst: true,
  })

  const deployProfiles = computed(() => deps.opsConfigs.value.filter((c) => c.key.startsWith('deploy.') && c.format === 'json'))
  const deployRuns = computed(() => deps.commandRuns.value.filter((run) => run.source === 'deploy'))
  const deploySteps = computed(() => [
    { title: '备份当前目录', desc: deployForm.value.backupDir || '/opt/backups' },
    { title: '拉取代码', desc: `${deployForm.value.branch || 'main'} · ${deployForm.value.repoUrl || '未填写仓库'}` },
    { title: '执行构建', desc: deployForm.value.buildCmd.trim() || '跳过构建' },
    { title: '重启服务', desc: deployForm.value.serviceName.trim() || '未配置服务名' },
    { title: '完成记录', desc: deployForm.value.grayFirst ? '灰度：先执行第一台' : '全部服务器执行' },
  ])

  function openDeployPanel() {
    deps.enterDeployWorkspace()
    deployForm.value.targetIds = deps.selectedExecTargets.value.length
      ? deps.selectedExecTargets.value.map((t) => t.id)
      : deps.sshTargets.value.filter((t) => t.last_online !== false).slice(0, 1).map((t) => t.id)
    deployVisible.value = true
  }

  function loadDeployProfile(id: number | undefined) {
    const profile = deployProfiles.value.find((p) => p.id === id)
    if (!profile) return
    try {
      const parsed = JSON.parse(profile.value) as Partial<DeployForm>
      deployForm.value = {
        ...deployForm.value,
        ...parsed,
        targetIds: Array.isArray(parsed.targetIds) ? parsed.targetIds : deployForm.value.targetIds,
        grayFirst: parsed.grayFirst !== false,
      }
    } catch {
      ElMessage.error('部署模板 JSON 格式错误')
    }
  }

  async function saveDeployProfile() {
    if (!deployForm.value.repoUrl.trim() || !deployForm.value.projectDir.trim()) {
      ElMessage.warning('请先填写 Git 仓库和项目目录')
      return
    }
    const keyBase = basenameOf(deployForm.value.projectDir).replace(/[^\w.-]+/g, '-').toLowerCase() || 'app'
    const input: OpsConfigInput = {
      key: `deploy.${keyBase}`,
      name: `${keyBase} 发布流程`,
      env: deployForm.value.branch || 'main',
      format: 'json',
      value: JSON.stringify(deployForm.value, null, 2),
      tags: '部署,发布',
      note: 'NAS 运维控制台发布部署模板',
    }
    try {
      if (deployConfigId.value) await opsConfigApi.update(deployConfigId.value, input)
      else {
        const created = await opsConfigApi.create(input)
        deployConfigId.value = created.id
      }
      deps.opsConfigs.value = await opsConfigApi.list()
      ElMessage.success('发布模板已保存')
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '保存发布模板失败')
    }
  }

  function deployTargets() {
    const ids = deployForm.value.grayFirst ? deployForm.value.targetIds.slice(0, 1) : deployForm.value.targetIds
    return ids.filter((id) => deps.sshTargets.value.some((t) => t.id === id))
  }

  function buildDeployCommand() {
    const f = deployForm.value
    const app = shQuote(f.projectDir)
    const backup = shQuote(f.backupDir)
    const appName = shQuote(basenameOf(f.projectDir))
    const branch = shQuote(f.branch || 'main')
    const repo = shQuote(f.repoUrl)
    const build = f.buildCmd.trim()
    const service = f.serviceName.trim()
    return [
      'set -e',
      `APP_DIR=${app}`,
      `BACKUP_DIR=${backup}`,
      `APP_NAME=${appName}`,
      'TS=$(date +%Y%m%d%H%M%S)',
      'mkdir -p "$BACKUP_DIR"',
      'if [ -d "$APP_DIR" ]; then tar -czf "$BACKUP_DIR/$APP_NAME-$TS.tgz" -C "$(dirname "$APP_DIR")" "$APP_NAME"; fi',
      `if [ ! -d "$APP_DIR/.git" ]; then mkdir -p "$(dirname "$APP_DIR")"; git clone --branch ${branch} ${repo} "$APP_DIR"; else cd "$APP_DIR"; git fetch --all; git checkout ${branch}; git pull --ff-only origin ${branch}; fi`,
      'cd "$APP_DIR"',
      build ? build : 'echo "跳过构建"',
      service ? `sudo -n systemctl restart ${shQuote(service)}` : 'echo "未配置服务名，跳过重启"',
      'echo "部署完成：$APP_DIR"',
    ].join('\n')
  }

  function buildRollbackCommand() {
    const f = deployForm.value
    const app = shQuote(f.projectDir)
    const backup = shQuote(f.backupDir)
    const appName = shQuote(basenameOf(f.projectDir))
    const service = f.serviceName.trim()
    return [
      'set -e',
      `APP_DIR=${app}`,
      `BACKUP_DIR=${backup}`,
      `APP_NAME=${appName}`,
      'LATEST=$(ls -1t "$BACKUP_DIR"/"$APP_NAME"-*.tgz 2>/dev/null | head -n1)',
      'if [ -z "$LATEST" ]; then echo "没有可回滚的备份"; exit 1; fi',
      service ? `sudo -n systemctl stop ${shQuote(service)} || true` : 'echo "未配置服务名，跳过停止服务"',
      'rm -rf "$APP_DIR"',
      'mkdir -p "$(dirname "$APP_DIR")"',
      'tar -xzf "$LATEST" -C "$(dirname "$APP_DIR")"',
      service ? `sudo -n systemctl start ${shQuote(service)}` : 'echo "未配置服务名，跳过启动服务"',
      'echo "已回滚到：$LATEST"',
    ].join('\n')
  }

  async function runDeployCommand(kind: 'deploy' | 'rollback') {
    if (!deployForm.value.targetIds.length) {
      ElMessage.warning('请选择部署服务器')
      return
    }
    if (!deployForm.value.projectDir.trim()) {
      ElMessage.warning('请填写项目目录')
      return
    }
    if (kind === 'deploy' && !deployForm.value.repoUrl.trim()) {
      ElMessage.warning('请填写 Git 仓库地址')
      return
    }
    const ids = deployTargets()
    if (!ids.length) {
      ElMessage.warning('没有可部署的 SSH 主机')
      return
    }
    deps.execTargetIds.value = ids
    deps.execCommand.value = kind === 'deploy' ? buildDeployCommand() : buildRollbackCommand()
    if (deps.execTimeout.value < 300) deps.execTimeout.value = 300
    deployVisible.value = false
    deps.focusExecPanel()
    await nextTick()
    await deps.runBatchCommand('deploy', kind === 'deploy' ? '发布部署' : '回滚上一版')
  }

  return {
    deployVisible,
    deployConfigId,
    deployForm,
    deployProfiles,
    deployRuns,
    deploySteps,
    openDeployPanel,
    loadDeployProfile,
    saveDeployProfile,
    runDeployCommand,
    buildDeployCommand,
    buildRollbackCommand,
  }
}
