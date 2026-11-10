// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _DashboardPageState 的聚合计算/格式化扩展：求和/比值/榜首/身份显示/时间戳/速率格式化。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {
  int _sumInt(List<SiteStatItem> sites, int Function(SiteStatItem) pick) =>
      sites.fold<int>(0, (sum, s) => sum + pick(s));

  double _sumDouble(
    List<SiteStatItem> sites,
    double Function(SiteStatItem) pick,
  ) => sites.fold<double>(0, (sum, s) => sum + pick(s));

  // 总分享率：优先用后端 totals.true_ratio（真实上传/下载，去魔力膨胀），否则回退到
  // sum(uploaded)/sum(downloaded)。不再取各站 ratio 的平均——平均分享率数学上无意义，
  // 且站点 ratio 含魔力/上传系数膨胀，会显著虚高（如 17.7 vs 真实 1.08）。
  double _totalRatio(List<SiteStatItem> sites) {
    final tr = _overview?.trueRatio ?? 0;
    if (tr > 0) return tr;
    final up = _sumInt(sites, (s) => s.uploaded);
    final dl = _sumInt(sites, (s) => s.downloaded);
    return dl > 0 ? up / dl : 0;
  }

  SiteStatItem? _topSiteBy(num Function(SiteStatItem) pick) {
    final sites = _visibleSites().where((s) => pick(s) > 0).toList();
    if (sites.isEmpty) return null;
    sites.sort((a, b) => pick(b).compareTo(pick(a)));
    return sites.first;
  }

  String _displaySiteName(SiteStatItem s) {
    if (!_config.showSiteName) return '';
    if (_config.blurSensitiveInfo) return '***';
    return s.siteName.isNotEmpty ? s.siteName : '-';
  }

  String _displayUserIdentity(SiteStatItem s) {
    if (!_config.showUsername) return '';
    if (_config.blurUserInfo) return '*** <***> UID ***';
    final parts = <String>[
      if (s.username.isNotEmpty) s.username,
      if (s.level.isNotEmpty) '<${s.level}>',
      if (s.userId.isNotEmpty) 'UID ${s.userId}',
    ];
    return parts.isEmpty ? '-' : parts.join(' ');
  }

  String _nowStamp() {
    final d = DateTime.now();
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.year}-${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  String _fmtSpeed(int bytesPerSec) =>
      bytesPerSec == 0 ? '0 B/s' : '${_fmtBytes(bytesPerSec)}/s';

  String _fmtToken(int n) {
    if (n >= 1000000) return '${(n / 1000000).toStringAsFixed(1)}M';
    if (n >= 1000) return '${(n / 1000).toStringAsFixed(1)}k';
    return '$n';
  }

}
