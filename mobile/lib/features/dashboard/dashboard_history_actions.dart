// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _DashboardPageState 的历史/设置入口扩展：历史数据弹窗、设置面板入口、统计 chip。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {
  // 用户历史数据：打开弹窗/底栏加载该站点 90 天快照（自适应浮层）。
  Future<void> _viewHistory(SiteStatItem s) async {
    List<SiteHistoryPoint> history = [];
    String? error;
    String? note;
    bool loading = true;
    try {
      final res = await _api.history(s.siteId, days: 90);
      history = res.points;
      if (res.truncated) {
        note = '该区间共 ${res.total} 个快照，仅显示最近 ${res.points.length} 个';
      }
    } catch (e) {
      // 读不到 ≠ 没有快照：吞成空列表会渲染「暂无历史快照」。
      error = '历史数据加载失败：$e';
    } finally {
      loading = false;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '${s.siteName} · 历史数据',
      wideWidth: 820,
      builder: (_) => DashboardHistoryBody(
        points: history,
        loading: loading,
        error: error,
        note: note,
        fmtBytes: _fmtBytes,
        fmtTime: _fmtTimeFull,
      ),
    );
  }

  void _openSettings() {
    final sites = _overview?.sites ?? const <SiteStatItem>[];
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => DashboardSettingsSheet(
        initial: _config,
        sites: sites,
        onSave: _saveConfig,
      ),
    );
  }

  Widget _statChip(String label, String value, String sub) => Padding(
        padding: const EdgeInsets.only(top: 6),
        child: Row(crossAxisAlignment: CrossAxisAlignment.center, children: [
          SizedBox(
              width: 76,
              child: Text(label, style: TextStyle(fontSize: 12, color: context.dimText))),
          const SizedBox(width: 8),
          Text(value, style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
          const SizedBox(width: 8),
          Expanded(
              child: Text(sub,
                  style: TextStyle(fontSize: 11, color: context.dimText),
                  overflow: TextOverflow.ellipsis)),
        ]),
      );
}
