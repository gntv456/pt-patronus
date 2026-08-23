<script setup lang="ts">
// CookieCloud 同步配置面板。
// 状态与逻辑来自 useCookieCloud 组合式函数。
import { useCookieCloud } from '@/composables/useCookieCloud'

const { ccCfg, ccForm, ccLoading, ccSaving, ccSyncing, saveCookieCloud, syncCookieCloud, pushCookieCloud } = useCookieCloud()
</script>

<template>
        <div v-loading="ccLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            浏览器装 CookieCloud 扩展并填服务器/UUID/密码后，PTPatronus 定时拉取最新 cookie，
            按域名同步浏览器 cookie：默认可仅覆盖本地 <b>已失效</b> 站点，也可选总是覆盖；并自动新增匹配内置定义的站点。配合小时级 cookie 保活，根治 session 短寿站掉线。
          </p>
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">启用 CookieCloud 同步</div>
              <el-switch v-model="ccForm.enabled" />
            </div>
            <el-input v-model="ccForm.server" class="mt8" placeholder="https://cc.example.com">
              <template #prepend>服务器</template>
            </el-input>
            <el-input v-model="ccForm.uuid" class="mt8" placeholder="UUID / Key">
              <template #prepend>UUID</template>
            </el-input>
            <el-input
              v-model="ccForm.password"
              type="password"
              show-password
              class="mt8"
              :placeholder="ccCfg?.password_configured ? '已配置（留空=不修改）' : '端到端密码'"
            >
              <template #prepend>密码</template>
            </el-input>
            <el-input v-model.number="ccForm.interval_min" type="number" class="mt8" placeholder="120">
              <template #prepend>间隔(分钟)</template>
            </el-input>
            <div class="mt8" style="display:flex;align-items:center;gap:var(--sp-3)">
              <span class="text-muted" style="min-width:7em">更新策略</span>
              <el-select v-model="ccForm.update_mode" style="flex:1">
                <el-option label="仅覆盖已失效站点（推荐）" value="disconnected" />
                <el-option label="总是用浏览器 cookie 覆盖" value="always" />
              </el-select>
            </div>
            <p class="text-muted" style="margin:6px 0 0;font-size: var(--fs-xs)">
              「仅失效」对齐 MoviePilot，保护本地手填有效 cookie；「总是覆盖」更接近浏览器 CookieCloud 实时同步，适合 session 短寿站。
            </p>
            <div class="mt8" style="display:flex;align-items:center;gap:var(--sp-3)">
              <span class="text-muted" style="min-width:7em">同步方向</span>
              <el-select v-model="ccForm.direction" style="flex:1">
                <el-option label="双向：先拉后推（推荐）" value="both" />
                <el-option label="仅拉取（云端 → 本地）" value="pull" />
                <el-option label="仅推送（本地 → 云端）" value="push" />
              </el-select>
            </div>
            <p class="text-muted" style="margin:6px 0 0;font-size: var(--fs-xs)">
              推送会把本端启用站 cookie 合并进 CookieCloud（保留云端其它 host），供浏览器/其它实例同步。
            </p>
          </div>
          <div class="oidc-save">
            <span v-if="ccCfg?.last_error" class="text-muted" style="margin-right: 12px">
              上次错误：{{ ccCfg.last_error }}
            </span>
            <span v-else-if="ccCfg?.last_sync_unix" class="text-muted" style="margin-right: 12px">
              上次同步：{{ new Date(ccCfg.last_sync_unix * 1000).toLocaleString() }}
            </span>
            <el-button :loading="ccSyncing" @click="syncCookieCloud">立即同步</el-button>
            <el-button :loading="ccSyncing" @click="pushCookieCloud">仅推送</el-button>
            <el-button type="primary" :loading="ccSaving" @click="saveCookieCloud">保存</el-button>
          </div>
        </div>
</template>

<style scoped>
.oidc-wrap { display: flex; flex-direction: column; gap: var(--sp-3); max-width: 640px; }
.oidc-help { font-size: var(--fs-body); line-height: 1.6; }
.oidc-save { display: flex; justify-content: flex-end; }
.ch-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.mt8 { margin-top: 8px; }
.card { padding: 16px 18px; margin-bottom: 12px; }
</style>
