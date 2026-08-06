<script setup lang="ts">
import type { MonitorTarget, OpsConfig, CommandRun } from '@/api/monitor'
import { targetAddr, runDateText, runStatusText, runStatusType } from '@/views/nasServerHelpers'

export interface DeployForm {
  targetIds: number[]
  repoUrl: string
  branch: string
  projectDir: string
  buildCmd: string
  serviceName: string
  backupDir: string
  grayFirst: boolean
}

// form 是父级 composable 持有的响应式对象，子组件直接绑定其字段。
defineProps<{
  visible: boolean
  configId: number | undefined
  form: DeployForm
  profiles: OpsConfig[]
  runs: CommandRun[]
  steps: { title: string; desc: string }[]
  sshTargets: MonitorTarget[]
  deployCommand: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:configId', v: number | undefined): void
  (e: 'loadProfile'): void
  (e: 'saveProfile'): void
  (e: 'run', mode: 'deploy' | 'rollback'): void
  (e: 'showRun', run: CommandRun): void
}>()
</script>

<template>
  <el-drawer
    :model-value="visible"
    title="发布部署"
    size="560px"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="deploy-panel">
      <div class="deploy-note">
        部署会先备份当前目录，再拉取 Git 仓库、执行构建命令并重启服务。开启灰度时只先部署选择列表里的第一台。
      </div>
      <el-form label-width="92px">
        <el-form-item label="模板">
          <div class="deploy-profile-row">
            <el-select
              :model-value="configId"
              clearable
              placeholder="选择已保存的发布流程"
              style="width: 100%"
              @update:model-value="emit('update:configId', $event)"
              @change="emit('loadProfile')"
            >
              <el-option
                v-for="p in profiles"
                :key="p.id"
                :label="`${p.name} · ${p.env || 'default'}`"
                :value="p.id"
              />
            </el-select>
            <el-button @click="emit('saveProfile')">保存模板</el-button>
          </div>
        </el-form-item>
        <el-form-item label="服务器" required>
          <el-select v-model="form.targetIds" multiple collapse-tags collapse-tags-tooltip style="width: 100%">
            <el-option
              v-for="t in sshTargets"
              :key="t.id"
              :label="`${t.name} · ${targetAddr(t)}`"
              :value="t.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="Git 仓库" required>
          <el-input v-model="form.repoUrl" placeholder="https://github.com/user/repo.git" />
        </el-form-item>
        <el-form-item label="分支">
          <el-input v-model="form.branch" placeholder="main" />
        </el-form-item>
        <el-form-item label="项目目录" required>
          <el-input v-model="form.projectDir" placeholder="/opt/app" />
        </el-form-item>
        <el-form-item label="构建命令">
          <el-input v-model="form.buildCmd" type="textarea" :rows="3" resize="vertical" placeholder="npm install && npm run build" />
        </el-form-item>
        <el-form-item label="服务名">
          <el-input v-model="form.serviceName" placeholder="ptpatronus.service，可留空" />
        </el-form-item>
        <el-form-item label="备份目录">
          <el-input v-model="form.backupDir" placeholder="/opt/backups" />
        </el-form-item>
        <el-form-item label="灰度">
          <el-switch v-model="form.grayFirst" active-text="先跑一台" inactive-text="全部执行" />
        </el-form-item>
      </el-form>
      <div class="deploy-step-list compact">
        <div v-for="(step, index) in steps" :key="step.title" class="deploy-step">
          <b>{{ index + 1 }}</b>
          <span>
            <strong>{{ step.title }}</strong>
            <small>{{ step.desc }}</small>
          </span>
        </div>
      </div>
      <div class="deploy-preview">
        <div class="deploy-preview-title">将执行的部署脚本</div>
        <pre>{{ deployCommand }}</pre>
      </div>
      <div class="deploy-actions">
        <el-button @click="emit('update:visible', false)">取消</el-button>
        <el-button type="warning" plain @click="emit('run', 'rollback')">回滚上一版</el-button>
        <el-button type="primary" @click="emit('run', 'deploy')">开始部署</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<style scoped src="../../views/nas-shared/deploy.css"></style>
