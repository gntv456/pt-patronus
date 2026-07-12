import 'package:flutter/material.dart';
import '../account/account_page.dart';
import '../announcements/announcements_page.dart';
import '../assessment/assessment_page.dart';
import '../backup/backup_page.dart';
import '../dashboard/dashboard_page.dart';
import '../downloaders/downloaders_page.dart';
import '../entertainment/entertainment_page.dart';
import '../file_manage/file_manage_page.dart';
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
import '../cloud/cloud_page.dart';
import '../search/search_page.dart';
import '../sites/sites_page.dart';
import '../subscriptions/subscriptions_page.dart';
import '../workflow/workflow_list_page.dart';
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

  /// 副标题释义（可选）：移动端无 hover，PT 黑话项用小字释义降低新手理解门槛（对齐 Web navDesc tooltip）。
  final String? desc;

  const NavItem({
    required this.label,
    required this.glyph,
    required this.from,
    required this.to,
    required this.builder,
    this.footer = false,
    this.desc,
  });
}

/// 导航分组结构（对齐 Web [frontend/src/layouts/MainLayout.vue] 的 navGroups）：
/// 概览 / 内容获取 / 站点活动 / 基础设施，共 4 组 20 项。
///
/// 「媒体」「种子浏览」不再作为独立导航项 —— Web 已把它们折进搜索页的
/// 种子/影视 Tab（见 search_page.dart），这里同步移除以保持双端一致。
final List<({String title, List<NavItem> items})> navGroups = [
  (
    title: '概览',
    items: [
      NavItem(label: '仪表盘', glyph: 'dashboard', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const DashboardPage()),
      NavItem(label: '公告', glyph: 'announcement', from: const Color(0xFFF7971E), to: const Color(0xFFFFD200), builder: (_) => const AnnouncementsPage()),
      NavItem(label: '消息', glyph: 'message', from: const Color(0xFF2193B0), to: const Color(0xFF6DD5ED), builder: (_) => const MessagesPage()),
    ],
  ),
  (
    title: '内容获取',
    items: [
      NavItem(label: '站点', glyph: 'sites', from: const Color(0xFF4285F4), to: const Color(0xFF22D3EE), builder: (_) => const SitesPage()),
      NavItem(label: '搜索', glyph: 'search', from: const Color(0xFFA18CD1), to: const Color(0xFFFBC2EB), builder: (_) => const SearchPage()),
      NavItem(label: '推荐', glyph: 'recommend', from: const Color(0xFFF7971E), to: const Color(0xFFFF6A00), builder: (_) => const RecommendPage()),
      NavItem(label: '订阅', glyph: 'subscribe', from: const Color(0xFFE100FF), to: const Color(0xFFFF6A00), builder: (_) => const SubscriptionsPage(), desc: '追剧自动推送'),
      NavItem(label: 'RSS', glyph: 'rss', from: const Color(0xFFF7971E), to: const Color(0xFFFF6A00), builder: (_) => const RssPage(), desc: '资源源订阅'),
      NavItem(label: '辅种', glyph: 'seeding', from: const Color(0xFF7F00FF), to: const Color(0xFF38EF7D), builder: (_) => const ReseedPage(), desc: '跨站辅助种子'),
    ],
  ),
  (
    title: '站点活动',
    items: [
      NavItem(label: '考核', glyph: 'assessment', from: const Color(0xFF0F9B0F), to: const Color(0xFF43CEA2), builder: (_) => const AssessmentPage(), desc: '保级 / 升级条件'),
      NavItem(label: '站免', glyph: 'freeleech', from: const Color(0xFF11998E), to: const Color(0xFF38EF7D), builder: (_) => const FreeleechPage(), desc: '全站免费促销'),
      NavItem(label: '后宫', glyph: 'harem', from: const Color(0xFFE100FF), to: const Color(0xFF7F00FF), builder: (_) => const HaremPage(), desc: '邀请管理'),
      NavItem(label: '勋章墙', glyph: 'medalwall', from: const Color(0xFFF39C12), to: const Color(0xFFE100FF), builder: (_) => const MedalWallPage(), desc: '勋章收集'),
      NavItem(label: '娱乐', glyph: 'entertainment', from: const Color(0xFF7F00FF), to: const Color(0xFFE100FF), builder: (_) => const EntertainmentPage(), desc: '站内小游戏'),
      NavItem(label: '监护室', glyph: 'savept', from: const Color(0xFFE74C3C), to: const Color(0xFFF39C12), builder: (_) => const SaveptPage(), desc: '站点健康监控'),
      NavItem(label: '刷流', glyph: 'brushflow', from: const Color(0xFFF7971E), to: const Color(0xFFFF6A00), builder: (_) => const BrushFlowPage(), desc: '自动刷量'),
      NavItem(label: '保种契约', glyph: 'contract', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const ContractPage(), desc: '分享率/做种下限告警'),
    ],
  ),
  (
    title: '基础设施',
    items: [
      NavItem(label: '下载器', glyph: 'downloaders', from: const Color(0xFF11998E), to: const Color(0xFF38EF7D), builder: (_) => const DownloadersPage()),
      NavItem(label: '媒体服务器', glyph: 'mediaserver', from: const Color(0xFF52B788), to: const Color(0xFF2D6A4F), builder: (_) => const MediaServerPage(), desc: 'Emby/Jellyfin/Plex'),
      NavItem(label: '云盘·STRM', glyph: 'cloud', from: const Color(0xFF4CC9F0), to: const Color(0xFF4361EE), builder: (_) => const CloudPage(), desc: '115/夸克/123 浏览+STRM'),
      NavItem(label: '文件', glyph: 'files', from: const Color(0xFFF7971E), to: const Color(0xFFFFD200), builder: (_) => const FileManagePage()),
      NavItem(label: 'NAS', glyph: 'server', from: const Color(0xFF0F2027), to: const Color(0xFF2C5364), builder: (_) => const MonitorPage()),
      NavItem(label: '工作流', glyph: 'workflow', from: const Color(0xFF6C5CE7), to: const Color(0xFF00B894), builder: (_) => const WorkflowListPage()),
      NavItem(label: '备份', glyph: 'backup', from: const Color(0xFF536976), to: const Color(0xFF2C3E50), builder: (_) => const BackupPage()),
    ],
  ),
];

/// 底部固定分组（对齐 Docker 端「设置/账户」区，移动端无顶栏用户菜单，故放侧栏底）。
final List<NavItem> navFooter = [
  NavItem(label: '通知设置', glyph: 'settings', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const NotificationSettingsPage()),
  NavItem(label: '运行日志', glyph: 'history', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const LogsPage()),
  NavItem(label: '用户中心', glyph: 'shield', from: const Color(0xFF6C5CE7), to: const Color(0xFF4285F4), builder: (_) => const AccountPage()),
];

/// 扁平全量列表（含 footer），供 [HomeController.index] 索引。
/// 顺序与 [navGroups] 渲染顺序一致：4 组主项 + footer。
final List<NavItem> navItems = [
  ...navGroups.expand((g) => g.items),
  ...navFooter,
];
