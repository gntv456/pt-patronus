<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; done: [] }>()
const content = ref('')
const name = ref('')
const output = ref('')
const loading = ref(false)

const tpl = `version: "3.8"
services:
  hello:
    image: hello-world`

watch(() => props.visible, (v) => {
  if (v) { content.value = tpl; name.value = ''; output.value = '' }
})

async function action(act: 'up' | 'down' | 'validate') {
  if (!content.value.trim()) { ElMessage.warning('请输入 compose 内容'); return }
  loading.value = true
  output.value = ''
  try {
    const fn = act === 'up' ? dockerApi.localComposeUp : act === 'down' ? dockerApi.localComposeDown : dockerApi.localComposeValidate
    const res = await fn(content.value, name.value.trim() || undefined)
    output.value = res.output || (res.ok ? '成功' : '失败')
    if (res.ok && act === 'up') { ElMessage.success('部署成功'); emit('done') }
    if (res.ok && act === 'down') { ElMessage.success('已停止'); emit('done') }
  } catch (e: any) { output.value = e.message }
  finally { loading.value = false }
}
</script>

<template>
  <el-dialog :model-value="visible" @update:model-value="emit('update:visible', $event)" title="Docker Compose 部署（本机）" width="700px" top="5vh">
    <el-form label-width="70px">
      <el-form-item label="项目名"><el-input v-model="name" placeholder="myapp（可选）" style="width: 300px" /></el-form-item>
      <el-form-item label="内容"><el-input v-model="content" type="textarea" :rows="16" placeholder="粘贴 docker-compose.yml 内容" style="font-family: monospace; font-size: 13px" /></el-form-item>
    </el-form>
    <div v-if="output" class="compose-output"><pre>{{ output }}</pre></div>
    <template #footer>
      <el-button @click="emit('update:visible', false)">关闭</el-button>
      <el-button :loading="loading" @click="action('validate')">验证</el-button>
      <el-button type="danger" :loading="loading" @click="action('down')">停止</el-button>
      <el-button type="primary" :loading="loading" @click="action('up')">部署</el-button>
    </template>
  </el-dialog>
</template>
