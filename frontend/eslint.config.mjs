// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// ESLint 9 扁平配置。两大职责：
//   1. i18n 防回潮门禁（no-raw-chinese 自定义规则，限定已迁移页面）
//   2. TypeScript 推荐规则的「告警级」子集（no-explicit-any / no-floating-promises
//      等先 warn 防增量，存量清零后再升 error——工程化审计 P2）。
import pluginVue from 'eslint-plugin-vue'
import pluginTs from '@typescript-eslint/eslint-plugin'
import tsParser from '@typescript-eslint/parser'

// 已完成 t() 化、非注释中文必须清零的文件（相对 src/）。迁移新页面后加入此列表。
const I18N_DONE = [
  'views/Login.vue',
  'views/Settings.vue',
  'views/Dashboard.vue',
  'views/Sites.vue',
  'views/Search.vue',
  'views/Downloaders.vue',
  'views/Subscriptions.vue',
  'views/Notices.vue',
  'views/Rss.vue',
  'views/Messages.vue',
  'views/Announcements.vue',
  'views/Savept.vue',
  'views/Assessment.vue',
  'views/Freeleech.vue',
  'views/MedalWall.vue',
  'views/Reseed.vue',
  'views/Contract.vue',
  'views/BrushFlow.vue',
  'views/Entertainment.vue',
  'views/Assistant.vue',
  'views/Recommend.vue',
  'views/Transfer.vue',
  'views/MediaServer.vue',
  'views/OpsCenter.vue',
  'views/PluginView.vue',
  'views/Harem.vue',
  'views/Workflow.vue',
  'views/Backup.vue',
  'views/Doctor.vue',
  'views/FileManager.vue',
  'views/Subtitle.vue',
  'views/MediaQuality.vue',
  'views/Trakt.vue',
  'views/FilterRules.vue',
  'views/MediaSettings.vue',
  'views/NasServer.vue',
  'components/ops/OpsMonitor.vue',
  'components/ops/OpsDatabase.vue',
  'components/ops/OpsProcess.vue',
  'components/ops/OpsDisk.vue',
  'views/Cloud.vue',
  'views/WorkflowEditor.vue',
  'views/SiteDetail.vue',
  'views/PluginMarketSection.vue',
  'views/PluginAuditDialog.vue',
  'views/Plugins.vue',
  'views/MediaDetail.vue',
  'views/Recommend.vue',
  'views/Search.vue',
  'views/Assistant.vue',
  'views/SiteRequests.vue',
  'views/Dashboard.vue',
  'views/Savept.vue',
  'views/Subscriptions.vue',
  'views/Freeleech.vue',
  'views/PluginLogsDialog.vue',
  'views/Notices.vue',
  'views/Login.vue',
  'views/MediaServer.vue',
  'components/ops/OpsDocker.vue',
  'components/downloader/TorrentTable.vue',
  'components/sites/SiteMatrixTable.vue',
  'components/monitor/DeviceForm.vue',
  'components/settings/NotificationPanel.vue',
  'components/ops/DockerCreateWizard.vue',
  'components/settings/GeneralPanel.vue',
  'components/FileManagerDialogs.vue',
  'components/dashboard/DashboardExportDialog.vue',
  'components/downloader/TorrentDetail.vue',
  'components/ops/OpsSsh.vue',
  'components/dashboard/DashboardWorkbench.vue',
  'components/monitor/DeviceHostDetail.vue',
  'components/settings/CookieCloudPanel.vue',
  'components/ops/DockerDetailDrawer.vue',
  'components/BigSmallGame.vue',
  'components/LongPTMedalGame.vue',
  'components/PandaFlipGame.vue',
  'components/EventBet.vue',
  'components/FilterRuleDialogs.vue',
  'components/FileManagerExtraDialogs.vue',
  'components/SpinnerGame.vue',
  'components/LongPTWheelGame.vue',
  'components/MagicFarmGame.vue',
  'components/LongPTDailyGame.vue',
  'components/JggGameUi.vue',
  'components/search/TorrentCard.vue',
  'components/search/TorrentFindMoreDialog.vue',
  'components/MediaResultsGrid.vue',
  'components/dashboard/DashboardTrendChart.vue',
  'components/dashboard/DashboardActivityTimeline.vue',
  'components/plugins/PluginActionDialog.vue',
  'components/CelebrityDialog.vue',
  'components/savept/SaveptSiteCard.vue',
  'components/ops/DockerComposeLocal.vue',
  'components/downloader/TorrentDrawer.vue',
  'components/ops/OpsTerminal.vue',
  'components/monitor/DeviceCard.vue',
  'components/savept/SaveptHeader.vue',
  'components/nas/NasHelpDialog.vue',
  'components/nas/NasCredentialDrawer.vue',
  'components/nas/NasConfigDrawer.vue',
  'components/downloader/AddTorrentPanel.vue',
  'components/dashboard/DashboardTaskCards.vue',
  'components/monitor/AgentManager.vue',
  'components/ops/DockerAppStore.vue',
  'components/ops/ComposeProjects.vue',
  'components/SearchPushDialog.vue',
  'components/nas/NasDeployDrawer.vue',
  'components/dashboard/DashboardAutomationCard.vue',
  'components/dashboard/DashboardQuickTools.vue',
  'components/TorrentResultsTable.vue',
  'components/settings/OcrPanel.vue',
  'components/workflow/WorkflowNode.vue',
  'components/settings/TgBotPanel.vue',
  'components/sites/SiteOperationsMatrix.vue',
  'components/settings/LlmGatewayPanel.vue',
  'components/settings/LogsPanel.vue',
  'components/settings/OidcPanel.vue',
  'components/settings/SchedulePanel.vue',
  'components/sites/SiteMatrixHistoryDialog.vue',
  'components/workflow/NodeParamForm.vue',
  'components/dashboard/DashboardAICard.vue',
  'components/dashboard/DashboardHealthStrip.vue',
  'components/dashboard/DashboardRealtimeCard.vue',
  'components/dashboard/DashboardResourceCard.vue',
  'components/dashboard/DashboardSyncCard.vue',
  'components/dashboard/DashboardWidgets.vue',
  'components/dashboard/DashboardTrendSection.vue',
  'components/monitor/AlertList.vue',
  'components/monitor/DeviceCameraDetail.vue',
  'components/monitor/DeviceServiceDetail.vue',
  'components/downloader/StatsBar.vue',
  'components/nas/NasExecPanel.vue',
  'components/nas/NasTerminalDrawer.vue',
  'components/nas/NasDeployWorkspace.vue',
  'components/nas/NasOpsGrid.vue',
  'components/ops/DockerCreateDialog.vue',
  'components/savept/SaveptAnnivCalendarDialog.vue',
  'components/search/TorrentPushDialog.vue',
  'components/SearchFilterBar.vue',
  'components/CommandPalette.vue',
  'components/GlobalNoticeBar.vue',
  'components/ReseedMissingDialog.vue',
  'components/SiteConnectivityStatus.vue',
  'components/SiteFilterDialog.vue',
  'components/SiteTorrentFindDialog.vue',
  'components/TorrentDetailDrawer.vue',
  'components/BlackjackGame.vue',
  'components/BetGame.vue',
  'components/BigSmallSidebar.vue',
  'components/MagicScratchGame.vue',
  'components/MedalSynthesisGame.vue',
  'components/ScratchCardGame.vue',
  'components/NovaHDLuckyGame.vue',
  'components/NovaHDScratchGame.vue',
  'layouts/MainLayout.vue',
  'views/PluginActionDialog.vue',
  'views/PluginConfigDialog.vue',
  'layouts/AppSidebar.vue',
  'components/ops/DockerExecTerminal.vue',
  'components/downloader/FilesDialog.vue',
  'components/downloader/TrackersDialog.vue',
  'components/monitor/RtspPlayer.vue',
  'components/monitor/DeviceDetail.vue',
  'components/NotificationCenter.vue',
  'components/savept/SaveptNotices.vue',
  'components/dashboard/DashboardRingChart.vue',
  'components/monitor/MetricChart.vue',
  'components/EmptyState.vue',
  'components/PluginSchemaField.vue',
  'components/dashboard/Sparkline.vue',
]

// no-raw-chinese：禁止在已迁移组件的 script/模板中硬编码含 CJK 的字符串字面量。
// 豁免：注释（// 与 <!--）、以及 `t('key', '中文兜底')` 的第二参数（两段式回退文案）。
const noRawChinese = {
  meta: {
    type: 'problem',
    docs: { description: '禁止在已迁移 i18n 的组件中硬编码中文文案（防回潮）' },
    schema: [],
    messages: {
      rawChinese: "检测到硬编码中文文案「{{text}}」：请改用 t('key', 兜底文案) 并把文案收进 src/i18n/locales/。",
    },
  },
  create(context) {
    const sourceCode = context.sourceCode ?? context.getSourceCode()
    const comments = new Set()
    for (const c of sourceCode.getAllComments?.() ?? []) {
      for (let i = c.loc.start.line; i <= c.loc.end.line; i++) comments.add(i)
    }
    const isFallbackArg = (node) => {
      // t('key', '中文兜底')：父 CallExpression 的第二个起参数
      const parent = node.parent
      if (!parent || parent.type !== 'CallExpression') return false
      const callee = sourceCode.getText(parent.callee)
      return /\bt\s*$/.test(callee) && parent.arguments.includes(node) && parent.arguments[0] !== node
    }
    const check = (node, raw) => {
      if (!raw || !/[\u4e00-\u9fff]/.test(raw)) return
      if (comments.has(node.loc.start.line)) return
      if (isFallbackArg(node)) return
      const text = raw.replace(/^['"`]/, '').replace(/['"`]$/, '').slice(0, 30)
      context.report({ node, messageId: 'rawChinese', data: { text } })
    }
    return {
      Literal: (node) => { if (typeof node.value === 'string') check(node, node.raw) },
      TemplateLiteral: (node) => check(node, sourceCode.getText(node)),
      VLiteral: (node) => check(node, `'${node.value}'`),
    }
  },
}

// vue 官方 flat 配置默认带旧式 processor（把 SFC 拆成虚拟 JS 文件交给 espree），
// 会绕过 vue-eslint-parser 并在 TS 语法上报 Parsing error。这里去掉 processor，
// 让 vue-eslint-parser 直接解析整个 SFC，script 块再由 @typescript-eslint/parser 接管。
const [vueSetup, ...vueRules] = pluginVue.configs['flat/essential']
const { processor: _drop, ...vueSetupNoProcessor } = vueSetup

export default [
  vueSetupNoProcessor,
  ...vueRules,
  {
    files: ['*.vue', '**/*.vue'],
    languageOptions: { parserOptions: { parser: tsParser, ecmaVersion: 'latest' } },
  },
  {
    files: I18N_DONE.map((f) => `src/${f}`),
    plugins: { 'pt-i18n': { rules: { 'no-raw-chinese': noRawChinese } } },
    rules: { 'pt-i18n/no-raw-chinese': 'error' },
  },
  {
    rules: {
      // 项目既有大量单词文件名（Sites.vue 等），非本次门禁目标
      'vue/multi-word-component-names': 'off',
      'vue/require-toggle-inside-transition': 'off',
    },
  },
  // TS 规则集（warn 级防增量）：只挑无需类型信息的规则——type-aware 项
  // （no-floating-promises 等）需要 projectService 全量类型图，lint 时长会翻数倍，
  // 留待后续单独开关（工程化审计 P2）。
  {
    files: ['src/**/*.ts'],
    plugins: { '@typescript-eslint': pluginTs },
    languageOptions: { parser: tsParser, ecmaVersion: 'latest', sourceType: 'module' },
    rules: {
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-non-null-assertion': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      '@typescript-eslint/prefer-as-const': 'warn',
      '@typescript-eslint/no-var-requires': 'warn',
    },
  },
  {
    // .vue 的 script 走 vue-eslint-parser（其 parserOptions.parser=tsParser），
    // 这里不设 languageOptions.parser，否则 SFC 模板部分解析失败。
    files: ['src/**/*.vue'],
    plugins: { '@typescript-eslint': pluginTs },
    rules: {
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-non-null-assertion': 'warn',
      '@typescript-eslint/prefer-as-const': 'warn',
    },
  },
  { ignores: ['dist/', 'node_modules/', 'test-results/', 'e2e/.data/', 'auto-imports.d.ts', 'components.d.ts'] },
]
