// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';
import 'dart:convert';
import 'dart:io' show File, Platform;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart'
    show TargetPlatform, defaultTargetPlatform;
import 'package:flutter/material.dart';
import 'package:ptpatronus/core/platform/file_picker.dart';
import 'package:ptpatronus/core/platform/platform_caps.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_nav_items.dart';
import 'package:flutter/rendering.dart' show RenderRepaintBoundary;
import 'package:flutter/services.dart' show MissingPluginException;
import 'package:path_provider/path_provider.dart';
import 'package:ptpatronus/api/agent.dart';
import 'package:ptpatronus/api/brushflow.dart';
import 'package:ptpatronus/api/contract.dart';
import 'package:ptpatronus/api/dashboard.dart';
// #403 任意异常 → 用户可读文案（后端每个错误码都带着可操作的中文，别在 catch 里丢掉）。
import 'package:ptpatronus/core/err_text.dart';
import 'package:ptpatronus/features/dashboard/dashboard_extras.dart';
import 'package:ptpatronus/features/dashboard/dashboard_history_body.dart';
import 'package:ptpatronus/features/dashboard/dashboard_settings_sheet.dart';
import 'package:ptpatronus/features/dashboard/dashboard_share_models.dart';
import 'package:ptpatronus/features/dashboard/total_dashboard_config.dart';
import 'package:ptpatronus/api/notice.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/system.dart';
import 'package:ptpatronus/api/workflow.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:share_plus/share_plus.dart';
import 'package:shared_preferences/shared_preferences.dart';

part 'dashboard_history_actions.dart';
part 'dashboard_calc.dart';
part 'dashboard_data.dart';
part 'dashboard_sections.dart';
part 'dashboard_sites_section.dart';
part 'dashboard_site_cards.dart';
part 'dashboard_share_card.dart';
part 'dashboard_share_rank.dart';
part 'dashboard_share_site_cards.dart';
part 'dashboard_share_render.dart';

const _totalDashboardConfigKey = 'total_dashboard_config_v1';

extension _ChunkedList<T> on List<T> {
  List<List<T>> chunked(int size) {
    final out = <List<T>>[];
    for (var i = 0; i < length; i += size) {
      out.add(sublist(i, (i + size > length) ? length : i + size));
    }
    return out;
  }
}

enum AlertKind { cookie, hr }

// 仪表盘：跨站统计汇总（指标卡墙，含 sparkline）+ 站点明细（宽屏表格 / 窄屏卡片）。
// 对齐 Web Dashboard.vue：5 张指标卡（上传/下载/做种/做种体积/魔力）+ 近 14 天 sparkline +
// 站点表格（站点/等级/上传/下载/分享率/做种/操作）+ 单站历史数据 + 单站/全站刷新。
// 数据来自 GET /dashboard、GET /dashboard/trend?days=14、POST /dashboard/sync[/:id]、GET /dashboard/history/:id。
class DashboardPage extends StatefulWidget {
  const DashboardPage({super.key});
  @override
  State<DashboardPage> createState() => _DashboardPageState();
}

class _DashboardPageState extends State<DashboardPage> with DashboardDataMixin {
  @override
  void initState() {
    super.initState();
    _loadConfig();
    _load();
    unawaited(_loadExtras());
    _ws.connect();
    _hrSub = _ws.hrAlerts.listen((list) {
      if (!mounted) return;
      setState(() => _hrAlerts = list);
    });
    _cookieSub = _ws.cookieAlerts.listen((list) {
      if (!mounted) return;
      setState(() => _cookieAlerts = list);
    });
    _snapSub = _ws.snapshots.listen((list) {
      if (!mounted) return;
      setState(() => _snapshots = list);
    });
    _noticeSub = _ws.notices.listen((n) {
      if (!mounted) return;
      setState(() => _recentNotices = [n, ..._recentNotices].take(30).toList());
    });
  }

  @override
  void dispose() {
    _hrSub?.cancel();
    _cookieSub?.cancel();
    _snapSub?.cancel();
    _noticeSub?.cancel();
    super.dispose();
  }

  @override
  void _toast(String msg) {
    final m = ScaffoldMessenger.maybeOf(context);
    m?.showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 2)),
    );
  }

  @override
  Widget build(BuildContext context) {
    final wide = isWideScreen(context);
    return PTPageScaffold(
      title: '仪表盘',
      toolbarHeight: wide ? 48 : 44,
      actions: [
        PTBarAction(
          icon: Icons.tune,
          label: '总仪表盘设置',
          overflow: true,
          onPressed: (_loading || _overview == null) ? null : _openSettings,
        ),
        PTBarAction(
          icon: Icons.image_outlined,
          label: '导出图片',
          onPressed: (_loading || _overview == null) ? null : _exportImage,
        ),
        PTBarAction(
          icon: Icons.sync,
          label: '刷新统计',
          loading: _syncing,
          onPressed: _syncing ? null : _syncAll,
        ),
      ],
      body: Padding(
        padding: EdgeInsets.fromLTRB(
          wide ? 20 : 10,
          wide ? 20 : 8,
          wide ? 20 : 10,
          wide ? 20 : 12,
        ),
        child: _body(context),
      ),
    );
  }

  Widget _body(BuildContext context) {
   if (_loading && _overview == null) {
      return const PTLoadingView();
   }
    if (_overview == null) {
      return PTEmptyState(
        icon: Icons.error_outline,
        text: _error == null ? '暂无数据' : '加载失败',
        description: _error,
        onRetry: _load,
      );
    }
    final o = _overview!;
    final fresh = o.siteCount == 0 && o.downloaderCount == 0;
    return RefreshIndicator(
      onRefresh: _refresh,
      child: ListView(
        padding: EdgeInsets.zero,
        children: [
          if (fresh) ...[
            _onboardingCard(context),
            SizedBox(height: isWideScreen(context) ? 14 : 10),
          ],
          if (_cookieAlerts.isNotEmpty) ...[
            _alertCard(AlertKind.cookie),
            SizedBox(height: isWideScreen(context) ? 14 : 10),
          ],
          if (_hrAlerts.isNotEmpty) ...[
            _alertCard(AlertKind.hr),
            SizedBox(height: isWideScreen(context) ? 14 : 10),
          ],
          _extrasFailedBanner(context),
          if (_extrasFailed.isNotEmpty)
            SizedBox(height: isWideScreen(context) ? 14 : 10),
          _noticeBanner(_statReadNotice),
          if (_statReadNotice.isNotEmpty)
            SizedBox(height: isWideScreen(context) ? 14 : 10),
          _metricGrid(context),
          SizedBox(height: isWideScreen(context) ? 14 : 10),
          _sitesSection(context),
          SizedBox(height: isWideScreen(context) ? 14 : 10),
          _opsSection(context),
          SizedBox(height: isWideScreen(context) ? 14 : 10),
          _autoSection(context),
          SizedBox(height: isWideScreen(context) ? 14 : 10),
          _activitySection(context),
          const SizedBox(height: 4),
        ],
      ),
    );
  }

  // 顶部账面条：并发区取数失败、每站统计读取失败都走同一条渲染（判据/样式只写一份）。
  Widget _noticeBanner(String notice) {
    if (notice.isEmpty) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: const Color(0xffd97706).withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(children: [
        const Icon(Icons.warning_amber_rounded,
            size: 16, color: Color(0xffd97706)),
        const SizedBox(width: 7),
        Expanded(
            child: Text(notice,
                style: const TextStyle(fontSize: 12, height: 1.35))),
      ]),
    );
  }

  // 并发区取数失败的顶部账面：卡片内的「加载失败」占位散在长页面下方，先在最上面点名一次。
  Widget _extrasFailedBanner(BuildContext context) =>
      _noticeBanner(extrasNotice(_extrasFailed));

  // 每站统计读取失败也要在顶部说清（判据与文案在 dashboard_extras.statReadNotice，
  // 与并发区同一套纯函数口径，界面只负责渲染）。
  String get _statReadNotice => statReadNotice(_overview?.statReadError);

  // 区②实时运维：下载器实时速率（WS snapshots）+ 系统资源（/system/metrics）。
  Widget _opsSection(BuildContext context) {
    final dl = _snapshots.fold<int>(0, (s, t) => s + (t.dlRate ?? 0));
    final up = _snapshots.fold<int>(0, (s, t) => s + (t.upRate ?? 0));
    final downloading = _snapshots.fold<int>(
        0,
        (s, t) =>
            s + t.torrents.where((x) => RegExp(r'download|leech').hasMatch(x.state)).length);
    final seeding = _snapshots.fold<int>(
        0, (s, t) => s + t.torrents.where((x) => RegExp(r'upload|seed').hasMatch(x.state)).length);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        const Text('实时运维', style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
        const SizedBox(height: 10),
        Row(children: [
          Expanded(child: _kvCell('↓ 总下载', _fmtSpeed(dl), PTColors.primary)),
          const SizedBox(width: 10),
          Expanded(child: _kvCell('↑ 总上传', _fmtSpeed(up), PTColors.success)),
        ]),
        const SizedBox(height: 8),
        Text('下载中 $downloading · 做种 $seeding · ${_snapshots.length} 个下载器',
            style: TextStyle(fontSize: 12, color: context.dimText)),
        const Divider(height: 24),
        if (_extrasAdminOnly)
          Text('系统资源$extrasAdminOnly',
              style: TextStyle(fontSize: 12, color: context.dimText))
        else if (!extrasAvailable(_extrasFailed, extrasSectionMetrics))
          Text('系统资源读取失败，下拉刷新重试',
              style: TextStyle(fontSize: 12, color: context.dimText))
        else if (_metrics == null)
          Text('系统资源采集中…', style: TextStyle(fontSize: 12, color: context.dimText))
        else ...[
          _resBar('CPU', _metrics!.cpuUsage, '${_metrics!.cpuCores} 核'),
          const SizedBox(height: 8),
          _resBar('内存', _metrics!.memUsage,
              '${_fmtBytes(_metrics!.memUsed)} / ${_fmtBytes(_metrics!.memTotal)}'),
          ..._metrics!.disks.take(2).map((d) => Padding(
                padding: const EdgeInsets.only(top: 8),
                child: _resBar(d.device, d.usage, '${_fmtBytes(d.used)} / ${_fmtBytes(d.total)}'),
              )),
        ],
      ]),
    );
  }

  // 区③④自动化运行 + AI 用量。
  Widget _autoSection(BuildContext context) {
    final brushOn = _brush.where((t) => t.enabled).length;
    final brushUp = _brush.fold<int>(0, (s, t) => s + t.totalUpload);
    final brushAdded = _brush.fold<int>(0, (s, t) => s + t.totalAdded);
    final contractOn = _contract.where((r) => r.enabled).toList();
    final breach = contractOn.where((r) => r.lastStatus == 'breach').length;
    final aiTokens =
        (_agentStats?.totalInputTokens ?? 0) + (_agentStats?.totalOutputTokens ?? 0);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
          const Text('自动化运行', style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
          if (breach > 0)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
              decoration: BoxDecoration(
                color: const Color(0xffdc2626).withValues(alpha: 0.13),
                borderRadius: BorderRadius.circular(999),
              ),
              child: const Text('契约违约',
                  style: TextStyle(fontSize: 12, color: PTColors.danger)),
            ),
        ]),
        const SizedBox(height: 8),
        _statChip(
            '工作流',
            extrasValue(
                failed: _extrasFailed,
                section: extrasSectionWorkflow,
                value: '${_wfStats?.enabled ?? 0}/${_wfStats?.total ?? 0}'),
            '启用/总数 · 成功 ${_wfStats?.runsSuccess ?? 0}'),
        _statChip(
            '刷流',
            extrasValue(
                failed: _extrasFailed,
                section: extrasSectionBrush,
                value: '$brushOn'),
            '累计 ↑${_fmtBytes(brushUp)} · $brushAdded 种'),
        _statChip(
            '契约违约',
            extrasValue(
                failed: _extrasFailed,
                section: extrasSectionContract,
                value: '$breach'),
            '共 ${contractOn.length} 条契约'),
        const Divider(height: 20),
        _statChip(
            'AI 用量',
            _extrasAdminOnly
                ? extrasAdminOnly
                : extrasValue(
                    failed: _extrasFailed,
                    section: extrasSectionAgent,
                    value: _agentStats == null
                        ? '-'
                        : '${_agentStats!.totalRequests} 次'),
            _agentStats == null
                ? ''
                : 'Token ${_fmtToken(aiTokens)} · \$${_agentStats!.totalCostUsd.toStringAsFixed(4)}'),
      ]),
    );
  }

  // 区⑥最近活动时间线（notice 聚合）。
  Widget _activitySection(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('最近活动', style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
        const SizedBox(height: 8),
        if (_recentNotices.isEmpty)
          Padding(
            padding: EdgeInsets.symmetric(vertical: 16),
            child: Center(
                child: Text(
                    extrasValue(
                        failed: _extrasFailed,
                        section: extrasSectionNotice,
                        value: '暂无最近活动',
                        unavailable: '通知读取失败，下拉刷新重试'),
                    style: TextStyle(fontSize: 12, color: context.dimText))),
          )
        else
          ..._recentNotices.take(12).map((n) {
            final lvlColor = switch (n.level) {
              NoticeLevel.success => const Color(0xff16a34a),
              NoticeLevel.warning => const Color(0xffd97706),
              NoticeLevel.danger => const Color(0xffdc2626),
              NoticeLevel.info => const Color(0xff2563eb),
            };
            final label = noticeSourceLabel[n.source] ?? n.source;
            String p(int v) => v.toString().padLeft(2, '0');
            final t = '${p(n.createdAt.month)}-${p(n.createdAt.day)} '
                '${p(n.createdAt.hour)}:${p(n.createdAt.minute)}';
            return Padding(
              padding: const EdgeInsets.symmetric(vertical: 5),
              child: Row(children: [
                Container(
                    width: 8,
                    height: 8,
                    decoration: BoxDecoration(color: lvlColor, shape: BoxShape.circle)),
                const SizedBox(width: 9),
                Expanded(
                    child: Text(n.title,
                        style: TextStyle(fontSize: 12.5),
                        overflow: TextOverflow.ellipsis)),
                const SizedBox(width: 8),
                Text(label, style: TextStyle(fontSize: 12, color: context.dimText)),
                const SizedBox(width: 8),
                Text(t, style: TextStyle(fontSize: 12, color: context.dimText)),
              ]),
            );
          }),
      ]),
    );
  }
}
