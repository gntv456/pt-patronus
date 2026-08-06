<script setup lang="ts">
import { Delete, Plus } from '@element-plus/icons-vue'

const visible = defineModel<boolean>('visible', { required: true })

defineProps<{
  draft: string[]
  official: string
}>()

const emit = defineEmits<{
  (e: 'add'): void
  (e: 'remove', index: number): void
  (e: 'restore'): void
  (e: 'save'): void
}>()
</script>

<template>
  <el-dialog v-model="visible" title="市场源管理" width="560px">
    <div class="market-source-tip">
      默认已内置官方仓库，可删除或追加第三方仓库。支持 GitHub 仓库地址（如 https://github.com/user/repo，可用 /tree/分支/目录 指定）或任意 plugin-market.json URL。列表顺序即优先级：刷新市场时按此处顺序聚合，相同插件 ID 先列者优先。
    </div>
    <div class="market-source-list">
      <div v-for="(_, i) in draft" :key="i" class="market-source-row">
        <el-tag v-if="draft[i] === official" type="warning" size="small" effect="light">官方</el-tag>
        <el-input
          v-model="draft[i]"
          clearable
          :placeholder="i === 0 ? 'https://github.com/owner/repo' : 'https://github.com/owner/repo'"
        />
        <el-button :icon="Delete" circle plain @click="emit('remove', i)" />
      </div>
    </div>
    <div class="market-source-add">
      <el-button :icon="Plus" plain @click="emit('add')">添加源</el-button>
      <el-button v-if="official && !draft.includes(official)" plain @click="emit('restore')">恢复官方源</el-button>
    </div>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" @click="emit('save')">保存</el-button>
    </template>
  </el-dialog>
</template>
