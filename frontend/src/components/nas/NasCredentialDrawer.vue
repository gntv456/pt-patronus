<script setup lang="ts">
import type { OpsCredential, OpsCredentialInput } from '@/api/monitor'
import { credentialTypeText } from '@/views/nasServerHelpers'

// form 是父级 composable 持有的响应式对象，子组件直接绑定其字段。
// credentialUsage 是父级传入的计算函数（依赖 targets）。
defineProps<{
  drawerVisible: boolean
  credentials: OpsCredential[]
  formVisible: boolean
  editing: OpsCredential | null
  form: OpsCredentialInput
  credentialUsage: (id: number) => number
}>()

const emit = defineEmits<{
  (e: 'update:drawerVisible', v: boolean): void
  (e: 'update:formVisible', v: boolean): void
  (e: 'add'): void
  (e: 'edit', c: OpsCredential): void
  (e: 'remove', c: OpsCredential): void
  (e: 'save'): void
}>()
</script>

<template>
  <el-drawer
    :model-value="drawerVisible"
    title="凭据中心"
    size="520px"
    @update:model-value="emit('update:drawerVisible', $event)"
  >
    <div class="cred-toolbar">
      <span>统一管理 SSH/NAS 凭据，设备绑定后改一次全局生效。</span>
      <el-button type="primary" size="small" @click="emit('add')">新增凭据</el-button>
    </div>
    <div v-if="credentials.length" class="cred-list">
      <div v-for="c in credentials" :key="c.id" class="cred-card">
        <div class="cred-main">
          <div>
            <b>{{ c.name }}</b>
            <span>{{ credentialTypeText(c.type) }} · {{ c.username || '无用户名' }}</span>
          </div>
          <el-tag size="small" type="info">{{ credentialUsage(c.id) }} 台设备</el-tag>
        </div>
        <div class="cred-flags">
          <span v-if="c.has_password">密码</span>
          <span v-if="c.has_private_key">私钥</span>
          <span v-if="c.tags">{{ c.tags }}</span>
        </div>
        <p v-if="c.note">{{ c.note }}</p>
        <div class="cred-actions">
          <el-button size="small" @click="emit('edit', c)">编辑</el-button>
          <el-button size="small" type="danger" plain @click="emit('remove', c)">删除</el-button>
        </div>
      </div>
    </div>
    <div v-else class="cred-empty">
      暂无统一凭据
      <el-button type="primary" size="small" @click="emit('add')">新增第一条</el-button>
    </div>
  </el-drawer>

  <el-dialog
    :model-value="formVisible"
    :title="editing ? '编辑凭据' : '新增凭据'"
    width="520px"
    destroy-on-close
    @update:model-value="emit('update:formVisible', $event)"
  >
    <el-form label-width="82px">
      <el-form-item label="名称" required>
        <el-input v-model="form.name" placeholder="如：生产 root / NAS 管理员" />
      </el-form-item>
      <el-form-item label="类型">
        <el-select v-model="form.type" style="width: 100%">
          <el-option label="SSH" value="ssh" />
          <el-option label="NAS" value="nas" />
          <el-option label="通用" value="generic" />
        </el-select>
      </el-form-item>
      <el-form-item label="用户名">
        <el-input v-model="form.username" placeholder="root / admin" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input
          v-model="form.password"
          type="password"
          show-password
          :placeholder="editing ? '留空不改' : '密码 / Token / API Key'"
        />
      </el-form-item>
      <el-form-item label="私钥">
        <el-input
          v-model="form.private_key"
          type="textarea"
          :rows="6"
          resize="vertical"
          :placeholder="editing?.has_private_key ? '已配置私钥，留空不改' : 'SSH 私钥，可留空'"
        />
      </el-form-item>
      <el-form-item label="标签">
        <el-input v-model="form.tags" placeholder="生产环境, root" />
      </el-form-item>
      <el-form-item label="备注">
        <el-input v-model="form.note" type="textarea" :rows="3" resize="vertical" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:formVisible', false)">取消</el-button>
      <el-button type="primary" @click="emit('save')">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped src="../../views/nas-shared/credential.css"></style>
