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
function usageColor(percent: number | undefined): string {
  if (!percent || percent <= 0) return '#7f8c8d'
  if (percent < 60) return '#27ae60'
  if (percent < 85) return '#f39c12'
  return '#e74c3c'
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
    const devName = partDevice.value.replace(/^\/dev\//, '')
    await ElMessageBox.prompt(
      `⚠️ 即将对 ${partDevice.value} 执行分区和格式化（${partForm.value.fstype}）。\n` +
      `这会清除该磁盘上的【所有数据】，操作不可撤销！\n` +
      `请输入设备名 "${devName}" 以确认：`,
      '危险操作确认',
      {
        type: 'error',
        confirmButtonText: '确认执行',
        cancelButtonText: '取消',
        inputPattern: new RegExp('^' + devName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$'),
        inputErrorMessage: `请输入：${devName}`,
      },
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
        <!-- 头部：大图标 + 设备名 + 总容量 -->
        <div class="disk-card-head">
          <div class="disk-icon">
            <GlassIcon name="harddisk" from="#0984e3" :to="categoryColor(d)" :size="52" />
          </div>
          <div class="disk-meta">
            <div class="disk-title-row">
              <span class="disk-name">{{ d.name }}</span>
              <span class="disk-size-big">{{ d.size_human }}</span>
            </div>
            <div class="disk-model">{{ d.model || d.vendor || '未知型号' }}</div>
            <div v-if="d.serial" class="disk-serial">SN: {{ d.serial }}</div>
            <div class="disk-badges">
              <span class="disk-badge disk-category" :style="{ background: categoryColor(d) }">{{ categoryLabel(d) }}</span>
              <span v-if="diskTypeLabel(d)" class="disk-badge disk-type-chip" :class="diskTypeLabel(d).toLowerCase()">{{ diskTypeLabel(d) }}</span>
            </div>
          </div>
        </div>

        <!-- 整盘容量条（Windows 文件管理器风格） -->
        <div v-if="d.total_bytes" class="disk-capacity">
          <div class="capacity-bar">
            <div class="capacity-fill" :style="{ width: Math.min(d.usage_percent || 0, 100) + '%', background: usageColor(d.usage_percent) }"></div>
          </div>
          <div class="capacity-text"><span class="cap-used">已用 {{ d.used_human }}</span> / 共 {{ d.total_human }} · <span class="cap-pct" :style="{ color: usageColor(d.usage_percent) }">{{ (d.usage_percent || 0).toFixed(1) }}%</span></div>
        </div>
        <div class="disk-divider"></div>

        <!-- 系统盘：只读展示分区 -->
        <div v-if="d.is_system" class="disk-partitions">
          <div class="part-section-title">分区信息</div>
          <div class="part-hint">系统盘，仅展示信息，不可操作</div>
          <div v-for="p in d.partitions" :key="p.name" class="part-row">
            <div class="part-row-main">
              <span class="part-name">{{ p.name }}</span>
              <span class="part-size">{{ p.size_human }}</span>
              <span class="part-fs">{{ p.fstype || '-' }}</span>
              <span class="part-mount">{{ p.mountpoint || '未挂载' }}</span>
            </div>
            <div v-if="p.usage_percent" class="part-usage">
              <div class="part-usage-bar">
                <div class="part-usage-fill" :style="{ width: Math.min(p.usage_percent, 100) + '%', background: usageColor(p.usage_percent) }"></div>
              </div>
              <span class="part-usage-text">{{ p.used_human }} / {{ p.size_human }}（{{ p.usage_percent.toFixed(0) }}%）</span>
            </div>
          </div>
          <div v-if="!d.partitions?.length" class="part-hint">无分区</div>
        </div>

        <!-- 未分区磁盘：可分区 -->
        <div v-else-if="d.category === 'unpartitioned'" class="disk-actions-area">
          <p class="disk-warn">该磁盘未分区。分区并格式化将清除其上所有数据。</p>
          <el-button type="warning" @click="openPartDialog(d)">分区并挂载</el-button>
        </div>

        <!-- 普通磁盘：展示分区，可挂载/卸载 -->
        <div v-else class="disk-partitions">
          <div class="part-section-title">分区列表</div>
          <div v-for="p in d.partitions" :key="p.name" class="part-row">
            <div class="part-row-main">
              <span class="part-name">{{ p.name }}</span>
              <span class="part-size">{{ p.size_human }}</span>
              <span class="part-fs">{{ p.fstype || '-' }}</span>
              <span class="part-mount" :title="p.mountpoint">{{ p.mountpoint || '未挂载' }}</span>
              <span class="part-ops">
                <el-button v-if="!p.mountpoint" size="small" type="primary" link @click="openMountDialog(p.name)">挂载</el-button>
                <el-button v-else size="small" type="danger" link @click="doUmount(p.mountpoint!)">卸载</el-button>
              </span>
            </div>
            <div v-if="p.usage_percent" class="part-usage">
              <div class="part-usage-bar">
                <div class="part-usage-fill" :style="{ width: Math.min(p.usage_percent, 100) + '%', background: usageColor(p.usage_percent) }"></div>
              </div>
              <span class="part-usage-text">{{ p.used_human }} / {{ p.size_human }}（{{ p.usage_percent.toFixed(0) }}%）</span>
            </div>
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
.disk-card { padding: 18px 20px; }

/* 卡片头部 */
.disk-card-head { display: flex; align-items: flex-start; gap: 14px; }
.disk-icon { flex-shrink: 0; line-height: 0; }
.disk-meta { flex: 1; min-width: 0; }
.disk-title-row { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; }
.disk-name { font-weight: 700; font-size: 19px; letter-spacing: .3px; }
.disk-size-big { font-size: 17px; font-weight: 600; color: var(--text-secondary, #6ea8fe); }
.disk-model { font-size: 13px; color: var(--text-secondary, #aaa); margin-top: 3px; }
.disk-serial { font-size: 11px; color: var(--text-tertiary, #999); margin-top: 1px; }
.disk-badges { display: flex; gap: 6px; margin-top: 8px; flex-wrap: wrap; }
.disk-badge { display: inline-flex; align-items: center; color: #fff; font-size: 12px; font-weight: 600; padding: 3px 10px; border-radius: 999px; white-space: nowrap; line-height: 1.6; }
.disk-category { box-shadow: var(--shadow-sm); }
.disk-type-chip { background: rgba(0,0,0,.30); }
.disk-type-chip.ssd { background: rgba(9,132,227,.85); }
.disk-type-chip.hdd { background: rgba(108,92,231,.85); }

/* 分隔线 */
.disk-divider { height: 1px; margin: 14px 0 6px; background: linear-gradient(90deg, transparent, rgba(128,128,128,.28), transparent); }

/* 分区列表 */
.disk-partitions { margin-top: 6px; }
.part-section-title { font-size: 12px; font-weight: 600; color: var(--text-secondary, #888); margin-bottom: 6px; text-transform: uppercase; letter-spacing: .5px; }
.part-row { display: flex; flex-direction: column; gap: 6px; padding: 8px 0; border-bottom: 1px solid rgba(128,128,128,.12); font-size: 13px; }
.part-row:last-child { border-bottom: none; }
.part-name { font-family: 'Consolas','Monaco',monospace; font-weight: 600; min-width: 96px; }
.part-size { color: var(--text-secondary,#9aa6b2); min-width: 60px; }
.part-fs { min-width: 44px; color: var(--text-secondary,#9aa6b2); }
.part-mount { flex: 1; color: var(--text-secondary,#9aa6b2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.part-ops { flex-shrink: 0; }
/* 分区行内部：主信息行 */
.part-row-main { display: flex; align-items: center; gap: 10px; width: 100%; }
/* 分区用量条 */
.part-usage { display: flex; align-items: center; gap: 10px; width: 100%; padding-left: 2px; }
.part-usage-bar { flex: 1; height: 8px; background: rgba(128,128,128,.18); border-radius: 4px; overflow: hidden; }
.part-usage-fill { height: 100%; border-radius: 4px; transition: width var(--dur-slow) var(--ease-out-smooth), background var(--dur-base) var(--ease-out-smooth); }
.part-usage-text { font-size: 11px; color: var(--text-tertiary,#9aa6b2); white-space: nowrap; min-width: 150px; text-align: right; }

/* 整盘容量条（Windows 文件管理器风格） */
.disk-capacity { margin: 12px 0 2px; }
.capacity-bar { width: 100%; height: 14px; background: rgba(128,128,128,.18); border-radius: 7px; overflow: hidden; box-shadow: var(--shadow-inset); }
.capacity-fill { height: 100%; border-radius: 7px; transition: width var(--dur-slow) var(--ease-out-smooth), background var(--dur-base) var(--ease-out-smooth); box-shadow: var(--shadow-xs); background-image: linear-gradient(180deg, rgba(255,255,255,.35), rgba(255,255,255,0) 60%); }
.capacity-text { font-size: 12px; color: var(--text-secondary,#9aa6b2); margin-top: 5px; }
.capacity-text .cap-used { color: var(--text-strong,#1f2937); font-weight: 600; }
.capacity-text .cap-pct { font-weight: 700; font-family: 'Consolas','Monaco',monospace; }
.part-hint { font-size: 12px; color: var(--text-tertiary,#aaa); padding: 4px 0; }
.disk-actions-area { margin-top: 8px; }
.disk-warn { font-size: 12px; color: #e67e22; margin-bottom: 8px; }
.form-hint { font-size: 12px; color: var(--text-secondary,#888); margin-left: 8px; }
</style>
