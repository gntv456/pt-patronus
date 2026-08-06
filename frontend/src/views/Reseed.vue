<script setup lang="ts">
import { safeUrl } from '@/utils/safeUrl'
import type { ReseedTask } from '@/api/reseed'
import ReseedMissingDialog from '@/components/ReseedMissingDialog.vue'
import { useReseed } from '@/views/useReseed'

const {
  config, tokenInput, savingToken, torznabUrl, torznabKey, savingTorznab,
  downloaders, selectedDl, reseeding, tasks, loadingTasks,
  focusSiteId, focusSiteName, statusFilter, resolving,
  setting, savingSetting, showMissing, missingTargets, missingResult, checking, sites,
  loadTasks, saveToken, saveTorznab, saveSettingField, startReseed, openMissing, checkMissing, resolve,
  statusType, statusLabel,
} = useReseed()
</script>

<template>
  <div class="reseed">
    <!-- 标题行 -->
    <div class="page-head">
      <h2 class="page-title">辅种</h2>
      <span class="subtitle text-muted">IYUU 精确匹配 / Torznab 候选 / 本地名称+大小 · 一键添加辅种</span>
      <span class="cfg-chip" :class="config.configured ? 'ok' : 'warn'"><span class="dot"></span>{{ config.configured ? 'IYUU 已配置' : 'IYUU 未配置' }}</span>
      <span class="cfg-chip" :class="config.torznab_configured ? 'ok' : 'warn'"><span class="dot"></span>{{ config.torznab_configured ? 'Torznab 已配置' : 'Torznab 未配置' }}</span>
    </div>

    <!-- 工具栏 -->
    <div class="toolbar card glass">
      <el-select v-model="selectedDl" placeholder="选择下载器" class="dl-select">
        <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
      </el-select>
      <el-alert v-if="!config.configured" type="info" :closable="false" show-icon title="未配置 IYUU：触发辅种将使用本地名称+大小跨站匹配（去 IYUU 单点兜底）" style="margin-bottom: 12px" />
      <el-button type="primary" :loading="reseeding" @click="startReseed">
        <GlassIcon name="seeding" from="#11998e" to="#38ef7d" :size="16" class="btn-ico" />
        <span>开始辅种</span>
      </el-button>
      <el-button @click="openMissing">
        <GlassIcon name="history" from="#6c5ce7" to="#4285f4" :size="16" class="btn-ico" />
        <span>未辅种检查</span>
      </el-button>
      <div class="spacer"></div>
      <el-popover trigger="click" placement="bottom-end" :width="380" popper-class="reseed-cfg-pop">
        <template #reference>
          <el-button class="cfg-btn">
            <GlassIcon name="settings" from="#6c5ce7" to="#4285f4" :size="16" class="btn-ico" />
            <span>设置</span>
          </el-button>
        </template>
        <div class="cfg-pop">
          <div class="cfg-sec">
            <div class="cfg-sec-head">
              <span class="cfg-sec-title">IYUU Token</span>
              <span class="cfg-state" :class="config.configured ? 'ok' : 'warn'">{{ config.configured ? `已配置 ${config.masked}` : '未配置' }}</span>
            </div>
            <div class="token-row">
              <el-input v-model="tokenInput" :placeholder="config.configured ? '输入新值覆盖' : '粘贴 IYUU token'" type="password" show-password clearable />
              <el-button type="primary" :loading="savingToken" @click="saveToken">保存</el-button>
            </div>
            <a class="cfg-link" href="https://iyuu.cn" target="_blank" rel="noopener">前往 iyuu.cn 获取 token 并绑定站点 →</a>
          </div>
          <div class="cfg-divider"></div>
          <div class="cfg-sec">
            <div class="cfg-sec-head">
              <span class="cfg-sec-title">Torznab</span>
              <span class="cfg-state" :class="config.torznab_configured ? 'ok' : 'warn'">{{ config.torznab_configured ? '已配置' : '未配置' }}</span>
            </div>
            <p class="cfg-hint text-muted">Jackett/Prowlarr 端点，作辅种候选源（可与 IYUU 并存）</p>
            <div class="token-row">
              <el-input v-model="torznabUrl" placeholder="https://jackett/api/v2.0/indexers/all/results/torznab" clearable size="small" />
            </div>
            <div class="token-row" style="margin-top: 6px">
              <el-input v-model="torznabKey" :placeholder="config.torznab_key_masked ? `API Key（已配置 ${config.torznab_key_masked}，留空保留）` : 'API Key（可选）'" show-password clearable size="small" @keyup.enter="saveTorznab" />
              <el-button type="primary" :loading="savingTorznab" @click="saveTorznab">保存</el-button>
            </div>
          </div>
          <div class="cfg-divider"></div>
          <div class="cfg-sec" v-if="selectedDl !== null">
            <div class="cfg-sec-title">自动辅种</div>
            <div class="switch-row">
              <span>定时自动辅种</span>
              <el-switch v-model="setting.auto_reseed" :loading="savingSetting" @change="saveSettingField" />
            </div>
            <div class="switch-row">
              <span class="sub-label">自动添加新候选</span>
              <el-switch v-model="setting.auto_reseed_add" :disabled="!setting.auto_reseed" :loading="savingSetting" @change="saveSettingField" />
            </div>
            <p class="cfg-cron text-muted">每天 3 点扫描（KEEPSEED_CRON 可改）</p>
          </div>
          <p v-else class="cfg-empty text-muted">请先选择下载器以配置自动辅种</p>
        </div>
      </el-popover>
    </div>

    <el-alert v-if="focusSiteId" type="info" :closable="false" show-icon style="margin-bottom: 12px" :title="`仅显示候选站「${focusSiteName}」的辅种任务（${tasks.length}）`" />

    <!-- 候选任务表 -->
    <div class="card glass table-card">
      <div class="card-head">
        <span>候选任务<span class="cnt text-muted">（{{ tasks.length }}）</span></span>
        <el-radio-group v-model="statusFilter" size="small" @change="loadTasks">
          <el-radio-button label="pending">待处理</el-radio-button>
          <el-radio-button label="added">已添加</el-radio-button>
          <el-radio-button label="skipped">已跳过</el-radio-button>
          <el-radio-button label="failed">失败</el-radio-button>
          <el-radio-button label="">全部</el-radio-button>
        </el-radio-group>
      </div>
      <div class="table-wrap">
        <el-table :data="tasks" v-loading="loadingTasks" height="100%" empty-text="暂无候选，点「开始辅种」">
          <el-table-column label="源种子" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="src-name">{{ row.source_name || row.info_hash }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="candidate_site" label="候选站点" width="110" />
          <el-table-column label="候选链接" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              <a :href="safeUrl(row.candidate_url)" target="_blank" rel="noopener" class="dl-link">下载链接</a>
            </template>
          </el-table-column>
          <el-table-column label="保存路径" min-width="160" show-overflow-tooltip>
            <template #default="{ row }"><span class="text-muted">{{ row.save_path || '-' }}</span></template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column label="操作" width="150" fixed="right">
            <template #default="{ row }">
              <div v-if="row.status === 'pending'" class="row-actions">
                <el-button size="small" type="primary" :loading="resolving.has(row.id)" @click="resolve(row as ReseedTask, 'add')">添加辅种</el-button>
                <el-button size="small" :loading="resolving.has(row.id)" @click="resolve(row as ReseedTask, 'skip')">跳过</el-button>
              </div>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 未辅种检查 -->
    <ReseedMissingDialog
      v-model:visible="showMissing" v-model:targets="missingTargets" v-model:result="missingResult" v-model:checking="checking"
      :sites="sites" @check="checkMissing"
    />
  </div>
</template>

<style scoped>
.reseed { display: flex; flex-direction: column; gap: 14px; height: 100%; min-height: 0; }
.page-head { display: flex; align-items: center; gap: 12px; flex: none; min-width: 0; }
.page-title { margin: 0; font-size: 22px; font-weight: 700; flex: none; }
.subtitle { margin: 0; font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.cfg-chip { display: inline-flex; align-items: center; gap: 6px; margin-left: auto; padding: 5px 12px; border-radius: 999px; font-size: 12px; font-weight: 600; white-space: nowrap; flex: none; background: rgba(255, 255, 255, 0.5); }
:root.dark .cfg-chip { background: rgba(255, 255, 255, 0.06); }
.cfg-chip .dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
.cfg-chip.ok { color: var(--pt-success); }
.cfg-chip.warn { color: var(--pt-warn); }
.toolbar { display: flex; align-items: center; gap: 10px; padding: 12px 16px; flex-wrap: wrap; flex: none; }
.dl-select { width: 220px; }
.btn-ico { margin-right: 5px; vertical-align: middle; }
.spacer { flex: 1; }
.table-card { flex: 1; min-height: 0; display: flex; flex-direction: column; padding: 14px 16px; }
.card-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; font-weight: 600; font-size: 15px; flex: none; }
.cnt { font-weight: 400; font-size: 13px; margin-left: 2px; }
.table-wrap { flex: 1; min-height: 0; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) { background-color: transparent !important; }
.src-name { font-size: 13px; }
.dl-link { color: #6ea8fe; font-size: 13px; }
.row-actions { display: flex; gap: 6px; }
</style>

<!-- 弹层（el-popover teleport 到 body，scoped 选不中，单独用 popper-class 命名空间） -->
<style>
.reseed-cfg-pop { --el-popover-padding: 16px; }
.reseed-cfg-pop .cfg-sec-title { font-weight: 600; font-size: 13px; }
.reseed-cfg-pop .cfg-sec-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.reseed-cfg-pop .cfg-state { font-size: 12px; font-weight: 600; }
.reseed-cfg-pop .cfg-state.ok { color: var(--pt-success); }
.reseed-cfg-pop .cfg-hint { margin: 0 0 8px; font-size: 12px; line-height: 1.4; }
.reseed-cfg-pop .cfg-state.warn { color: var(--pt-warn); }
.reseed-cfg-pop .token-row { display: flex; gap: 8px; }
.reseed-cfg-pop .cfg-link { display: inline-block; margin-top: 8px; font-size: 12px; color: var(--pt-primary); text-decoration: none; }
.reseed-cfg-pop .cfg-divider { height: 1px; background: rgba(128, 128, 128, 0.18); margin: 14px 0; }
.reseed-cfg-pop .switch-row { display: flex; align-items: center; justify-content: space-between; padding: 6px 0; font-size: 13px; }
.reseed-cfg-pop .switch-row .sub-label { color: var(--text-muted); }
.reseed-cfg-pop .cfg-cron { font-size: 12px; margin: 6px 0 0; }
.reseed-cfg-pop .cfg-empty { font-size: 13px; margin: 0; }
</style>
