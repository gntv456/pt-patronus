<script setup lang="ts">
import type { MonitorTarget, OpsConfig, CommandRun } from '@/api/monitor'
import { useI18n } from 'vue-i18n'
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

// form 用 defineModel 双向绑定（父级 v-model:form）：表单字段修改经 update:form 回传。
const { t } = useI18n()

const form = defineModel<DeployForm>('form', { required: true })

defineProps<{
  visible: boolean
  configId: number | undefined
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
    :title="t('ndd.title')"
    size="560px"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="deploy-panel">
      <div class="deploy-note">
        {{ t('ndd.intro') }}
      </div>
      <el-form label-width="92px">
        <el-form-item :label="t('ndd.profile')">
          <div class="deploy-profile-row">
            <el-select
              :model-value="configId"
              clearable
              :placeholder="t('ndd.profilePh')"
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
            <el-button @click="emit('saveProfile')">{{ t('ndd.saveProfile') }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('ndd.servers')" required>
          <el-select v-model="form.targetIds" multiple collapse-tags collapse-tags-tooltip style="width: 100%">
            <el-option
              v-for="t in sshTargets"
              :key="t.id"
              :label="`${t.name} · ${targetAddr(t)}`"
              :value="t.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('ndd.repo')" required>
          <el-input v-model="form.repoUrl" placeholder="https://github.com/user/repo.git" />
        </el-form-item>
        <el-form-item :label="t('ndd.branch')">
          <el-input v-model="form.branch" placeholder="main" />
        </el-form-item>
        <el-form-item :label="t('ndd.projectDir')" required>
          <el-input v-model="form.projectDir" placeholder="/opt/app" />
        </el-form-item>
        <el-form-item :label="t('ndd.buildCmd')">
          <el-input v-model="form.buildCmd" type="textarea" :rows="3" resize="vertical" placeholder="npm install && npm run build" />
        </el-form-item>
        <el-form-item :label="t('ndd.serviceName')">
          <el-input v-model="form.serviceName" :placeholder="t('ndd.svcPh')" />
        </el-form-item>
        <el-form-item :label="t('ndd.backupDir')">
          <el-input v-model="form.backupDir" placeholder="/opt/backups" />
        </el-form-item>
        <el-form-item :label="t('ndd.gray')">
          <el-switch v-model="form.grayFirst" :active-text="t('ndd.grayOn')" :inactive-text="t('ndd.grayOff')" />
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
        <div class="deploy-preview-title">{{ t('ndd.previewTitle') }}</div>
        <pre>{{ deployCommand }}</pre>
      </div>
      <div class="deploy-actions">
        <el-button @click="emit('update:visible', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="warning" plain @click="emit('run', 'rollback')">{{ t('ndd.rollback') }}</el-button>
        <el-button type="primary" @click="emit('run', 'deploy')">{{ t('ndd.deploy') }}</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<style scoped src="../../views/nas-shared/deploy.css"></style>
