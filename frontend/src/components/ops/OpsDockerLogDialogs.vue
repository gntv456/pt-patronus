<script setup lang="ts">
// OpsDocker 的日志抽屉 + Compose 部署/拉取镜像对话框（从 OpsDocker.vue 拆出）。
import { useI18n } from 'vue-i18n'

defineModel<boolean>('logDrawer', { default: false })
defineModel<string>('logContainer', { default: '' })
defineModel<string>('logContent', { default: '' })
defineModel<boolean>('logLoading', { default: false })
defineModel<boolean>('composeVisible', { default: false })
defineModel<string>('composePath', { default: '' })
defineModel<string>('composeOutput', { default: '' })
defineModel<boolean>('composeLoading', { default: false })
defineModel<boolean>('pullDialog', { default: false })
defineModel<string>('pullRef', { default: '' })
defineModel<string>('pullResult', { default: '' })
defineModel<boolean>('pullLoading', { default: false })

const emit = defineEmits<{
  'update:composePath': [v: string]
  'update:pullRef': [v: string]
  'update:logDrawer': [v: boolean]
  'update:composeVisible': [v: boolean]
  'update:pullDialog': [v: boolean]
  composeAction: [kind: 'up' | 'down']
  doPull: []
}>()

const { t } = useI18n()
</script>

<template>
    <!-- 日志抽屉 -->
    <el-drawer :model-value="logDrawer" @update:model-value="emit('update:logDrawer', $event)" :title="t('opsdocker.logsTitle', { name: logContainer })" size="60%" direction="rtl">
      <div :v-loading="!!logLoading" class="log-viewer">
        <pre>{{ logContent }}</pre>
      </div>
    </el-drawer>

    <!-- Compose 部署对话框 -->
    <el-dialog :model-value="composeVisible" @update:model-value="emit('update:composeVisible', $event)" :title="t('opsdocker.composeDeploy')" width="500px">
      <el-form label-width="60px"
        ><el-form-item :label="t('opsdocker.dir')"
          ><el-input :model-value="composePath" @update:model-value="emit('update:composePath', $event)" placeholder="/opt/myapp" /></el-form-item
      ></el-form>
      <div v-if="composeOutput" class="compose-output">
        <pre>{{ composeOutput }}</pre>
      </div>
      <template #footer>
        <el-button @click="emit('update:composeVisible', false)">{{ t('common.close') }}</el-button>
        <el-button type="danger" :loading="composeLoading" @click="emit('composeAction', 'down')">Compose Down</el-button>
        <el-button type="primary" :loading="composeLoading" @click="emit('composeAction', 'up')">Compose Up</el-button>
      </template>
    </el-dialog>

    <!-- 拉取镜像对话框 -->
    <el-dialog :model-value="pullDialog" @update:model-value="emit('update:pullDialog', $event)" :title="t('opsdocker.pullImage')" width="500px">
      <el-form label-width="70px"
        ><el-form-item :label="t('opsdocker.imageName')"
          ><el-input
            :model-value="pullRef" @update:model-value="emit('update:pullRef', $event)"
            placeholder="nginx / redis:7 / ghcr.io/..."
            @keyup.enter="emit('doPull')" /></el-form-item
      ></el-form>
      <div v-if="pullResult" class="pull-output">
        <pre>{{ pullResult }}</pre>
      </div>
      <template #footer>
        <el-button @click="emit('update:pullDialog', false)">{{ t('common.close') }}</el-button>
        <el-button type="primary" :loading="pullLoading" @click="emit('doPull')">{{ t('opsdocker.pullGo') }}</el-button>
      </template>
    </el-dialog>
</template>
