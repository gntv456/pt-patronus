<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; created: [] }>()
const form = ref({ image: '', name: '', cmd: '', envText: '', portText: '', bindText: '', restart_policy: 'unless-stopped', auto_start: true })
const loading = ref(false)

watch(() => props.visible, (v) => { if (v) form.value = { image: '', name: '', cmd: '', envText: '', portText: '', bindText: '', restart_policy: 'unless-stopped', auto_start: true } })

async function submit() {
  if (!form.value.image.trim()) { ElMessage.warning('请输入镜像名'); return }
  loading.value = true
  try {
    const ports = form.value.portText.split('\n').map(l => l.trim()).filter(Boolean).map(l => {
      const [h, c] = l.split(':')
      return { host: parseInt(h) || 0, container: parseInt(c || h) || 0 }
    })
    const env: Record<string, string> = {}
    form.value.envText.split('\n').map(l => l.trim()).filter(Boolean).forEach(l => {
      const idx = l.indexOf('='); if (idx > 0) env[l.substring(0, idx)] = l.substring(idx + 1)
    })
    const binds = form.value.bindText.split('\n').map(l => l.trim()).filter(Boolean)
    const cmd = form.value.cmd.trim() ? form.value.cmd.trim().split(/\s+/) : undefined
    const res = await dockerApi.localCreate({ image: form.value.image.trim(), name: form.value.name.trim() || undefined, cmd, env, ports, binds, restart_policy: form.value.restart_policy, auto_start: form.value.auto_start })
    if (res.ok) { ElMessage.success('容器已创建'); emit('update:visible', false); emit('created') }
    else ElMessage.error(res.error || '创建失败')
  } catch (e: any) { ElMessage.error(e.message) }
  finally { loading.value = false }
}
</script>

<template>
  <el-dialog :model-value="visible" @update:model-value="emit('update:visible', $event)" title="创建容器" width="560px">
    <el-form label-width="90px">
      <el-form-item label="镜像" required><el-input v-model="form.image" placeholder="nginx:latest" /></el-form-item>
      <el-form-item label="名称"><el-input v-model="form.name" placeholder="my-container（可选）" /></el-form-item>
      <el-form-item label="端口映射"><el-input v-model="form.portText" type="textarea" :rows="2" placeholder="每行一个，格式 8080:80" /></el-form-item>
      <el-form-item label="环境变量"><el-input v-model="form.envText" type="textarea" :rows="2" placeholder="每行一个，格式 KEY=value" /></el-form-item>
      <el-form-item label="卷挂载"><el-input v-model="form.bindText" type="textarea" :rows="2" placeholder="每行一个，格式 /host:/container" /></el-form-item>
      <el-form-item label="启动命令"><el-input v-model="form.cmd" placeholder="覆盖默认命令（可选）" /></el-form-item>
      <el-form-item label="重启策略">
        <el-select v-model="form.restart_policy" style="width: 100%">
          <el-option value="no" label="不自动重启" /><el-option value="always" label="总是重启" />
          <el-option value="unless-stopped" label="除非手动停止" /><el-option value="on-failure" label="失败时重启" />
        </el-select>
      </el-form-item>
      <el-form-item label="自动启动"><el-switch v-model="form.auto_start" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:visible', false)">取消</el-button>
      <el-button type="primary" :loading="loading" @click="submit">创建</el-button>
    </template>
  </el-dialog>
</template>
