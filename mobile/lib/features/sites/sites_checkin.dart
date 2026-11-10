// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _SitesPageState 的连通性/签到 mixin：探测、原因弹窗、单站/全部签到。
// mixin on State 可用 setState；从 sites_page.dart 拆出，行为不变。
part of 'package:ptpatronus/features/sites/sites_page.dart';

mixin SitesCheckinMixin on State<SitesPage> {
  // 宿主 _SitesPageState 提供的钩子。
  void _toast(String m);

  List<Site> _sites = [];
  List<SiteDef> _defs = []; // 添加表单下拉项缓存
  bool _loading = true; // 首访即显示加载态，避免短暂空白像页面坏了
  String? _error;
  Map<int, SiteConnectivityStatus> _connectivity = {};
  bool _connectivityLoading = false;
  /// #404 连通状态读不到时的原因（null = 读到了）。
  /// 必须与 `_connectivity = {}` 一起看：空清单既可能是「全都没联通」，
  /// 也可能是「这次没读到」，两者在界面上同形，而用户据此判断站点是否掉线。
  String? _connectivityError;
  int? _checkining; // 正在签到的站点 id
  int? _testing; // 正在连接测试的站点 id
  bool _checkinAllLoading = false;

  bool get _isAdmin => ApiClient().isAdmin;

  Future<void> _loadConnectivity() async {
    if (mounted) setState(() => _connectivityLoading = true);
    try {
      final statuses = await SiteApi().statuses();
      if (!mounted) return;
      setState(() => _connectivity = {for (final s in statuses) s.siteId: s});
    } catch (e) {
      // #404 读不到连通状态时**必须说读不到**：静默回 {} 与「所有站都没联通」同形，
      // 而这一页正是用户判断站点是否掉线的依据（仪表盘那份已在 #403 修过，这里是第二份拷贝）。
      _connectivityError = errTextOf(e);
      if (mounted) setState(() => _connectivity = {});
    } finally {
      if (mounted) setState(() => _connectivityLoading = false);
    }
  }

  // —— 连接测试 / 签到 / 一键签到（业务逻辑保持不变）——
  Future<void> _testConnection(Site s) async {
    setState(() => _testing = s.id);
    try {
      final r = await SiteApi().test(s.id);
      if (r['ok'] == true) {
        if (mounted) {
          setState(() {
            _connectivity = {
              ..._connectivity,
              s.id: SiteConnectivityStatus(
                siteId: s.id,
                siteName: s.name,
                state: SiteConnectivityState.connected,
                level: r['level'] as String? ?? '',
              ),
            };
          });
        }
        _toast('${s.name}：连接正常，等级 ${r['level'] ?? '-'}');
      } else {
        if (mounted) {
          setState(() {
            _connectivity = {
              ..._connectivity,
              s.id: SiteConnectivityStatus(
                siteId: s.id,
                siteName: s.name,
                state: SiteConnectivityState.disconnected,
                error: r['error'] as String? ?? '',
              ),
            };
          });
        }
        _toast('${s.name}：${r['error'] ?? '连接失败'}');
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _connectivity = {
            ..._connectivity,
            s.id: SiteConnectivityStatus(
              siteId: s.id,
              siteName: s.name,
              state: SiteConnectivityState.disconnected,
              error: '$e',
            ),
          };
        });
      }
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _testing = null);
    }
  }

  /// #404 连通状态「这次没读到」时的原因对话框。
  ///
  /// 与 _showConnectivityReason 分开是因为两者语义不同：那一档是「读到了、站点真掉线」
  /// （原因来自 site_connectivity 缓存里的 error）；这一档是「**没读到**」——
  /// 缓存压根没取回来，原因来自这次请求本身。混在一起会把「没读到」说成「站点掉线」。
  Future<void> _showConnectivityReadError(Site s) async {
    final reason = _connectivityError;
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('📊 ${s.name} 的连通状态没读到'),
        content: Text(
          reason ??
              '这次没读到连通状态（不是「站点掉线」）：请下拉后重试。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: const Text('知道了'),
          ),
        ],
      ),
    );
  }

  Future<void> _showConnectivityReason(Site s) async {
    final status = _connectivity[s.id];
    if (status == null || status.state == SiteConnectivityState.connected) {
      return;
    }
    final title = status.state == SiteConnectivityState.disabled
        ? '${s.name} 已停用'
        : '${s.name} 未联通';
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(title),
        content: Text(status.error.isNotEmpty ? status.error : '站点未返回具体失败原因'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: const Text('知道了'),
          ),
        ],
      ),
    );
  }

  String _checkinMsg(Map<String, dynamic> r) {
    final name = r['site_name'] as String? ?? '';
    final success = r['success'] == true;
    final message = r['message'] as String? ?? '';
    final bonus = (r['bonus'] as num?)?.toDouble() ?? 0;
    final error = r['error'] as String? ?? '';
    if (success) {
      if (bonus > 0) return '$name：签到成功，获得 $bonus 魔力';
      if (message.contains('已签到')) return '$name：今天已签到';
      return '$name：${message.isNotEmpty ? message : '签到成功'}';
    }
    if (error.contains('not supported')) return '$name：该站点暂不支持签到';
    if (error.contains('not logged in') || message.contains('not logged in')) {
      return '$name：未登录，请先登录';
    }
    return '$name：${error.isNotEmpty ? error : (message.isNotEmpty ? message : '签到失败')}';
  }

  Future<void> _checkin(Site s) async {
    setState(() => _checkining = s.id);
    try {
      final r = await SiteApi().checkin(s.id);
      _toast(_checkinMsg(r));
    } catch (e) {
      _toast('${s.name}：请求失败：$e');
    } finally {
      if (mounted) setState(() => _checkining = null);
    }
  }

  Future<void> _checkinAll() async {
    setState(() => _checkinAllLoading = true);
    try {
      final res = await SiteApi().checkinAll();
      // 「清单读不出」与「没有启用的站点」是两件事（#258）。
      if (res.listError.isNotEmpty) {
        _toast('站点清单读不出来：${res.listError}');
        return;
      }
      final results = res.results;
      if (results.isEmpty) {
        _toast('没有启用的站点');
        return;
      }
      var ok = 0, already = 0;
      final failed = <String>[];
      for (final r in results) {
        if (r['success'] == true) {
          if ((r['message'] as String? ?? '').contains('已签到')) {
            already++;
          } else {
            ok++;
          }
        } else if (!(r['error'] as String? ?? '').contains('not supported')) {
          failed.add(r['site_name'] as String? ?? '?');
        }
      }
      final summary = '成功 $ok / 已签 $already';
      _toast(
        failed.isEmpty
            ? '一键签到完成：$summary'
            : '一键签到完成：$summary / 失败 ${failed.length}（${failed.join('、')}）',
      );
    } catch (e) {
      _toast('一键签到失败：$e');
    } finally {
      if (mounted) setState(() => _checkinAllLoading = false);
    }
  }
}
