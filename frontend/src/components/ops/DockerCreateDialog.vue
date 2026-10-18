<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'
import { createNotice } from '@/utils/dockerCreateFeedback'

const { t } = useI18n()
const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; created: [] }>()
const form = ref({
  image: '',
  name: '',
  cmd: '',
  envText: '',
  portText: '',
  bindText: '',
  restart_policy: 'unless-stopped',
  auto_start: true,
})
const loading = ref(false)

watch(
  () => props.visible,
  (v) => {
    if (v)
      form.value = {
        image: '',
        name: '',
        cmd: '',
        envText: '',
        portText: '',
        bindText: '',
        restart_policy: 'unless-stopped',
        auto_start: true,
      }
  },
)

async function submit() {
  if (!form.value.image.trim()) {
    ElMessage.warning(t('dw.imageRequired'))
    return
  }
  loading.value = true
  try {
    const ports = form.value.portText
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)
      .map((l) => {
        const [h, c] = l.split(':')
        return { host: parseInt(h) || 0, container: parseInt(c || h) || 0 }
      })
    const env: Record<string, string> = {}
    form.value.envText
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)
      .forEach((l) => {
        const idx = l.indexOf('=')
        if (idx > 0) env[l.substring(0, idx)] = l.substring(idx + 1)
      })
    const binds = form.value.bindText
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)
    const cmd = form.value.cmd.trim() ? form.value.cmd.trim().split(/\s+/) : undefined
    const res = await dockerApi.localCreate({
      image: form.value.image.trim(),
      name: form.value.name.trim() || undefined,
      cmd,
      env,
      ports,
      binds,
      restart_policy: form.value.restart_policy,
      auto_start: form.value.auto_start,
    })
    const n = createNotice(res, t)
    if (n.kind === 'warning') ElMessage.warning(n.msg)
    else if (n.kind === 'error') ElMessage.error(n.msg)
    else ElMessage.success(n.msg)
    if (n.close) {
      emit('update:visible', false)
      emit('created')
    }
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    :title="t('dw.title')"
    width="560px"
  >
    <el-form label-width="90px">
      <el-form-item :label="t('dcd2.image')" required
        ><el-input v-model="form.image" placeholder="nginx:latest"
      /></el-form-item>
      <el-form-item :label="t('common.name')"
        ><el-input v-model="form.name" :placeholder="t('dcd2.namePh')"
      /></el-form-item>
      <el-form-item :label="t('dcd2.ports')"
        ><el-input v-model="form.portText" type="textarea" :rows="2" :placeholder="t('dcd2.portsPh')"
      /></el-form-item>
      <el-form-item :label="t('dcd2.env')"
        ><el-input v-model="form.envText" type="textarea" :rows="2" :placeholder="t('dcd2.envPh')"
      /></el-form-item>
      <el-form-item :label="t('dcd2.vol')"
        ><el-input v-model="form.bindText" type="textarea" :rows="2" :placeholder="t('dcd2.volPh')"
      /></el-form-item>
      <el-form-item :label="t('dcd2.cmd')"><el-input v-model="form.cmd" :placeholder="t('dcd2.cmdPh')" /></el-form-item>
      <el-form-item :label="t('dcd2.restart')">
        <el-select v-model="form.restart_policy" style="width: 100%">
          <el-option value="no" :label="t('dw.restart.no')" /><el-option
            value="always"
            :label="t('dw.restart.always')"
          />
          <el-option value="unless-stopped" :label="t('dw.restart.unless')" /><el-option
            value="on-failure"
            :label="t('dw.restart.onFailure')"
          />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('dcd2.autoStart')"><el-switch v-model="form.auto_start" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:visible', false)">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="loading" @click="submit">{{ t('dcd2.create') }}</el-button>
    </template>
  </el-dialog>
</template>
