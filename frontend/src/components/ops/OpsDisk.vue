<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { diskApi, type DiskEntry } from '@/api/disk'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import GlassIcon from '@/components/GlassIcon.vue'

const targets = ref<MonitorTarget[]>([])
const selectedTarget = ref<number>(0)
const disks = ref<DiskEntry[]>([])
const loading = ref(false)
const actionLoading = ref(false)
const error = ref('')

// 分区对话框
const partDialog = ref(false)
const partDevice = ref('')
const partForm = ref({ fstype: 'ext4' as 'ext4' | 'xfs', mountpoint: '', auto_mount: true, nofail: true })

// 挂载对话框
const mountDialog = ref(false)
const mountDevice = ref('')
const mountForm = ref({ mountpoint: '', auto_mount: true, nofail: true })

const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host'))

async function loadTargets() {
  try {
    targets.value = await monitorApi.list()
    if (sshTargets.value.length && !selectedTarget.value) {
      selectedTarget.value = sshTargets.value[0].id
    }
  } catch (e: any) {
    error.value = e.message
  }
}

async function refresh() {
  if (!selectedTarget.value) return
  loading.value = true
  error.value = ''
  try {
    const resp = await diskApi.list(selectedTarget.value)
    disks.value = resp.disks || []
  } catch (e: any) {
    error.value = e.message
    disks.value = []
  } finally {
    loading.value = false
  }
}

function categoryLabel(d: DiskEntry): string {
  if (d.is_system) return '系统盘'
  if (d.category === 'unpartitioned') return '未分区'
  return '普通磁盘'
}
function categoryColor(d: DiskEntry): string {
  if (d.is_system) return '#e74c3c'
  if (d.category === 'unpartitioned') return '#f39c12'
  return '#27ae60'
}
function diskTypeLabel(d: DiskEntry): string {
  if (d.rota === '1') return 'HDD'
  if (d.rota === '0') return 'SSD'
  return ''
}

function openPartDialog(d: DiskEntry) {
  partDevice.value = d.name
  partForm.value = { fstype: 'ext4', mountpoint: '', auto_mount: true, nofail: true }
  partDialog.value = true
}

async function doPartition() {
  if (!partForm.value.mountpoint) {
    ElMessage.warning('请填写挂载点')
    return
  }
  try {
    await ElMessageBox.confirm(
      `⚠️ 即将对 ${partDevice.value} 执行分区和格式化（${partForm.value.fstype}）。` +
      `这会清除该磁盘上的【所有数据】，操作不可撤销！请确认设备名无误且已备份。`,
      '危险操作确认',
      { type: 'error', confirmButtonText: '我已备份，强制执行', cancelButtonText: '取消' },
    )
    actionLoading.value = true
    const res = await diskApi.partition(selectedTarget.value, {
      device: partDevice.value,
      fstype: partForm.value.fstype,
      mountpoint: partForm.value.mountpoint,
      auto_mount: partForm.value.auto_mount,
      nofail: partForm.value.nofail,
      confirm_risk: true,
    })
    if (res.ok) {
      ElMessage.success(`分区成功：${res.partition || partDevice.value} 已挂载到 ${partForm.value.mountpoint}`)
      partDialog.value = false
      refresh()
    } else {
      ElMessage.error(res.error || '分区失败')
    }
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message || String(e))
  } finally {
    actionLoading.value = false
  }
}

function openMountDialog(partName: string) {
  mountDevice.value = partName
  mountForm.value = { mountpoint: '', auto_mount: true, nofail: true }
  mountDialog.value = true
}

async function doMount() {
  if (!mountForm.value.mountpoint) {
    ElMessage.warning('请填写挂载点')
    return
  }
  actionLoading.value = true
  try {
    const res = await diskApi.mount(selectedTarget.value, {
      device: mountDevice.value,
      mountpoint: mountForm.value.mountpoint,
      auto_mount: mountForm.value.auto_mount,
      nofail: mountForm.value.nofail,
    })
    if (res.ok) {
      ElMessage.success('挂载成功')
      mountDialog.value = false
      refresh()
    } else {
      ElMessage.error(res.error || '挂载失败')
    }
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    actionLoading.value = false
  }
}

async function doUmount(mountpoint: string) {
  try {
    await ElMessageBox.confirm(
      `确定要卸载 ${mountpoint} 吗？请先停止正在读写该挂载点的应用、容器和进程。`,
      '卸载确认',
      { type: 'warning' },
    )
    actionLoading.value = true
    const res = await diskApi.umount(selectedTarget.value, { mountpoint, remove_fstab: true })
    if (res.ok) {
      ElMessage.success('已卸载')
      refresh()
    } else {
      ElMessage.error(res.error || '卸载失败（可能挂载点正忙）')
    }
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message)
  } finally {
    actionLoading.value = false
  }
}

onMounted(async () => {
  await loadTargets()
  if (selectedTarget.value) refresh()
})
</script>

<template>
  <div class="ops-disk">
    <!-- 工具栏 -->
    <div class="disk-toolbar glass">
      <div class="toolbar-left">
        <label class="target-label">目标主机</label>
        <el-select v-model="selectedTarget" placeholder="选择 SSH 主机" style="width: 220px" @change="refresh">
          <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
      </div>
      <el-button type="primary" :icon="'Refresh'" :loading="loading" @click="refresh">刷新</el-button>
    </div>

    <div v-if="error" class="disk-error">{{ error }}</div>
    <div v-if="!sshTargets.length" class="disk-empty glass">
      尚未添加 SSH 类型主机。请在 NAS → 监控目标 中添加一台 SSH 主机（即本机或目标 NAS 的 SSH 凭据），才能进行磁盘管理。
    </div>

    <div v-loading="loading" class="disk-grid">
      <div v-for="d in disks" :key="d.name" class="disk-card glass">
        <div class="disk-card-head">
          <div class="disk-icon">
            <GlassIcon name="harddisk" from="#0984e3" :to="categoryColor(d)" :size="36" />
          </div>
          <div class="disk-meta">
            <div class="disk-name">{{ d.name }} <span class="disk-type-chip">{{ diskTypeLabel(d) }}</span></div>
            <div class="disk-sub">{{ d.size_human }} · {{ d.model || d.vendor || '未知型号' }}</div>
            <div v-if="d.serial" class="disk-serial">SN: {{ d.serial }}</div>
          </div>
          <span class="disk-category" :style="{ background: categoryColor(d) }">{{ categoryLabel(d) }}</span>
        </div>

        <!-- 系统盘：只读展示分区 -->
        <div v-if="d.is_system" class="disk-partitions">
          <div class="part-hint">系统盘，仅展示信息，不可操作</div>
          <div v-for="p in d.partitions" :key="p.name" class="part-row">
            <span class="part-name">{{ p.name }}</span>
            <span class="part-size">{{ p.size_human }}</span>
            <span class="part-fs">{{ p.fstype || '-' }}</span>
            <span class="part-mount">{{ p.mountpoint || '未挂载' }}</span>
          </div>
        </div>

        <!-- 未分区磁盘：可分区 -->
        <div v-else-if="d.category === 'unpartitioned'" class="disk-actions-area">
          <p class="disk-warn">该磁盘未分区。分区并格式化将清除其上所有数据。</p>
          <el-button type="warning" @click="openPartDialog(d)">分区并挂载</el-button>
        </div>

        <!-- 普通磁盘：展示分区，可挂载/卸载 -->
        <div v-else class="disk-partitions">
          <div v-for="p in d.partitions" :key="p.name" class="part-row">
            <span class="part-name">{{ p.name }}</span>
            <span class="part-size">{{ p.size_human }}</span>
            <span class="part-fs">{{ p.fstype || '-' }}</span>
            <span class="part-mount" :title="p.mountpoint">{{ p.mountpoint || '未挂载' }}</span>
            <span class="part-ops">
              <el-button v-if="!p.mountpoint" size="small" type="primary" link @click="openMountDialog(p.name)">挂载</el-button>
              <el-button v-else size="small" type="danger" link @click="doUmount(p.mountpoint!)">卸载</el-button>
            </span>
          </div>
          <div v-if="!d.partitions?.length" class="part-hint">无分区</div>
        </div>
      </div>
    </div>

    <!-- 分区对话框 -->
    <el-dialog v-model="partDialog" title="分区并挂载" width="480px">
      <el-alert type="error" :closable="false" show-icon style="margin-bottom: 16px">
        将对 {{ partDevice }} 创建 GPT 分区表并格式化，原有数据将被清除。
      </el-alert>
      <el-form label-width="90px">
        <el-form-item label="设备">{{ partDevice }}</el-form-item>
        <el-form-item label="文件系统">
          <el-radio-group v-model="partForm.fstype">
            <el-radio value="ext4">ext4</el-radio>
            <el-radio value="xfs">xfs</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="挂载点">
          <el-input v-model="partForm.mountpoint" placeholder="/mnt/data" />
        </el-form-item>
        <el-form-item label="自动挂载">
          <el-switch v-model="partForm.auto_mount" />
          <span class="form-hint">写入 /etc/fstab，重启后自动挂载</span>
        </el-form-item>
        <el-form-item v-if="partForm.auto_mount" label="nofail">
          <el-switch v-model="partForm.nofail" />
          <span class="form-hint">磁盘不可用时不阻塞启动</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="partDialog = false">取消</el-button>
        <el-button type="danger" :loading="actionLoading" @click="doPartition">确认分区</el-button>
      </template>
    </el-dialog>

    <!-- 挂载对话框 -->
    <el-dialog v-model="mountDialog" title="挂载分区" width="420px">
      <el-form label-width="90px">
        <el-form-item label="设备">{{ mountDevice }}</el-form-item>
        <el-form-item label="挂载点">
          <el-input v-model="mountForm.mountpoint" placeholder="/mnt/data" />
        </el-form-item>
        <el-form-item label="自动挂载">
          <el-switch v-model="mountForm.auto_mount" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="mountDialog = false">取消</el-button>
        <el-button type="primary" :loading="actionLoading" @click="doMount">挂载</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-disk { padding: 0 4px; }
.disk-toolbar { display: flex; align-items: center; justify-content: space-between; padding: 12px 18px; margin-bottom: 16px; }
.toolbar-left { display: flex; align-items: center; gap: 12px; }
.target-label { font-size: 13px; color: var(--text-secondary, #888); white-space: nowrap; }
.disk-error { color: #e74c3c; margin-bottom: 12px; padding: 8px 14px; background: rgba(231,76,60,.08); border-radius: 8px; }
.disk-empty { padding: 28px; text-align: center; color: var(--text-secondary, #888); }
.disk-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(380px, 1fr)); gap: 16px; }
.disk-card { padding: 16px 18px; }
.disk-card-head { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 12px; }
.disk-icon { flex-shrink: 0; }
.disk-meta { flex: 1; min-width: 0; }
.disk-name { font-weight: 600; font-size: 15px; }
.disk-type-chip { font-size: 11px; padding: 1px 6px; border-radius: 4px; background: rgba(0,0,0,.08); color: var(--text-secondary,#888); }
.disk-sub { font-size: 12px; color: var(--text-secondary, #999); margin-top: 2px; }
.disk-serial { font-size: 11px; color: var(--text-tertiary, #aaa); }
.disk-category { flex-shrink: 0; color: #fff; font-size: 11px; padding: 3px 8px; border-radius: 4px; white-space: nowrap; }
.disk-partitions { margin-top: 8px; }
.part-row { display: flex; align-items: center; gap: 8px; padding: 6px 0; border-bottom: 1px solid rgba(128,128,128,.12); font-size: 13px; }
.part-row:last-child { border-bottom: none; }
.part-name { font-family: monospace; min-width: 90px; }
.part-size { color: var(--text-secondary,#888); min-width: 56px; }
.part-fs { min-width: 44px; }
.part-mount { flex: 1; color: var(--text-secondary,#888); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.part-ops { flex-shrink: 0; }
.part-hint { font-size: 12px; color: var(--text-tertiary,#aaa); padding: 4px 0; }
.disk-actions-area { margin-top: 8px; }
.disk-warn { font-size: 12px; color: #e67e22; margin-bottom: 8px; }
.form-hint { font-size: 12px; color: var(--text-secondary,#888); margin-left: 8px; }
</style>
