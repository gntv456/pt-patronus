<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { diskApi, type DiskEntry } from '@/api/disk'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import GlassIcon from '@/components/GlassIcon.vue'

const { t } = useI18n()
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
  if (d.is_system) return t('opsdisk.sysDisk')
  if (d.category === 'unpartitioned') return t('opsdisk.unpartitioned')
  return t('opsdisk.normalDisk')
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
    ElMessage.warning(t('opsdisk.mountpointRequired'))
    return
  }
  try {
    const devName = partDevice.value.replace(/^\/dev\//, '')
    await ElMessageBox.prompt(
      t('opsdisk.confirmPart1', { dev: partDevice.value, fs: partForm.value.fstype }) +
        t('opsdisk.confirmPart2') +
        t('opsdisk.confirmPart3', { dev: devName }),
      t('opsdisk.confirmPartTitle'),
      {
        type: 'error',
        confirmButtonText: t('opsdisk.confirmRun'),
        cancelButtonText: t('common.cancel'),
        inputPattern: new RegExp('^' + devName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$'),
        inputErrorMessage: t('opsdisk.inputErr', { dev: devName }),
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
      ElMessage.success(
        t('opsdisk.partOk', { part: res.partition || partDevice.value, mount: partForm.value.mountpoint }),
      )
      partDialog.value = false
      refresh()
    } else {
      ElMessage.error(res.error || t('opsdisk.partFailed'))
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
    ElMessage.warning(t('opsdisk.mountpointRequired'))
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
      ElMessage.success(t('opsdisk.mountOk'))
      mountDialog.value = false
      refresh()
    } else {
      ElMessage.error(res.error || t('opsdisk.mountFailed'))
    }
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    actionLoading.value = false
  }
}

async function doUmount(mountpoint: string) {
  try {
    await ElMessageBox.confirm(t('opsdisk.confirmUmount', { mount: mountpoint }), t('opsdisk.confirmUmountTitle'), {
      type: 'warning',
    })
    actionLoading.value = true
    const res = await diskApi.umount(selectedTarget.value, { mountpoint, remove_fstab: true })
    if (res.ok) {
      ElMessage.success(t('opsdisk.umountOk'))
      refresh()
    } else {
      ElMessage.error(res.error || t('opsdisk.umountFailed'))
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
        <label class="target-label">{{ t('opsproc.targetHost') }}</label>
        <el-select
          v-model="selectedTarget"
          :placeholder="t('opsproc.selectHost')"
          style="width: 220px"
          @change="refresh"
        >
          <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
      </div>
      <el-button type="primary" :icon="'Refresh'" :loading="loading" @click="refresh">{{
        t('common.refresh')
      }}</el-button>
    </div>

    <div v-if="error" class="disk-error">{{ error }}</div>
    <div v-if="!sshTargets.length" class="disk-empty glass">
      {{ t('opsdisk.noHostHint') }}
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
            <div class="disk-model">{{ d.model || d.vendor || t('opsdisk.unknownModel') }}</div>
            <div v-if="d.serial" class="disk-serial">SN: {{ d.serial }}</div>
            <div class="disk-badges">
              <span class="disk-badge disk-category" :style="{ background: categoryColor(d) }">{{
                categoryLabel(d)
              }}</span>
              <span v-if="diskTypeLabel(d)" class="disk-badge disk-type-chip" :class="diskTypeLabel(d).toLowerCase()">{{
                diskTypeLabel(d)
              }}</span>
            </div>
          </div>
        </div>

        <!-- 整盘容量条（Windows 文件管理器风格） -->
        <div v-if="d.total_bytes" class="disk-capacity">
          <div class="capacity-bar">
            <div
              class="capacity-fill"
              :style="{ width: Math.min(d.usage_percent || 0, 100) + '%', background: usageColor(d.usage_percent) }"
            ></div>
          </div>
          <div class="capacity-text">
            <span class="cap-used">{{ t('opsdisk.used') }} {{ d.used_human }}</span> / {{ t('opsdisk.total') }}
            {{ d.total_human }} ·
            <span class="cap-pct" :style="{ color: usageColor(d.usage_percent) }"
              >{{ (d.usage_percent || 0).toFixed(1) }}%</span
            >
          </div>
        </div>
        <div class="disk-divider"></div>

        <!-- 系统盘：只读展示分区 -->
        <div v-if="d.is_system" class="disk-partitions">
          <div class="part-section-title">{{ t('opsdisk.partInfo') }}</div>
          <div class="part-hint">{{ t('opsdisk.sysDiskHint') }}</div>
          <div v-for="p in d.partitions" :key="p.name" class="part-row">
            <div class="part-row-main">
              <span class="part-name">{{ p.name }}</span>
              <span class="part-size">{{ p.size_human }}</span>
              <span class="part-fs">{{ p.fstype || '-' }}</span>
              <span class="part-mount">{{ p.mountpoint || t('opsdisk.notMounted') }}</span>
            </div>
            <div v-if="p.usage_percent" class="part-usage">
              <div class="part-usage-bar">
                <div
                  class="part-usage-fill"
                  :style="{ width: Math.min(p.usage_percent, 100) + '%', background: usageColor(p.usage_percent) }"
                ></div>
              </div>
              <span class="part-usage-text"
                >{{ p.used_human }} / {{ p.size_human }}（{{ p.usage_percent.toFixed(0) }}%）</span
              >
            </div>
          </div>
          <div v-if="!d.partitions?.length" class="part-hint">{{ t('opsdisk.noPartitions') }}</div>
        </div>

        <!-- 未分区磁盘：可分区 -->
        <div v-else-if="d.category === 'unpartitioned'" class="disk-actions-area">
          <p class="disk-warn">{{ t('opsdisk.unpartWarn') }}</p>
          <el-button type="warning" @click="openPartDialog(d)">{{ t('opsdisk.partAndMount') }}</el-button>
        </div>

        <!-- 普通磁盘：展示分区，可挂载/卸载 -->
        <div v-else class="disk-partitions">
          <div class="part-section-title">{{ t('opsdisk.partList') }}</div>
          <div v-for="p in d.partitions" :key="p.name" class="part-row">
            <div class="part-row-main">
              <span class="part-name">{{ p.name }}</span>
              <span class="part-size">{{ p.size_human }}</span>
              <span class="part-fs">{{ p.fstype || '-' }}</span>
              <span class="part-mount" :title="p.mountpoint">{{ p.mountpoint || t('opsdisk.notMounted') }}</span>
              <span class="part-ops">
                <el-button v-if="!p.mountpoint" size="small" type="primary" link @click="openMountDialog(p.name)">{{
                  t('opsdisk.mount')
                }}</el-button>
                <el-button v-else size="small" type="danger" link @click="doUmount(p.mountpoint!)">{{
                  t('opsdisk.umount')
                }}</el-button>
              </span>
            </div>
            <div v-if="p.usage_percent" class="part-usage">
              <div class="part-usage-bar">
                <div
                  class="part-usage-fill"
                  :style="{ width: Math.min(p.usage_percent, 100) + '%', background: usageColor(p.usage_percent) }"
                ></div>
              </div>
              <span class="part-usage-text"
                >{{ p.used_human }} / {{ p.size_human }}（{{ p.usage_percent.toFixed(0) }}%）</span
              >
            </div>
          </div>
          <div v-if="!d.partitions?.length" class="part-hint">{{ t('opsdisk.noPartitions') }}</div>
        </div>
      </div>
    </div>

    <!-- 分区对话框 -->
    <el-dialog v-model="partDialog" :title="t('opsdisk.partAndMount')" width="480px">
      <el-alert type="error" :closable="false" show-icon style="margin-bottom: 16px">
        {{ t('opsdisk.partDialogDesc', { dev: partDevice }) }}
      </el-alert>
      <el-form label-width="90px">
        <el-form-item :label="t('opsdisk.device')">{{ partDevice }}</el-form-item>
        <el-form-item :label="t('opsdisk.fstype')">
          <el-radio-group v-model="partForm.fstype">
            <el-radio value="ext4">ext4</el-radio>
            <el-radio value="xfs">xfs</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('opsdisk.mountpoint')">
          <el-input v-model="partForm.mountpoint" placeholder="/mnt/data" />
        </el-form-item>
        <el-form-item :label="t('opsdisk.autoMount')">
          <el-switch v-model="partForm.auto_mount" />
          <span class="form-hint">{{ t('opsdisk.fstabHint') }}</span>
        </el-form-item>
        <el-form-item v-if="partForm.auto_mount" label="nofail">
          <el-switch v-model="partForm.nofail" />
          <span class="form-hint">{{ t('opsdisk.nofailHint') }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="partDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="actionLoading" @click="doPartition">{{
          t('opsdisk.confirmPartBtn')
        }}</el-button>
      </template>
    </el-dialog>

    <!-- 挂载对话框 -->
    <el-dialog v-model="mountDialog" :title="t('opsdisk.mountTitle')" width="420px">
      <el-form label-width="90px">
        <el-form-item :label="t('opsdisk.device')">{{ mountDevice }}</el-form-item>
        <el-form-item :label="t('opsdisk.mountpoint')">
          <el-input v-model="mountForm.mountpoint" placeholder="/mnt/data" />
        </el-form-item>
        <el-form-item :label="t('opsdisk.autoMount')">
          <el-switch v-model="mountForm.auto_mount" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="mountDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="actionLoading" @click="doMount">{{ t('opsdisk.mount') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-disk {
  padding: 0 4px;
}
.disk-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 18px;
  margin-bottom: 16px;
}
.toolbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.target-label {
  font-size: 13px;
  color: var(--text-secondary, #888);
  white-space: nowrap;
}
.disk-error {
  color: #e74c3c;
  margin-bottom: 12px;
  padding: 8px 14px;
  background: rgba(231, 76, 60, 0.08);
  border-radius: 8px;
}
.disk-empty {
  padding: 28px;
  text-align: center;
  color: var(--text-secondary, #888);
}
.disk-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(380px, 1fr));
  gap: 16px;
}
.disk-card {
  padding: 18px 20px;
}

/* 卡片头部 */
.disk-card-head {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}
.disk-icon {
  flex-shrink: 0;
  line-height: 0;
}
.disk-meta {
  flex: 1;
  min-width: 0;
}
.disk-title-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
}
.disk-name {
  font-weight: 700;
  font-size: 19px;
  letter-spacing: 0.3px;
}
.disk-size-big {
  font-size: 17px;
  font-weight: 600;
  color: var(--text-secondary, #6ea8fe);
}
.disk-model {
  font-size: 13px;
  color: var(--text-secondary, #aaa);
  margin-top: 3px;
}
.disk-serial {
  font-size: 11px;
  color: var(--text-tertiary, #999);
  margin-top: 1px;
}
.disk-badges {
  display: flex;
  gap: 6px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.disk-badge {
  display: inline-flex;
  align-items: center;
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  padding: 3px 10px;
  border-radius: 999px;
  white-space: nowrap;
  line-height: 1.6;
}
.disk-category {
  box-shadow: var(--shadow-sm);
}
.disk-type-chip {
  background: rgba(0, 0, 0, 0.3);
}
.disk-type-chip.ssd {
  background: rgba(9, 132, 227, 0.85);
}
.disk-type-chip.hdd {
  background: rgba(108, 92, 231, 0.85);
}

/* 分隔线 */
.disk-divider {
  height: 1px;
  margin: 14px 0 6px;
  background: linear-gradient(90deg, transparent, rgba(128, 128, 128, 0.28), transparent);
}

/* 分区列表 */
.disk-partitions {
  margin-top: 6px;
}
.part-section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary, #888);
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.part-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 0;
  border-bottom: 1px solid rgba(128, 128, 128, 0.12);
  font-size: 13px;
}
.part-row:last-child {
  border-bottom: none;
}
.part-name {
  font-family: 'Consolas', 'Monaco', monospace;
  font-weight: 600;
  min-width: 96px;
}
.part-size {
  color: var(--text-secondary, #9aa6b2);
  min-width: 60px;
}
.part-fs {
  min-width: 44px;
  color: var(--text-secondary, #9aa6b2);
}
.part-mount {
  flex: 1;
  color: var(--text-secondary, #9aa6b2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.part-ops {
  flex-shrink: 0;
}
/* 分区行内部：主信息行 */
.part-row-main {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}
/* 分区用量条 */
.part-usage {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding-left: 2px;
}
.part-usage-bar {
  flex: 1;
  height: 8px;
  background: rgba(128, 128, 128, 0.18);
  border-radius: 4px;
  overflow: hidden;
}
.part-usage-fill {
  height: 100%;
  border-radius: 4px;
  transition:
    width var(--dur-slow) var(--ease-out-smooth),
    background var(--dur-base) var(--ease-out-smooth);
}
.part-usage-text {
  font-size: 11px;
  color: var(--text-tertiary, #9aa6b2);
  white-space: nowrap;
  min-width: 150px;
  text-align: right;
}

/* 整盘容量条（Windows 文件管理器风格） */
.disk-capacity {
  margin: 12px 0 2px;
}
.capacity-bar {
  width: 100%;
  height: 14px;
  background: rgba(128, 128, 128, 0.18);
  border-radius: 7px;
  overflow: hidden;
  box-shadow: var(--shadow-inset);
}
.capacity-fill {
  height: 100%;
  border-radius: 7px;
  transition:
    width var(--dur-slow) var(--ease-out-smooth),
    background var(--dur-base) var(--ease-out-smooth);
  box-shadow: var(--shadow-xs);
  background-image: linear-gradient(180deg, rgba(255, 255, 255, 0.35), rgba(255, 255, 255, 0) 60%);
}
.capacity-text {
  font-size: 12px;
  color: var(--text-secondary, #9aa6b2);
  margin-top: 5px;
}
.capacity-text .cap-used {
  color: var(--text-strong, #1f2937);
  font-weight: 600;
}
.capacity-text .cap-pct {
  font-weight: 700;
  font-family: 'Consolas', 'Monaco', monospace;
}
.part-hint {
  font-size: 12px;
  color: var(--text-tertiary, #aaa);
  padding: 4px 0;
}
.disk-actions-area {
  margin-top: 8px;
}
.disk-warn {
  font-size: 12px;
  color: #e67e22;
  margin-bottom: 8px;
}
.form-hint {
  font-size: 12px;
  color: var(--text-secondary, #888);
  margin-left: 8px;
}
</style>
