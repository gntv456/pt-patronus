// _DashboardPageState 的数据加载/同步 mixin：配置、统计、连通性、全量/单站同步。
// （含 setState 段；从 dashboard_page.dart 拆出，行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

/// 布局保存的结果判定：`setString` 返回 false 表示本机偏好没写进去（配额/平台通道问题）。
/// 返回 null = 调用方可以应用新配置；非空 = 要展示给用户的中文原因，且不得切内存态。
String? layoutSaveFailure(bool written) {
  if (written) return null;
  return '仪表盘布局保存失败：本机偏好存储写入未成功，下次启动仍会是当前这套设置';
}

/// 本机偏好写入的注入缝：真机传 `SharedPreferences.setString`，用例注入返回 false 的写入。
typedef ConfigWriter = Future<bool> Function(String key, String value);

/// 保存布局的编排：写失败只出声、绝不应用新配置；写成功才应用。
/// 「失败不许切内存态」这一步从前只存在于 _saveConfig 的行序里，抽出来才可断言。
Future<void> saveDashboardConfig({
  required String key,
  required String payload,
  required ConfigWriter write,
  required void Function(String reason) onFail,
  required void Function() apply,
}) async {
  final reason = layoutSaveFailure(await write(key, payload));
  if (reason != null) {
    onFail(reason);
    return;
  }
  apply();
}

mixin DashboardDataMixin on State<DashboardPage> {
  List<SiteStatItem> _visibleSites() {
    final all = _overview?.sites ?? const <SiteStatItem>[];
    final filtered = all
        .where((s) => _config.isSiteSelected(s.siteId))
        .toList();
    if (_config.siteDataLimit > 0 && filtered.length > _config.siteDataLimit) {
      return filtered.take(_config.siteDataLimit).toList();
    }
    return filtered;
  }

  String? _siteFaviconUrl(SiteStatItem site) {
    if (_config.blurSensitiveInfo) return null;
    final baseUrl = _siteBaseUrls[site.siteId]?.trim();
    if (baseUrl == null || baseUrl.isEmpty) return null;
    final uri = Uri.tryParse(baseUrl);
    if (uri == null ||
        uri.host.isEmpty ||
        (uri.scheme != 'http' && uri.scheme != 'https')) {
      return null;
    }
    return '${uri.origin}/favicon.ico';
  }

  // 宿主 _DashboardPageState 提供的钩子。
  void _toast(String msg);
  Future<void> _ensureSiteBaseUrls() async {
    if (_siteBaseUrls.isNotEmpty) return;
    final sites = await _siteApi.list();
    if (!mounted) return;
    setState(() {
      _siteBaseUrls = {
        for (final site in sites)
          if (site.baseUrl.trim().isNotEmpty) site.id: site.baseUrl.trim(),
      };
    });
  }

  Future<void> _prepareShareFavicons() async {
    try {
      await _ensureSiteBaseUrls();
    } catch (_) {
      return;
    }
    if (!mounted || _config.blurSensitiveInfo) return;
    final urls = _visibleSites()
        .take(8)
        .map(_siteFaviconUrl)
        .whereType<String>()
        .toSet();
    await Future.wait([
      for (final url in urls)
        precacheImage(NetworkImage(url), context)
            .timeout(const Duration(seconds: 3), onTimeout: () {})
            .catchError((_) {}),
    ]);
  }

  final _api = DashboardApi();
  final _siteApi = SiteApi();
  final _ws = WsClient();
  final _shareKey = GlobalKey(); // 仪表盘分享图 RepaintBoundary（toImage 截图）
  DashboardOverview? _overview;
  Map<int, String> _siteBaseUrls = {};
  List<HRAlert> _hrAlerts = [];
  List<String> _cookieAlerts = [];
  Map<int, SiteConnectivityStatus> _connectivity = {};
  bool _connectivityLoading = false;
  /// #403 连通状态读不到时的原因（null = 读到了）。
  /// 必须与 `_connectivity = {}` 一起看：空清单既可能是「全都没联通」，
  /// 也可能是「这次没读到」，两者在界面上同形，而用户据此判断站点是否掉线。
  String? _connectivityError;
  TotalDashboardConfig _config = TotalDashboardConfig.defaults;
  final Set<int> _refreshing = {}; // 单站刷新中的 site_id 集合
  bool _syncing = false; // 全站同步中（刷新统计按钮 loading）
  bool _loading = true;
  String? _error;
  StreamSubscription? _hrSub;
  StreamSubscription? _cookieSub;
  // —— 仪表盘新增区数据（实时运维 / 自动化 / 活动时间线）——
  List<TorrentSnapshot> _snapshots = [];
  SystemMetrics? _metrics;
  WorkflowStats? _wfStats;
  AgentStats? _agentStats;
  List<BrushFlowTask> _brush = [];
  List<ContractRule> _contract = [];
  List<Notice> _recentNotices = [];
  StreamSubscription? _snapSub;
  StreamSubscription? _noticeSub;
  final _wfApi = WorkflowApi();
  final _agentApi = AgentApi();
  final _brushApi = BrushFlowApi();
  final _contractApi = ContractApi();
  final _noticeApi = NoticeApi();

  Future<void> _loadConfig() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_totalDashboardConfigKey);
    if (raw == null || raw.isEmpty) return;
    try {
      final next = TotalDashboardConfig.fromJson(
        jsonDecode(raw) as Map<String, dynamic>,
      );
      if (mounted) setState(() => _config = next);
    } catch (_) {
      // Keep defaults if the saved payload is from an incompatible build.
    }
  }

  Future<void> _saveConfig(TotalDashboardConfig config) async {
    final prefs = await SharedPreferences.getInstance();
    // setString 返回 false 表示本机偏好没写进去（配额/平台通道问题）：
    // 从前不看返回值，面板照常关闭、内存里也切成新布局，用户以为布局已保存，
    // 下次启动才发现回到旧设置。失败时既不出声也不改内存态。
    await saveDashboardConfig(
      key: _totalDashboardConfigKey,
      payload: jsonEncode(config.toJson()),
      write: prefs.setString,
      onFail: (reason) {
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text(reason)));
        }
      },
      apply: () {
        if (mounted) setState(() => _config = config);
      },
    );
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      _overview = await _api.overview();
      _error = null;
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
    unawaited(_loadConnectivity());
  }

  // 并发拉取仪表盘新增区数据（自动化/AI/系统资源/通知），单项失败不阻断（对齐 Web Promise.allSettled）。
  // 但失败必须记账：吞掉之后卡片会拿 0/空数组继续渲染，「读不到」就伪装成「真的没有」。
  List<String> _extrasFailed = const [];
  // /agent/stats 与 /system/metrics 是 admin-only（探针实测：非管理员回 403 admin_required）。
  // 判「没权限」以真实响应为准，不信客户端缓存的角色——缓存缺失时若当成非管理员，
  // 会把管理员本来能看的数据也藏起来。
  bool _extrasAdminOnly = false;

  Future<void> _loadExtras() async {
    WorkflowStats? wf;
    AgentStats? ai;
    List<BrushFlowTask> brush = [];
    List<ContractRule> contract = [];
    List<Notice> notices = [];
    SystemMetrics? metrics;
    final failed = <String>[];
    var adminHidden = false;
    await Future.wait([
      () async {
        try {
          wf = await _wfApi.stats();
        } catch (_) {
          failed.add(extrasSectionWorkflow);
        }
      }(),
      () async {
        try {
          ai = await _agentApi.stats();
        } catch (e) {
          if (extrasForbidden(e)) {
            adminHidden = true;
          } else {
            failed.add(extrasSectionAgent);
          }
        }
      }(),
      () async {
        try {
          brush = await _brushApi.list();
        } catch (_) {
          failed.add(extrasSectionBrush);
        }
      }(),
      () async {
        try {
          contract = await _contractApi.list();
        } catch (_) {
          failed.add(extrasSectionContract);
        }
      }(),
      () async {
        try {
          notices = await _noticeApi.list();
        } catch (_) {
          failed.add(extrasSectionNotice);
        }
      }(),
      () async {
        try {
          metrics = await SystemApi.metrics();
        } catch (e) {
          if (extrasForbidden(e)) {
            adminHidden = true;
          } else {
            failed.add(extrasSectionMetrics);
          }
        }
      }(),
    ]);
    if (!mounted) return;
    setState(() {
      _extrasFailed = failed;
      _extrasAdminOnly = adminHidden;
      _wfStats = wf;
      _agentStats = ai;
      _brush = brush;
      _contract = contract;
      _recentNotices = notices.take(30).toList();
      _metrics = metrics;
    });
  }

  Future<void> _loadConnectivity() async {
    if (mounted) setState(() => _connectivityLoading = true);
    try {
      final statuses = await _siteApi.statuses();
      if (!mounted) return;
      setState(() => _connectivity = {for (final s in statuses) s.siteId: s});
    } catch (e) {
      // #403 读不到连通状态时**必须说读不到**：静默回 {} 与「所有站都没联通」同形，
      // 而这页正是用户判断站点是否掉线的依据（此前会显示成一片正常）。
      _connectivityError = errTextOf(e);
      if (mounted) setState(() => _connectivity = {});
    } finally {
      if (mounted) setState(() => _connectivityLoading = false);
    }
  }

  Future<void> _refresh() async {
    try {
      final overview = await _api.overview();
      if (!mounted) return;
      setState(() {
        _overview = overview;
        _error = null;
      });
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    }
    unawaited(_loadConnectivity());
    // 下拉刷新要真的重试并发区，否则提示里的「请下拉刷新重试」是一句做不到的承诺。
    unawaited(_loadExtras());
  }

  /// #403 连通状态「这次没读到」时的原因对话框。
  ///
  /// 与 _showConnectivityReason 分开是因为两者语义不同：那一档是「读到了、站点真掉线」
  /// （原因来自 site_connectivity 缓存里的 error）；这一档是「**没读到**」——
  /// 缓存压根没取回来，原因来自这次请求本身。
  /// 混在一起会把「没读到」说成「站点掉线」，方向完全错（#387 那一族）。
  Future<void> _showConnectivityReadError(SiteStatItem site) async {
    final reason = _connectivityError;
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('📊 ${site.siteName} 的连通状态没读到'),
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

  Future<void> _showConnectivityReason(SiteStatItem site) async {
    final status = _connectivity[site.siteId];
    if (status == null || status.state == SiteConnectivityState.connected) {
      return;
    }
    final title = status.state == SiteConnectivityState.disabled
        ? '${site.siteName} 已停用'
        : '${site.siteName} 未联通';
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

  // 全站同步：立即抓取所有启用站点统计（回填新加入站点的 0 数据），刷新成功替换总览。
  Future<void> _syncAll() async {
    setState(() => _syncing = true);
    try {
      final ov = await _api.sync();
      if (!mounted) return;
      setState(() => _overview = ov);
      _toast('统计已同步');
    } catch (e) {
      // #403 同上：后端给的原因优先（「请稍后重试」只在真没有原因时才用）。
      if (mounted) _toast(errTextOf(e) ?? '同步失败，请稍后重试');
    } finally {
      if (mounted) setState(() => _syncing = false);
    }
  }

  // 单站刷新：仅重新抓取该站点，刷新成功后用返回的总览替换。
  Future<void> _syncOne(SiteStatItem s) async {
    setState(() => _refreshing.add(s.siteId));
    try {
      final ov = await _api.syncOne(s.siteId);
      if (!mounted) return;
      setState(() => _overview = ov);
      _toast('${s.siteName} 已刷新');
    } catch (e) {
      // #403 别把后端给的原因丢掉：这条出口在「站点凭据解不开」时回的是
      // 「请到站点里重新保存该站点凭据；本次未向站点发请求」（后端 #398）——
      // 用户不知道该去站点页重登 Cookie，就只会反复点刷新。
      // errTextOf 拿不到时（网络异常等）仍退回本句兜底，不会凭空编一个原因。
      if (mounted) _toast(errTextOf(e) ?? '${s.siteName} 刷新失败');
    } finally {
      if (mounted) setState(() => _refreshing.remove(s.siteId));
    }
  }
}
