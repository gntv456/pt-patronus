<script setup lang="ts">
// 通用分区内容：主题切换、版本/更新检查、帮助/反馈/更新日志入口与弹窗。
// 状态与逻辑来自 useGeneralSettings 组合式函数（与 Settings.vue 共享单例）。
import { ref } from 'vue'
import { useGeneralSettings } from '@/composables/useGeneralSettings'
import { useUiPrefs, ACCENT_PRESETS } from '@/composables/useUiPrefs'

const {
  dark, version, updateChecking, updateInfo, updateDlgVisible, helpVisible,
  checkUpdate, copyUpgradeCmd, toggleTheme, openIssues, openChangelog,
} = useGeneralSettings()

// 界面定制（对标 MSaber 界面自定义面板）：主题色/风格/壁纸/遮罩/卡片风格，偏好随账号同步。
const { normalized, set, fetchBingDaily } = useUiPrefs()
const wallpaperFile = ref<File | null>(null)

function onWallpaperFile(f: File | null) {
  wallpaperFile.value = f
  if (!f) return
  // 本地文件转 base64 存偏好（64KB 服务端上限：超大图直接读会超，提示用网络地址）
  if (f.size > 48 * 1024) {
    ElMessage.warning('本地壁纸过大（>48KB），建议使用网络图片地址')
    return
  }
  const reader = new FileReader()
  reader.onload = () => set({ wallpaperKind: 'local', wallpaperUrl: String(reader.result) })
  reader.readAsDataURL(f)
}
</script>

<template>
  <div class="card glass">
    <div class="row">
      <div class="row-label">
        <span class="row-title">深色主题</span>
        <span class="text-muted row-hint">切换界面明暗</span>
      </div>
      <el-switch :model-value="dark" @change="toggleTheme" />
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">主题配色</span>
        <span class="text-muted row-hint">主色 / 渐变 / 强调色一键切换</span>
      </div>
      <div class="accent-picker">
        <button
          v-for="p in ACCENT_PRESETS"
          :key="p.value"
          class="accent-dot"
          :class="{ active: normalized().accent === p.value }"
          :style="{ background: p.color }"
          :title="p.label"
          @click="set({ accent: p.value })"
        />
      </div>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">界面风格</span>
        <span class="text-muted row-hint">玻璃（磨砂通透）/ 现代（实体扁平）</span>
      </div>
      <el-select :model-value="normalized().uiStyle" style="width: 120px" @change="(v: string) => set({ uiStyle: v })">
        <el-option label="玻璃" value="glass" />
        <el-option label="现代" value="modern" />
      </el-select>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">背景壁纸</span>
        <span class="text-muted row-hint">默认渐变 / 必应每日 / 网络图片 / 本地文件</span>
      </div>
      <el-select
        :model-value="normalized().wallpaperKind"
        style="width: 130px"
        @change="(v: string) => { if (v === 'bing') fetchBingDaily(); set({ wallpaperKind: v }) }"
      >
        <el-option label="默认渐变" value="none" />
        <el-option label="必应每日" value="bing" />
        <el-option label="网络图片" value="url" />
        <el-option label="本地文件" value="local" />
      </el-select>
    </div>
    <div v-if="normalized().wallpaperKind === 'url'" class="row">
      <div class="row-label">
        <span class="row-title">壁纸地址</span>
        <span class="text-muted row-hint">https:// 开头的图片直链</span>
      </div>
      <el-input
        :model-value="normalized().wallpaperUrl"
        placeholder="https://example.com/bg.jpg"
        style="width: 320px"
        @change="(v: string) => set({ wallpaperUrl: v.trim() })"
      />
    </div>
    <div v-else-if="normalized().wallpaperKind === 'local'" class="row">
      <div class="row-label">
        <span class="row-title">选择文件</span>
        <span class="text-muted row-hint">小图（≤48KB）内嵌保存；大图用网络地址</span>
      </div>
      <input type="file" accept="image/*" @change="(e: Event) => onWallpaperFile((e.target as HTMLInputElement).files?.[0] ?? null)" />
    </div>
    <div v-if="normalized().wallpaperKind !== 'none'" class="row">
      <div class="row-label">
        <span class="row-title">遮罩浓度</span>
        <span class="text-muted row-hint">壁纸上的蒙层，保证文字可读</span>
      </div>
      <el-select :model-value="normalized().mask" style="width: 120px" @change="(v: string) => set({ mask: v })">
        <el-option label="轻" value="light" />
        <el-option label="标准" value="standard" />
        <el-option label="浓" value="strong" />
      </el-select>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">卡片样式</span>
        <span class="text-muted row-hint">阴影（默认）/ 边框</span>
      </div>
      <el-select :model-value="normalized().cardStyle" style="width: 120px" @change="(v: string) => set({ cardStyle: v })">
        <el-option label="阴影" value="shadow" />
        <el-option label="边框" value="border" />
      </el-select>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">侧栏默认折叠</span>
        <span class="text-muted row-hint">进入页面时侧边栏收起为图标栏</span>
      </div>
      <el-switch :model-value="normalized().sidebarCollapsed" @change="(v: string | number | boolean) => set({ sidebarCollapsed: !!v })" />
    </div>
  </div>
  <div class="card glass">
    <div class="row about">
      <div class="row-label">
        <span class="row-title">PTPatronus</span>
        <span class="text-muted row-hint">PT 守护神 · Go + Vue3 + Flutter</span>
      </div>
      <div class="about-actions">
        <el-tag size="small" type="info" effect="plain">v{{ version }}</el-tag>
        <el-button size="small" :loading="updateChecking" @click="checkUpdate">检查更新</el-button>
      </div>
    </div>
  </div>

  <!-- 帮助 / 反馈 / 更新日志 -->
  <div class="card glass help-card">
    <div class="help-row" @click="helpVisible = true">
      <div class="row-label">
        <span class="row-title">使用帮助</span>
        <span class="text-muted row-hint">新手引导 · PT 黑话释义</span>
      </div>
      <span class="help-arrow">›</span>
    </div>
    <div class="help-row" @click="openIssues">
      <div class="row-label">
        <span class="row-title">问题反馈</span>
        <span class="text-muted row-hint">提 Issue / 建议与缺陷</span>
      </div>
      <span class="help-arrow">›</span>
    </div>
    <div class="help-row" @click="openChangelog">
      <div class="row-label">
        <span class="row-title">更新日志</span>
        <span class="text-muted row-hint">查看版本变更记录</span>
      </div>
      <span class="help-arrow">›</span>
    </div>
  </div>

  <!-- 在线更新检查弹窗 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="updateDlgVisible" :title="`发现新版本 v${updateInfo?.latestVersion ?? ''}`" width="560px">
    <div v-if="updateInfo" class="update-body">
      <p class="update-ver">当前 v{{ updateInfo.currentVersion }} → 最新 v{{ updateInfo.latestVersion }}<span v-if="updateInfo.isBelowMin" class="update-forced">（版本过低，建议立即升级）</span></p>
      <div v-if="updateInfo.releaseName" class="update-name">{{ updateInfo.releaseName }}</div>
      <div v-if="updateInfo.releaseNotes" class="update-notes">{{ updateInfo.releaseNotes }}</div>
      <template v-if="updateInfo.instructions">
        <div class="update-cmd-label">升级命令（在宿主机执行）</div>
        <pre class="update-cmd">{{ updateInfo.instructions }}</pre>
        <el-button size="small" @click="copyUpgradeCmd">复制命令</el-button>
      </template>
      <el-link v-else-if="updateInfo.downloadUrl" type="primary" :href="updateInfo.downloadUrl" target="_blank">下载更新</el-link>
    </div>
    <template #footer>
      <el-button @click="updateDlgVisible = false">{{ updateInfo?.isForced ? '我已知悉' : '稍后再说' }}</el-button>
    </template>
  </el-dialog>

  <!-- 使用帮助 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="helpVisible" title="使用帮助" width="560px">
    <div class="help-body">
      <h4>核心价值</h4>
      <p>PTPatronus 是 PT 站点保号防封利器。推荐上手路径：</p>
      <ol>
        <li>在「站点」添加并登录你的 PT 站点（填 Cookie / 账号）</li>
        <li>在「下载器」添加 qBittorrent / Transmission / uTorrent / Deluge</li>
        <li>开启「辅种」「考核」「签到」等自动化，让工具替你保级</li>
      </ol>

      <h4>PT 黑话释义</h4>
      <ul class="jargon">
        <li><b>辅种</b>：跨站辅助种子 —— 同一资源在不同站点间自动补种，提升分享率</li>
        <li><b>监护室</b>：站点健康监控 —— 实时跟踪 Cookie / H&R / 在线状态</li>
        <li><b>后宫</b>：邀请管理 —— 跟踪站内邀请与被邀请人</li>
        <li><b>站免</b>：全站免费促销 —— 聚合各站限时 Free 资源</li>
        <li><b>考核</b>：保级 / 升级条件 —— 跟踪站点对用户的硬性指标要求</li>
        <li><b>H&amp;R</b>：Hit and Run —— 站点强制的做种时长 / 比例要求，未达标会扣分</li>
      </ul>
    </div>
    <template #footer>
      <el-button @click="helpVisible = false">知道了</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.card { padding: 16px 18px; margin-bottom: 12px; }
.row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); }
.row-label { display: flex; flex-direction: column; gap: var(--sp-1); }
.row-title { font-weight: 600; }
.row-hint { font-size: var(--fs-xs); }
.accent-picker { display: flex; gap: var(--sp-2); }
.accent-dot {
  width: 22px; height: 22px; border-radius: 50%;
  border: 2px solid transparent; cursor: pointer; padding: 0;
  transition: transform var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth);
}
.accent-dot:hover { transform: scale(1.15); }
.accent-dot.active { border-color: var(--text-strong); transform: scale(1.15); }
.about-actions { display: flex; align-items: center; gap: var(--sp-2); }

.help-card { padding: 4px 18px; }
.help-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 0;
  cursor: pointer;
  border-bottom: 1px solid var(--glass-border, rgba(0, 0, 0, 0.06)); transition: background var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth)
}
.help-row:last-child { border-bottom: none; }
.help-row:hover { background: var(--pt-primary-bg); }
.help-arrow { color: var(--text-muted); font-size: var(--fs-xl); }
.help-body { line-height: 1.7; font-size: var(--fs-body); }
.help-body h4 { margin: 14px 0 6px; font-size: var(--fs-md); }
.help-body h4:first-child { margin-top: 0; }
.help-body p, .help-body ol, .help-body ul { margin: 4px 0; }
.help-body ol, .help-body ul { padding-left: 22px; }
.help-body .jargon li { margin: 4px 0; }

.update-ver { color: var(--text-muted); font-size: var(--fs-body); margin-bottom: 8px; }
.update-forced { color: var(--pt-danger); font-weight: 600; }
.update-name { font-weight: 600; margin-bottom: 6px; }
.update-notes { white-space: pre-wrap; line-height: 1.6; font-size: var(--fs-body); margin-bottom: 12px; }
.update-cmd-label { font-weight: 600; font-size: var(--fs-body); margin-bottom: 4px; }
.update-cmd {
  background: var(--pt-primary-bg);
  padding: 10px 12px; border-radius: var(--radius-xs); margin-bottom: 8px;
  font-family: ui-monospace, Menlo, Consolas, monospace; font-size: var(--fs-xs);
  white-space: pre-wrap; word-break: break-all;
}
</style>
