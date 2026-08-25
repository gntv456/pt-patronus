<script setup lang="ts">
// CookieCloud 同步配置面板。
// 状态与逻辑来自 useCookieCloud 组合式函数。
// mode：external=外部服务器；builtin=内置服务端（PTPatronus 自身充当，浏览器扩展直推）。
import { computed } from 'vue'
import { useCookieCloud } from '@/composables/useCookieCloud'
import { copyText } from '@/utils/clipboard'

const {
  ccCfg, ccForm, ccLoading, ccSaving, ccSyncing,
  ccGeneratedPassword, ccBuiltinEndpoint,
  saveCookieCloud, regenerateBuiltinCreds, syncCookieCloud, pushCookieCloud,
} = useCookieCloud()

const isBuiltin = computed(() => ccForm.value.mode === 'builtin')
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

            <div class="mt8" style="display:flex;align-items:center;gap:var(--sp-3)">
              <span class="text-muted" style="min-width:7em">服务端模式</span>
              <el-radio-group v-model="ccForm.mode">
                <el-radio-button value="builtin">内置服务端（推荐）</el-radio-button>
                <el-radio-button value="external">外部服务器</el-radio-button>
              </el-radio-group>
            </div>
            <p class="text-muted" style="margin:6px 0 0;font-size: var(--fs-xs)">
              内置服务端 = PTPatronus 自己充当 CookieCloud 服务器，浏览器扩展直接推 cookie 过来，<b>无需再部署 CookieCloud 容器</b>；推送后立即同步。外部服务器 = 连自建/公共 CookieCloud。
            </p>

            <!-- builtin：扩展配置指引 -->
            <template v-if="isBuiltin">
              <div class="mt8 builtin-guide">
                <div class="guide-title">浏览器扩展这样填（安装 CookieCloud 官方扩展后）</div>
                <div class="guide-row">
                  <span class="guide-label">服务器</span>
                  <code class="guide-val">{{ ccBuiltinEndpoint }}</code>
                  <el-button size="small" text type="primary" @click="copyText(ccBuiltinEndpoint, '地址已复制', '复制失败，请手动选中复制')">复制</el-button>
                </div>
                <div class="guide-row">
                  <span class="guide-label">UUID</span>
                  <code class="guide-val">{{ ccCfg?.uuid || ccForm.uuid || '（保存后自动生成）' }}</code>
                  <el-button v-if="ccCfg?.uuid" size="small" text type="primary" @click="copyText(ccCfg!.uuid, 'UUID 已复制', '复制失败，请手动选中复制')">复制</el-button>
                </div>
                <div class="guide-row">
                  <span class="guide-label">密码</span>
                  <code class="guide-val">{{ ccGeneratedPassword || (ccCfg?.password_configured ? '已配置（重新生成可见）' : '（保存后自动生成）') }}</code>
                  <el-button v-if="ccGeneratedPassword" size="small" text type="primary" @click="copyText(ccGeneratedPassword, '密码已复制', '复制失败，请手动选中复制')">复制</el-button>
                </div>
                <p class="text-muted" style="margin:8px 0 0;font-size: var(--fs-xs)">
                  首次切到内置模式点「保存」自动生成 UUID/密码（密码只显示一次）；扩展填好后点「上传」即推送，PTPatronus 收到后立即同步。
                </p>
                <el-button size="small" class="mt8" @click="regenerateBuiltinCreds">重新生成凭证</el-button>
              </div>
            </template>

            <!-- external：原有 server/uuid/password 表单 -->
            <template v-else>
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
            </template>

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
.builtin-guide { border: 1px solid var(--border-color, rgba(255,255,255,.12)); border-radius: 8px; padding: 12px; }
.guide-title { font-weight: 600; margin-bottom: 8px; }
.guide-row { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
.guide-label { min-width: 3.5em; color: var(--text-muted); font-size: var(--fs-xs); }
.guide-val { flex: 1; font-size: var(--fs-xs); word-break: break-all; background: var(--glass-bg, rgba(0,0,0,.08)); padding: 2px 8px; border-radius: 4px; }
</style>
