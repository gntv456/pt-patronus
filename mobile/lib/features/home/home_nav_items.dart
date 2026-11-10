// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import '../account/account_page.dart';
import '../announcements/announcements_page.dart';
import '../assistant/assistant_page.dart';
import '../assessment/assessment_page.dart';
import '../backup/backup_page.dart';
import '../doctor/doctor_page.dart';
import '../dashboard/dashboard_page.dart';
import '../downloaders/downloaders_page.dart';
import '../entertainment/entertainment_page.dart';
import '../file_manage/file_manage_page.dart';
import '../filter_rules/filter_rules_page.dart';
import '../freeleech/freeleech_page.dart';
import '../harem/harem_page.dart';
import '../medal_wall/medal_wall_page.dart';
import '../messages/messages_page.dart';
import '../monitor/monitor_page.dart';
import '../notification/notification_settings_page.dart';
import '../recommend/recommend_page.dart';
import '../reseed/reseed_page.dart';
import '../rss/rss_page.dart';
import '../savept/savept_page.dart';
import '../brushflow/brushflow_page.dart';
import '../contract/contract_page.dart';
import '../media_server/media_server_page.dart';
import '../media_quality/media_quality_page.dart';
import '../cloud/cloud_page.dart';
import '../subtitle/subtitle_page.dart';
import '../transfer/transfer_page.dart';
import '../trakt/trakt_page.dart';
import '../search/search_page.dart';
import '../sites/sites_page.dart';
import '../subscriptions/subscriptions_page.dart';
import '../workflow/workflow_list_page.dart';
import '../system_settings/system_settings_page.dart';
import '../system/logs_page.dart';

/// 一条主菜单目的地。
///
/// 图标用 [GlassIcon]（彩色渐变圆角块 + 白色 Lucide 字形，对齐 Web GlassIcon）。
/// `glyph` 是 [kGlyphs] 的 key；`from`/`to` 是 135° 渐变配色（与 Web MainLayout.vue 逐项一致）。
class NavItem {
  final String label;
  final String glyph;
  final Color from;
  final Color to;
  final WidgetBuilder builder;

  /// true → 固定渲染在抽屉/侧栏底部分组（通知设置、用户中心），不随主列表滚动。
  final bool footer;

  /// true → 不在抽屉/侧栏渲染，但仍占 [navItems] 索引位（供顶栏入口 select，保持 IndexedStack 保活）。
  /// 用途：AI 助手已移至顶栏常驻（对齐 Web [MainLayout.vue] 顶栏 🤖），菜单中隐藏。
  final bool hidden;

  /// 副标题释义（可选）：移动端无 hover，PT 黑话项用小字释义降低新手理解门槛（对齐 Web navDesc tooltip）。
  final String? desc;

  const NavItem({
    required this.label,
    required this.glyph,
    required this.from,
    required this.to,
    required this.builder,
    this.footer = false,
    this.hidden = false,
    this.desc,
  });
}

/// 导航分组结构（对齐 Web [frontend/src/layouts/MainLayout.vue] 的 navGroups）：
/// P2 对齐 Web：侧栏仅显示定稿主菜单；移出项 hidden 保留 IndexedStack 与深链。
///
/// 「媒体」「种子浏览」不再作为独立导航项 —— Web 已把它们折进搜索页的
/// 种子/影视 Tab（见 search_page.dart），这里同步移除以保持双端一致。
final List<({String title, List<NavItem> items})> navGroups = [
  (
    title: '概览',
    items: [
      NavItem(label: '仪表盘', glyph: 'dashboard', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const DashboardPage()),
      NavItem(label: 'AI 助手', glyph: 'bot', from: const Color(0xFF6C5CE7), to: const Color(0xFFA855F7), builder: (_) => const AssistantPage(), hidden: true, desc: '自然语言控制全平台'),
      NavItem(label: '推荐', glyph: 'recommend', from: const Color(0xFFF7971E), to: const Color(0xFFFF6A00), builder: (_) => const RecommendPage()),
      NavItem(label: '订阅', glyph: 'subscribe', from: const Color(0xFFE100FF), to: const Color(0xFFFF6A00), builder: (_) => const SubscriptionsPage(), desc: '追剧自动推送'),
      NavItem(label: '站点', glyph: 'sites', from: const Color(0xFF4285F4), to: const Color(0xFF22D3EE), builder: (_) => const SitesPage()),
    ],
  ),
  (
    title: '动态',
    items: [
      NavItem(label: '消息', glyph: 'message', from: const Color(0xFF2193B0), to: const Color(0xFF6DD5ED), builder: (_) => const MessagesPage()),
      NavItem(label: '公告', glyph: 'announcement', from: const Color(0xFFF7971E), to: const Color(0xFFFFD200), builder: (_) => const AnnouncementsPage()),
    ],
  ),
  (
    title: '内容获取',
    items: [
      NavItem(label: '搜索', glyph: 'search', from: const Color(0xFFA18CD1), to: const Color(0xFFFBC2EB), builder: (_) => const SearchPage(), hidden: true),
      NavItem(label: 'RSS', glyph: 'rss', from: const Color(0xFFF7971E), to: const Color(0xFFFF6A00), builder: (_) => const RssPage(), desc: '资源源订阅', hidden: true),
      NavItem(label: '整理', glyph: 'files', from: const Color(0xFF6C5CE7), to: const Color(0xFF00B894), builder: (_) => const TransferPage(), desc: '下载完成自动整理入库', hidden: true),
      NavItem(label: '过滤规则', glyph: 'filter', from: const Color(0xFF5B86E5), to: const Color(0xFF36D1DC), builder: (_) => const FilterRulesPage(), desc: '资源筛选优先级规则', hidden: true),
      NavItem(label: '辅种', glyph: 'seeding', from: const Color(0xFF7F00FF), to: const Color(0xFF38EF7D), builder: (_) => const ReseedPage(), desc: '跨站辅助种子', hidden: true),
    ],
  ),
  (
    title: '站点活动',
    items: [
      NavItem(label: '娱乐', glyph: 'entertainment', from: const Color(0xFF7F00FF), to: const Color(0xFFE100FF), builder: (_) => const EntertainmentPage(), desc: '站内小游戏'),
      NavItem(label: '勋章墙', glyph: 'medalwall', from: const Color(0xFFF39C12), to: const Color(0xFFE100FF), builder: (_) => const MedalWallPage(), desc: '勋章收集'),
      NavItem(label: '后宫', glyph: 'harem', from: const Color(0xFFE100FF), to: const Color(0xFF7F00FF), builder: (_) => const HaremPage(), desc: '邀请管理'),
      NavItem(label: '考核', glyph: 'assessment', from: const Color(0xFF0F9B0F), to: const Color(0xFF43CEA2), builder: (_) => const AssessmentPage(), desc: '保级 / 升级条件', hidden: true),
      NavItem(label: '站免', glyph: 'freeleech', from: const Color(0xFF11998E), to: const Color(0xFF38EF7D), builder: (_) => const FreeleechPage(), desc: '全站免费促销', hidden: true),
      NavItem(label: '监护室', glyph: 'savept', from: const Color(0xFFE74C3C), to: const Color(0xFFF39C12), builder: (_) => const SaveptPage(), desc: '站点健康监控', hidden: true),
      NavItem(label: '刷流', glyph: 'brushflow', from: const Color(0xFFF7971E), to: const Color(0xFFFF6A00), builder: (_) => const BrushFlowPage(), desc: '自动刷量', hidden: true),
      NavItem(label: '保种契约', glyph: 'contract', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const ContractPage(), desc: '分享率/做种下限告警', hidden: true),
    ],
  ),
  (
    title: '基础设施',
    items: [
      NavItem(label: '下载器', glyph: 'downloaders', from: const Color(0xFF11998E), to: const Color(0xFF38EF7D), builder: (_) => const DownloadersPage()),
      NavItem(label: '媒体服务器', glyph: 'mediaserver', from: const Color(0xFF52B788), to: const Color(0xFF2D6A4F), builder: (_) => const MediaServerPage(), desc: 'Emby/Jellyfin/Plex'),
      NavItem(label: '文件', glyph: 'files', from: const Color(0xFFF7971E), to: const Color(0xFFFFD200), builder: (_) => const FileManagePage()),
      NavItem(label: 'NAS', glyph: 'server', from: const Color(0xFF0F2027), to: const Color(0xFF2C5364), builder: (_) => const MonitorPage()),
      NavItem(label: '云盘·STRM', glyph: 'cloud', from: const Color(0xFF4CC9F0), to: const Color(0xFF4361EE), builder: (_) => const CloudPage(), desc: '115/夸克/123 浏览+STRM', hidden: true),
      NavItem(label: '字幕', glyph: 'subtitle', from: const Color(0xFF4361EE), to: const Color(0xFF7209B7), builder: (_) => const SubtitlePage(), desc: '跨源字幕搜索下载', hidden: true),
      NavItem(label: 'Trakt', glyph: 'trakt', from: const Color(0xFFB5179E), to: const Color(0xFFF72585), builder: (_) => const TraktPage(), desc: 'WatchList→订阅', hidden: true),
      NavItem(label: '媒体质量', glyph: 'mediaquality', from: const Color(0xFFF12711), to: const Color(0xFFF5AF19), builder: (_) => const MediaQualityPage(), desc: '多评分聚合+低分保护', hidden: true),
      NavItem(label: '备份', glyph: 'backup', from: const Color(0xFF536976), to: const Color(0xFF2C3E50), builder: (_) => const BackupPage(), hidden: true),
      NavItem(label: '诊断', glyph: 'stethoscope', from: const Color(0xFF0F2027), to: const Color(0xFFEF4444), builder: (_) => const DoctorPage(), hidden: true),
      NavItem(label: '系统设置', glyph: 'settings', from: const Color(0xFF485563), to: const Color(0xFF29323C), builder: (_) => const SystemSettingsPage(), desc: 'AI 网关 / Telegram / 目录监视'),
    ],
  ),
  (
    title: '自动化',
    items: [
      NavItem(label: '工作流', glyph: 'workflow', from: const Color(0xFF6C5CE7), to: const Color(0xFF00B894), builder: (_) => const WorkflowListPage()),
    ],
  ),
];

/// 底部固定分组（对齐 Docker 端「设置/账户」区，移动端无顶栏用户菜单，故放侧栏底）。
final List<NavItem> navFooter = [
  NavItem(label: '通知设置', glyph: 'settings', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const NotificationSettingsPage()),
  NavItem(label: '运行日志', glyph: 'history', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const LogsPage(), hidden: true),
  NavItem(label: '用户中心', glyph: 'shield', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const AccountPage()),
];

/// 扁平全量列表（含 footer），供 [HomeController.index] 索引。
/// 顺序与 [navGroups] 渲染顺序一致：6 组主项（含 hidden）+ footer。
final List<NavItem> navItems = [
  ...navGroups.expand((g) => g.items),
  ...navFooter,
];
