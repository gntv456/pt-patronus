<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { downloaderApi, type Downloader } from '@/api/downloader'
import TorrentDrawer from '@/components/downloader/TorrentDrawer.vue'
import { useAuthStore } from '@/stores/auth'

// 非 admin 可读可用，但删除下载器是 admin 专属（后端 RequireAdmin 守卫）。
const auth = useAuthStore()

const list = ref<Downloader[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const form = ref({ name: '', type: 'qb', url: '', username: '', password: '', is_default: false })
// 编辑态：null=新建，否则持有正在编辑的下载器 id
const editingId = ref<number | null>(null)
const isEdit = computed(() => editingId.value !== null)
const dialogTitle = computed(() => (isEdit.value ? '编辑下载器' : '添加下载器'))

// 种子抽屉
const drawerDl = ref<Downloader | null>(null)
const drawerVisible = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    list.value = await downloaderApi.list()
  } catch (e: unknown) {
    error.value = (e as Error).message || '加载失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  form.value = { name: '', type: 'qb', url: '', username: '', password: '', is_default: false }
  dialogVisible.value = true
}

function openEdit(dl: Downloader) {
  editingId.value = dl.id
  form.value = {
    name: dl.name,
    type: dl.type,
    url: dl.url,
    username: dl.username,
    password: '', // 留空=不修改（后端 Update 仅在非空时覆写）
    is_default: dl.is_default,
  }
  dialogVisible.value = true
}

async function submit() {
  if (!form.value.name || !form.value.url) {
    ElMessage.warning('名称与地址必填')
    return
  }
  try {
    if (isEdit.value) {
      await downloaderApi.update(editingId.value!, form.value)
      ElMessage.success('已更新')
    } else {
      await downloaderApi.create(form.value)
      ElMessage.success('已添加')
    }
    dialogVisible.value = false
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '失败')
  }
}

async function removeDownloader(dl: Downloader) {
  await ElMessageBox.confirm(`确认删除下载器「${dl.name}」？`, '删除', { type: 'warning' })
  await downloaderApi.remove(dl.id)
  ElMessage.success('已删除')
  await load()
}

async function test(dl: Downloader) {
  const r = await downloaderApi.test(dl.id)
  r.ok ? ElMessage.success(`${dl.name} 连接正常`) : ElMessage.warning(r.error || '连接失败')
}

function openTorrents(dl: Downloader) {
  drawerDl.value = dl
  drawerVisible.value = true
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">下载器</h2>
      <el-button type="primary" @click="openCreate">+ 添加下载器</el-button>
    </div>

    <div v-loading="loading" class="grid">
      <div v-if="error" class="empty glass error-state">
        <div class="err-text">⚠ {{ error }}</div>
        <el-button size="small" @click="load">重试</el-button>
      </div>
      <template v-else>
      <div v-for="dl in list" :key="dl.id" class="glass glass-hover card">
        <div class="row1">
          <GlassIcon
            :name="dl.type === 'tr' ? 'files' : 'downloaders'"
            :from="dl.type === 'tr' ? '#536976' : '#6c5ce7'"
            :to="dl.type === 'tr' ? '#29323c' : '#4285f4'"
            :size="40"
            class="dl-badge"
          />
          <div class="dname-wrap">
            <span class="dname">{{ dl.name }}</span>
            <span class="dtype-tag text-muted">{{ dl.type === 'tr' ? 'Transmission' : 'qBittorrent' }}</span>
          </div>
        </div>
        <div class="url text-muted">{{ dl.url }}</div>
        <div class="actions">
          <el-button size="small" @click="test(dl)">连接测试</el-button>
          <el-button size="small" type="primary" @click="openTorrents(dl)">任务列表</el-button>
          <el-button size="small" @click="openEdit(dl)">编辑</el-button>
          <el-button v-if="auth.isAdmin" size="small" type="danger" plain @click="removeDownloader(dl)">删除</el-button>
        </div>
      </div>
      <div v-if="!list.length && !loading" class="empty glass">暂无下载器</div>
      </template>
    </div>

    <!-- 新建/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="460">
      <el-form label-width="80">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio value="qb">qBittorrent</el-radio>
            <el-radio value="tr">Transmission</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="地址"><el-input v-model="form.url" placeholder="http://127.0.0.1:8080" /></el-form-item>
        <el-form-item label="用户名"><el-input v-model="form.username" /></el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="isEdit ? '留空则不修改' : ''"
          />
        </el-form-item>
        <el-form-item label="默认"><el-switch v-model="form.is_default" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">{{ isEdit ? '保存' : '添加' }}</el-button>
      </template>
    </el-dialog>

    <!-- 任务抽屉（种子管理全功能） -->
    <TorrentDrawer v-if="drawerDl" :downloader="drawerDl" v-model:visible="drawerVisible" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  margin: 4px 0;
  font-size: 22px;
  font-weight: 700;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.card {
  padding: 18px;
}
.row1 {
  display: flex;
  align-items: center;
  gap: 12px;
}
.dname-wrap {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.dname {
  font-size: 16px;
  font-weight: 600;
}
.dtype-tag {
  font-size: 12px;
}
.url {
  margin: 12px 0 14px;
  font-size: 13px;
  word-break: break-all;
}
.actions {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
}
.actions :deep(.el-button) {
  width: 100%;
  margin: 0;
}
.empty {
  padding: 40px;
  text-align: center;
}
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.err-text {
  color: var(--pt-danger, #f56c6c);
  font-size: 14px;
}
</style>
