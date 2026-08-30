// _DashboardPageState 的数据加载/同步 mixin：配置、统计、连通性、全量/单站同步。
// （含 setState 段；从 dashboard_page.dart 拆出，行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

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
    await prefs.setString(
      _totalDashboardConfigKey,
      jsonEncode(config.toJson()),
    );
    if (mounted) setState(() => _config = config);
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
  Future<void> _loadExtras() async {
    WorkflowStats? wf;
    AgentStats? ai;
    List<BrushFlowTask> brush = [];
    List<ContractRule> contract = [];
    List<Notice> notices = [];
    SystemMetrics? metrics;
    await Future.wait([
      () async { try { wf = await _wfApi.stats(); } catch (_) {} }(),
      () async { try { ai = await _agentApi.stats(); } catch (_) {} }(),
      () async { try { brush = await _brushApi.list(); } catch (_) {} }(),
      () async { try { contract = await _contractApi.list(); } catch (_) {} }(),
      () async { try { notices = await _noticeApi.list(); } catch (_) {} }(),
      () async { try { metrics = await SystemApi.metrics(); } catch (_) {} }(),
    ]);
    if (!mounted) return;
    setState(() {
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
    } catch (_) {
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
    } catch (_) {
      if (mounted) _toast('同步失败，请稍后重试');
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
    } catch (_) {
      if (mounted) _toast('${s.siteName} 刷新失败');
    } finally {
      if (mounted) setState(() => _refreshing.remove(s.siteId));
    }
  }
}
