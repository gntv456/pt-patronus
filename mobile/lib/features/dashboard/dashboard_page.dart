import 'dart:async';
import 'dart:convert';
import 'dart:io' show File, Platform;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart'
    show TargetPlatform, defaultTargetPlatform;
import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_nav_items.dart';
import 'package:flutter/rendering.dart' show RenderRepaintBoundary;
import 'package:flutter/services.dart' show MissingPluginException;
import 'package:path_provider/path_provider.dart';
import 'package:ptpatronus/api/dashboard.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:share_plus/share_plus.dart';
import 'package:shared_preferences/shared_preferences.dart';

const _totalDashboardConfigKey = 'total_dashboard_config_v1';

class TotalDashboardConfig {
  final bool showTotalSites;
  final bool showUploadCount;
  final bool showUpload;
  final bool showDownload;
  final bool showSeedingCount;
  final bool showSeedingVolume;
  final bool showBonus;
  final bool showBonusPerHour;
  final bool showRatio;
  final bool showRankPanel;
  final bool showSiteList;
  final bool showSiteName;
  final bool showUsername;
  final bool blurSensitiveInfo;
  final bool blurUserInfo;
  final bool compactMode;
  final int siteDataLimit;
  final List<int> selectedSiteIds;

  const TotalDashboardConfig({
    this.showTotalSites = true,
    this.showUploadCount = true,
    this.showUpload = true,
    this.showDownload = true,
    this.showSeedingCount = true,
    this.showSeedingVolume = true,
    this.showBonus = true,
    this.showBonusPerHour = true,
    this.showRatio = true,
    this.showRankPanel = true,
    this.showSiteList = true,
    this.showSiteName = true,
    this.showUsername = true,
    this.blurSensitiveInfo = false,
    this.blurUserInfo = false,
    this.compactMode = true,
    this.siteDataLimit = 0,
    this.selectedSiteIds = const [],
  });

  static const defaults = TotalDashboardConfig();

  bool isSiteSelected(int siteId) =>
      selectedSiteIds.isEmpty || selectedSiteIds.contains(siteId);

  TotalDashboardConfig copyWith({
    bool? showTotalSites,
    bool? showUploadCount,
    bool? showUpload,
    bool? showDownload,
    bool? showSeedingCount,
    bool? showSeedingVolume,
    bool? showBonus,
    bool? showBonusPerHour,
    bool? showRatio,
    bool? showRankPanel,
    bool? showSiteList,
    bool? showSiteName,
    bool? showUsername,
    bool? blurSensitiveInfo,
    bool? blurUserInfo,
    bool? compactMode,
    int? siteDataLimit,
    List<int>? selectedSiteIds,
  }) {
    return TotalDashboardConfig(
      showTotalSites: showTotalSites ?? this.showTotalSites,
      showUploadCount: showUploadCount ?? this.showUploadCount,
      showUpload: showUpload ?? this.showUpload,
      showDownload: showDownload ?? this.showDownload,
      showSeedingCount: showSeedingCount ?? this.showSeedingCount,
      showSeedingVolume: showSeedingVolume ?? this.showSeedingVolume,
      showBonus: showBonus ?? this.showBonus,
      showBonusPerHour: showBonusPerHour ?? this.showBonusPerHour,
      showRatio: showRatio ?? this.showRatio,
      showRankPanel: showRankPanel ?? this.showRankPanel,
      showSiteList: showSiteList ?? this.showSiteList,
      showSiteName: showSiteName ?? this.showSiteName,
      showUsername: showUsername ?? this.showUsername,
      blurSensitiveInfo: blurSensitiveInfo ?? this.blurSensitiveInfo,
      blurUserInfo: blurUserInfo ?? this.blurUserInfo,
      compactMode: compactMode ?? this.compactMode,
      siteDataLimit: siteDataLimit ?? this.siteDataLimit,
      selectedSiteIds: selectedSiteIds ?? this.selectedSiteIds,
    );
  }

  Map<String, dynamic> toJson() => {
    'show_total_sites': showTotalSites,
    'show_upload_count': showUploadCount,
    'show_upload': showUpload,
    'show_download': showDownload,
    'show_seeding_count': showSeedingCount,
    'show_seeding_volume': showSeedingVolume,
    'show_bonus': showBonus,
    'show_bonus_per_hour': showBonusPerHour,
    'show_ratio': showRatio,
    'show_rank_panel': showRankPanel,
    'show_site_list': showSiteList,
    'show_site_name': showSiteName,
    'show_username': showUsername,
    'blur_sensitive_info': blurSensitiveInfo,
    'blur_user_info': blurUserInfo,
    'compact_mode': compactMode,
    'site_data_limit': siteDataLimit,
    'selected_site_ids': selectedSiteIds,
  };

  factory TotalDashboardConfig.fromJson(Map<String, dynamic> json) {
    bool b(String key, bool fallback) =>
        json[key] is bool ? json[key] as bool : fallback;
    int i(String key, int fallback) =>
        json[key] is num ? (json[key] as num).toInt() : fallback;
    final ids = ((json['selected_site_ids'] as List?) ?? const [])
        .map((e) => e is num ? e.toInt() : int.tryParse('$e'))
        .whereType<int>()
        .toList();
    return TotalDashboardConfig(
      showTotalSites: b('show_total_sites', true),
      showUploadCount: b('show_upload_count', true),
      showUpload: b('show_upload', true),
      showDownload: b('show_download', true),
      showSeedingCount: b('show_seeding_count', true),
      showSeedingVolume: b('show_seeding_volume', true),
      showBonus: b('show_bonus', true),
      showBonusPerHour: b('show_bonus_per_hour', true),
      showRatio: b('show_ratio', true),
      showRankPanel: b('show_rank_panel', true),
      showSiteList: b('show_site_list', true),
      showSiteName: b('show_site_name', true),
      showUsername: b('show_username', true),
      blurSensitiveInfo: b('blur_sensitive_info', false),
      blurUserInfo: b('blur_user_info', false),
      compactMode: b('compact_mode', true),
      siteDataLimit: i('site_data_limit', 0),
      selectedSiteIds: ids,
    );
  }
}

extension _ChunkedList<T> on List<T> {
  List<List<T>> chunked(int size) {
    final out = <List<T>>[];
    for (var i = 0; i < length; i += size) {
      out.add(sublist(i, (i + size > length) ? length : i + size));
    }
    return out;
  }
}

// 仪表盘：跨站统计汇总（指标卡墙，含 sparkline）+ 站点明细（宽屏表格 / 窄屏卡片）。
// 对齐 Web Dashboard.vue：5 张指标卡（上传/下载/做种/做种体积/魔力）+ 近 14 天 sparkline +
// 站点表格（站点/等级/上传/下载/分享率/做种/操作）+ 单站历史数据 + 单站/全站刷新。
// 数据来自 GET /dashboard、GET /dashboard/trend?days=14、POST /dashboard/sync[/:id]、GET /dashboard/history/:id。
class DashboardPage extends StatefulWidget {
  const DashboardPage({super.key});
  @override
  State<DashboardPage> createState() => _DashboardPageState();
}

class _DashboardPageState extends State<DashboardPage> {
  final _api = DashboardApi();
  final _siteApi = SiteApi();
  final _ws = WsClient();
  final _shareKey = GlobalKey(); // 仪表盘分享图 RepaintBoundary（toImage 截图）
  DashboardOverview? _overview;
  Map<int, String> _siteBaseUrls = {};
  List<HRAlert> _hrAlerts = [];
  List<String> _cookieAlerts = [];
  TotalDashboardConfig _config = TotalDashboardConfig.defaults;
  final Set<int> _refreshing = {}; // 单站刷新中的 site_id 集合
  bool _syncing = false; // 全站同步中（刷新统计按钮 loading）
  bool _loading = true;
  String? _error;
  StreamSubscription? _hrSub;
  StreamSubscription? _cookieSub;

  @override
  void initState() {
    super.initState();
    _loadConfig();
    _load();
    _ws.connect();
    _hrSub = _ws.hrAlerts.listen((list) {
      if (!mounted) return;
      setState(() => _hrAlerts = list);
    });
    _cookieSub = _ws.cookieAlerts.listen((list) {
      if (!mounted) return;
      setState(() => _cookieAlerts = list);
    });
  }

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

  @override
  void dispose() {
    _hrSub?.cancel();
    _cookieSub?.cancel();
    super.dispose();
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

  void _toast(String msg) {
    final m = ScaffoldMessenger.maybeOf(context);
    m?.showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 2)),
    );
  }

  // 用户历史数据：打开弹窗/底栏加载该站点 90 天快照（自适应浮层）。
  Future<void> _viewHistory(SiteStatItem s) async {
    List<SiteHistoryPoint> history = [];
    bool loading = true;
    try {
      history = await _api.history(s.siteId, days: 90);
    } catch (_) {
      history = const [];
    } finally {
      loading = false;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '${s.siteName} · 历史数据',
      wideWidth: 820,
      builder: (_) => _HistoryBody(
        points: history,
        loading: loading,
        fmtBytes: _fmtBytes,
        fmtTime: _fmtTimeFull,
      ),
    );
  }

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

  void _openSettings() {
    final sites = _overview?.sites ?? const <SiteStatItem>[];
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => _TotalDashboardSettingsSheet(
        initial: _config,
        sites: sites,
        onSave: _saveConfig,
      ),
    );
  }

  String _nowStamp() {
    final d = DateTime.now();
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.year}-${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
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

  Future<void> _exportImage() async {
    await _prepareShareFavicons();
    if (!mounted) return;
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (sheetContext) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 18),
          child: SizedBox(
            height: MediaQuery.sizeOf(sheetContext).height * 0.86,
            child: Column(
              children: [
                const Text(
                  '导出总仪表盘图片',
                  style: TextStyle(fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 12),
                Expanded(
                  child: SingleChildScrollView(
                    child: Center(
                      child: SingleChildScrollView(
                        scrollDirection: Axis.horizontal,
                        child: RepaintBoundary(
                          key: _shareKey,
                          child: Material(
                            color: Colors.transparent,
                            child: _shareCard(),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 14),
                Row(
                  children: [
                    Expanded(
                      child: OutlinedButton.icon(
                        onPressed: _saveImage,
                        icon: const Icon(Icons.save_alt),
                        label: const Text('保存图片'),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: FilledButton.icon(
                        onPressed: _shareImage,
                        icon: const Icon(Icons.share),
                        label: const Text('分享图片'),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  String _dashboardImageFileName() {
    final d = DateTime.now();
    String p(int n) => n.toString().padLeft(2, '0');
    return 'ptpatronus-dashboard-${d.year}${p(d.month)}${p(d.day)}-${p(d.hour)}${p(d.minute)}${p(d.second)}.png';
  }

  Future<Uint8List?> _captureDashboardPng() async {
    final ctx = _shareKey.currentContext;
    final boundary = ctx?.findRenderObject() is RenderRepaintBoundary
        ? ctx!.findRenderObject() as RenderRepaintBoundary
        : null;
    if (boundary == null) return null;
    final img = await boundary.toImage(pixelRatio: 3.0);
    final byteData = await img.toByteData(format: ui.ImageByteFormat.png);
    img.dispose();
    if (byteData == null) return null;
    return byteData.buffer.asUint8List(
      byteData.offsetInBytes,
      byteData.lengthInBytes,
    );
  }

  bool _filePickerSaveWritesBytes() {
    return defaultTargetPlatform == TargetPlatform.android ||
        defaultTargetPlatform == TargetPlatform.iOS;
  }

  Future<String> _writeImageToDocuments(Uint8List png, String fileName) async {
    final dir = await getApplicationDocumentsDirectory();
    final file = File('${dir.path}${Platform.pathSeparator}$fileName');
    await file.writeAsBytes(png, flush: true);
    return file.path;
  }

  Future<void> _saveImage() async {
    String? savedPath;
    try {
      final png = await _captureDashboardPng();
      if (png == null) return;
      final fileName = _dashboardImageFileName();

      try {
        final path = await FilePicker.platform.saveFile(
          dialogTitle: '保存总仪表盘图片',
          fileName: fileName,
          type: FileType.custom,
          allowedExtensions: const ['png'],
          bytes: png,
        );
        if (path == null) return;
        savedPath = path;
        if (!_filePickerSaveWritesBytes()) {
          await File(path).writeAsBytes(png, flush: true);
        }
      } on MissingPluginException {
        savedPath = await _writeImageToDocuments(png, fileName);
      } on UnimplementedError {
        savedPath = await _writeImageToDocuments(png, fileName);
      }

      if (!mounted) return;
      Navigator.pop(context);
      _toast('图片已保存：$savedPath');
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('$e')));
      }
    }
  }

  Future<void> _shareImage() async {
    try {
      final png = await _captureDashboardPng();
      if (png == null) return;
      await Share.shareXFiles([
        XFile.fromData(
          png,
          mimeType: 'image/png',
          name: _dashboardImageFileName(),
        ),
      ]);
      if (mounted) {
        Navigator.pop(context);
      }
    } on MissingPluginException {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('当前平台不支持分享，请使用保存图片')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('$e')));
      }
    }
  }

  Widget _shareCard() {
    final sites = _visibleSites();
    final totalUploaded = _sumInt(sites, (s) => s.uploaded);
    final totalDownloaded = _sumInt(sites, (s) => s.downloaded);
    final totalSeeding = _sumInt(sites, (s) => s.seeding);
    final totalSeedingSize = _sumInt(sites, (s) => s.seedingSize);
    final totalUploads = _sumInt(sites, (s) => s.uploads);
    final totalBonus = _sumDouble(sites, (s) => s.bonus);
    final totalBonusPerHour = _sumDouble(sites, (s) => s.bonusPerHour);
    // 优先真实分享率（totals.true_ratio），避免魔力膨胀虚高。
    final trueRatio = _overview?.trueRatio ?? 0;
    final ratio = trueRatio > 0
        ? trueRatio
        : (totalDownloaded > 0 ? totalUploaded / totalDownloaded : 0.0);
    final displayUser = _shareDisplayUser(sites);
    final shownSites = _config.showSiteList
        ? sites.take(8).toList()
        : <SiteStatItem>[];
    final summaryMetrics = <_ShareMetric>[
      if (_config.showTotalSites)
        _ShareMetric('总站点数', '${sites.length}', const Color(0xFF45C4F0)),
      if (_config.showUploadCount)
        _ShareMetric('总发布数', '$totalUploads', const Color(0xFFFFC857)),
      if (_config.showUpload)
        _ShareMetric('总上传量', _fmtBytes(totalUploaded), const Color(0xFF62D48E)),
      if (_config.showDownload)
        _ShareMetric(
          '总下载量',
          _fmtBytes(totalDownloaded),
          const Color(0xFFFF6B6B),
        ),
      if (_config.showSeedingCount)
        _ShareMetric('总做种数', '$totalSeeding', const Color(0xFF45C4F0)),
      if (_config.showSeedingVolume)
        _ShareMetric(
          '总做种量',
          _fmtBytes(totalSeedingSize),
          const Color(0xFF62D48E),
        ),
      if (_config.showBonus)
        _ShareMetric('总积分值', _fmtCompact(totalBonus), const Color(0xFFFFC857)),
      if (_config.showBonusPerHour)
        _ShareMetric(
          '每小时积分',
          _fmtCompact(totalBonusPerHour),
          const Color(0xFFFF9F43),
        ),
      if (_config.showRatio)
        _ShareMetric('总分享率', _fmtRatio(ratio), const Color(0xFF45C4F0)),
    ];

    return Container(
      width: 360,
      decoration: const BoxDecoration(color: Color(0xFF121212)),
      padding: const EdgeInsets.fromLTRB(18, 18, 18, 16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 34,
                height: 34,
                alignment: Alignment.center,
                decoration: const BoxDecoration(
                  color: Color(0xFF45C4F0),
                  shape: BoxShape.circle,
                ),
                child: Text(
                  _shareUserInitial(displayUser),
                  style: const TextStyle(
                    color: Color(0xFF121212),
                    fontSize: 17,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      displayUser,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: Color(0xFFF8FAFC),
                        fontSize: 18,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 2),
                    const Text(
                      'PT Patronus 总仪表盘',
                      style: TextStyle(color: Color(0xFFB6C2CF), fontSize: 11),
                    ),
                  ],
                ),
              ),
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(
                    _nowStamp(),
                    style: const TextStyle(
                      color: Color(0xFFB6C2CF),
                      fontSize: 10,
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    '详细数据导出',
                    style: TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
                  ),
                ],
              ),
            ],
          ),
          const SizedBox(height: 16),
          const Divider(height: 1, color: Color(0x33FFFFFF)),
          const SizedBox(height: 16),
          _sharePanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  '站点数据统计',
                  style: TextStyle(
                    color: Color(0xFFF8FAFC),
                    fontSize: 16,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 4),
                const Text(
                  '聚合当前总仪表盘筛选范围内的最新数据',
                  style: TextStyle(color: Color(0xFFB6C2CF), fontSize: 10),
                ),
                const SizedBox(height: 14),
                _shareMetricGrid(summaryMetrics, columns: 3),
              ],
            ),
          ),
          if (_config.showRankPanel && sites.isNotEmpty) ...[
            const SizedBox(height: 12),
            _shareRankPanel(),
          ],
          if (shownSites.isNotEmpty) ...[
            const SizedBox(height: 16),
            const Text(
              '分站点用户档案',
              style: TextStyle(
                color: Color(0xFFF8FAFC),
                fontSize: 16,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: 3),
            const Text(
              '用户名、UID、等级等来自 user_profiles 最新真实快照',
              style: TextStyle(color: Color(0xFF7F8B98), fontSize: 10),
            ),
            const SizedBox(height: 10),
            for (var i = 0; i < shownSites.length; i++) ...[
              _shareSiteCard(shownSites[i], i),
              if (i != shownSites.length - 1) const SizedBox(height: 8),
            ],
            if (sites.length > shownSites.length)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  '及其它 ${sites.length - shownSites.length} 站',
                  style: const TextStyle(
                    color: Color(0xFF7F8B98),
                    fontSize: 10,
                  ),
                ),
              ),
          ],
          const SizedBox(height: 16),
          const Divider(height: 1, color: Color(0x33FFFFFF)),
          const SizedBox(height: 10),
          Row(
            children: [
              const Text(
                'Generated by PT Patronus',
                style: TextStyle(color: Color(0xFFB6C2CF), fontSize: 10),
              ),
              const Spacer(),
              Text(
                '${sites.length} sites · ${_nowStamp()}',
                style: const TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _sharePanel({required Widget child, Color? color}) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: color ?? const Color(0xFF1E293B),
        borderRadius: BorderRadius.circular(8),
      ),
      child: child,
    );
  }

  Widget _shareMetricGrid(List<_ShareMetric> metrics, {required int columns}) {
    if (metrics.isEmpty) {
      return const Text(
        '暂无可导出字段',
        style: TextStyle(color: Color(0xFF7F8B98), fontSize: 11),
      );
    }
    final rows = metrics.chunked(columns);
    return Column(
      children: [
        for (var rowIndex = 0; rowIndex < rows.length; rowIndex++)
          Padding(
            padding: EdgeInsets.only(
              bottom: rowIndex == rows.length - 1 ? 0 : 10,
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final metric in rows[rowIndex])
                  Expanded(child: _shareMetric(metric)),
                for (var i = rows[rowIndex].length; i < columns; i++)
                  const Expanded(child: SizedBox()),
              ],
            ),
          ),
      ],
    );
  }

  Widget _shareMetric(_ShareMetric metric) {
    return Padding(
      padding: const EdgeInsets.only(right: 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            metric.label,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
          ),
          const SizedBox(height: 3),
          Text(
            metric.value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              color: metric.color,
              fontSize: 13,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }

  Widget _shareRankPanel() {
    final ranks = <_ShareRank?>[
      if (_config.showUpload)
        _shareRank(
          '上传',
          const Color(0xFF62D48E),
          (s) => s.uploaded,
          (s) => _fmtBytes(s.uploaded),
        ),
      if (_config.showDownload)
        _shareRank(
          '下载',
          const Color(0xFFFF6B6B),
          (s) => s.downloaded,
          (s) => _fmtBytes(s.downloaded),
        ),
      if (_config.showUploadCount)
        _shareRank(
          '发布',
          const Color(0xFFFFC857),
          (s) => s.uploads,
          (s) => '${s.uploads}',
        ),
      if (_config.showSeedingCount)
        _shareRank(
          '做种',
          const Color(0xFF45C4F0),
          (s) => s.seeding,
          (s) => '${s.seeding}',
        ),
      if (_config.showRatio)
        _shareRank(
          '分享率',
          const Color(0xFF45C4F0),
          (s) => s.ratio,
          (s) => _fmtRatio(s.ratio),
        ),
      if (_config.showBonus)
        _shareRank(
          '积分',
          const Color(0xFFB98CFF),
          (s) => s.bonus,
          (s) => _fmtCompact(s.bonus),
        ),
      if (_config.showBonusPerHour)
        _shareRank(
          '每小时积分',
          const Color(0xFFFF9F43),
          (s) => s.bonusPerHour,
          (s) => _fmtCompact(s.bonusPerHour),
        ),
    ].whereType<_ShareRank>().take(6).toList();
    if (ranks.isEmpty) return const SizedBox.shrink();
    return _sharePanel(
      color: const Color(0xFF263238),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            '冠军排行',
            style: TextStyle(
              color: Color(0xFFF8FAFC),
              fontSize: 16,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 12),
          _shareRankGrid(ranks),
        ],
      ),
    );
  }

  Widget _shareRankGrid(List<_ShareRank> ranks) {
    final rows = ranks.chunked(3);
    return Column(
      children: [
        for (var rowIndex = 0; rowIndex < rows.length; rowIndex++)
          Padding(
            padding: EdgeInsets.only(
              bottom: rowIndex == rows.length - 1 ? 0 : 12,
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final rank in rows[rowIndex])
                  Expanded(child: _shareRankCell(rank)),
                for (var i = rows[rowIndex].length; i < 3; i++)
                  const Expanded(child: SizedBox()),
              ],
            ),
          ),
      ],
    );
  }

  _ShareRank? _shareRank(
    String label,
    Color color,
    num Function(SiteStatItem) score,
    String Function(SiteStatItem) value,
  ) {
    final site = _topSiteBy(score);
    if (site == null) return null;
    return _ShareRank(label, value(site), _shareDisplaySiteName(site), color);
  }

  Widget _shareRankCell(_ShareRank rank) {
    return Padding(
      padding: const EdgeInsets.only(right: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                  color: rank.color,
                  shape: BoxShape.circle,
                ),
              ),
              const SizedBox(width: 5),
              Expanded(
                child: Text(
                  rank.label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
                ),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            rank.value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              color: rank.color,
              fontSize: 13,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            rank.siteName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Color(0xFFB6C2CF), fontSize: 9),
          ),
        ],
      ),
    );
  }

  Color _shareSiteAccent(int index) {
    const colors = [
      Color(0xFF45C4F0),
      Color(0xFF62D48E),
      Color(0xFFFFC857),
      Color(0xFFB98CFF),
      Color(0xFFFF9F43),
      Color(0xFFFF6B6B),
    ];
    return colors[index % colors.length];
  }

  Widget _shareSiteIconFallback(SiteStatItem s, int index) {
    final name = _shareDisplaySiteName(s).trim();
    final label = name.isEmpty || name == '--' || name == '***'
        ? '${index + 1}'
        : name.substring(0, 1).toUpperCase();
    return Container(
      width: 28,
      height: 28,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: _shareSiteAccent(index),
        shape: BoxShape.circle,
      ),
      child: Text(
        label,
        style: const TextStyle(
          color: Color(0xFF121212),
          fontSize: 12,
          fontWeight: FontWeight.w800,
        ),
      ),
    );
  }

  Widget _shareSiteIcon(SiteStatItem s, int index) {
    final fallback = _shareSiteIconFallback(s, index);
    final url = _siteFaviconUrl(s);
    if (url == null) return fallback;
    return Container(
      width: 28,
      height: 28,
      clipBehavior: Clip.antiAlias,
      decoration: const BoxDecoration(
        color: Color(0xFFF8FAFC),
        shape: BoxShape.circle,
      ),
      child: Image.network(
        url,
        fit: BoxFit.contain,
        filterQuality: FilterQuality.medium,
        gaplessPlayback: true,
        loadingBuilder: (context, child, progress) =>
            progress == null ? child : fallback,
        errorBuilder: (context, error, stackTrace) => fallback,
      ),
    );
  }

  Widget _shareSiteCard(SiteStatItem s, int index) {
    final metrics = <_ShareMetric>[
      if (_config.showUsername)
        _ShareMetric(
          '用户名',
          _config.blurUserInfo
              ? '***'
              : (s.username.isEmpty ? '--' : s.username),
          const Color(0xFFF8FAFC),
        ),
      if (_config.showUsername)
        _ShareMetric(
          'UID',
          _config.blurUserInfo ? '***' : (s.userId.isEmpty ? '--' : s.userId),
          const Color(0xFFF8FAFC),
        ),
      if (_config.showUsername)
        _ShareMetric(
          '等级',
          _config.blurUserInfo ? '***' : (s.level.isEmpty ? '--' : s.level),
          const Color(0xFFF8FAFC),
        ),
      if (_config.showUploadCount)
        _ShareMetric(
          '发布',
          s.uploads > 0 ? '${s.uploads}' : '--',
          const Color(0xFFFFC857),
        ),
      if (_config.showSeedingCount)
        _ShareMetric('做种', '${s.seeding}', const Color(0xFF45C4F0)),
      if (_config.showUpload)
        _ShareMetric('上传', _fmtBytes(s.uploaded), const Color(0xFF62D48E)),
      if (_config.showDownload)
        _ShareMetric('下载', _fmtBytes(s.downloaded), const Color(0xFFFF6B6B)),
      if (_config.showRatio)
        _ShareMetric('分享率', _fmtRatio(s.ratio), const Color(0xFF45C4F0)),
      if (_config.showSeedingVolume)
        _ShareMetric(
          '做种量',
          s.seedingSize > 0 ? _fmtBytes(s.seedingSize) : '--',
          const Color(0xFF62D48E),
        ),
      if (_config.showBonus)
        _ShareMetric(
          '积分',
          s.bonus > 0 ? _fmtCompact(s.bonus) : '--',
          const Color(0xFFFFC857),
        ),
      if (_config.showBonusPerHour)
        _ShareMetric(
          '每小时积分',
          s.bonusPerHour > 0 ? _fmtCompact(s.bonusPerHour) : '--',
          const Color(0xFFFF9F43),
        ),
    ];
    return _sharePanel(
      color: index.isEven ? const Color(0xFF1E293B) : const Color(0xFF263238),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _shareSiteIcon(s, index),
              const SizedBox(width: 9),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      _shareDisplaySiteName(s),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: Color(0xFFF8FAFC),
                        fontSize: 14,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      _config.showUsername
                          ? _shareDisplayIdentity(s)
                          : '用户信息已隐藏',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: Color(0xFFB6C2CF),
                        fontSize: 10,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          _shareMetricGrid(metrics, columns: 4),
        ],
      ),
    );
  }

  String _shareDisplayUser(List<SiteStatItem> sites) {
    if (_config.blurUserInfo) return '***';
    if (!_config.showUsername) return 'PT 用户';
    final counts = <String, int>{};
    for (final site in sites) {
      final username = site.username.trim();
      if (username.isNotEmpty) {
        counts[username] = (counts[username] ?? 0) + 1;
      }
    }
    if (counts.isEmpty) return 'PT 用户';
    final entries = counts.entries.toList()
      ..sort((a, b) {
        final byCount = b.value.compareTo(a.value);
        return byCount != 0 ? byCount : a.key.compareTo(b.key);
      });
    return entries.first.key;
  }

  String _shareUserInitial(String displayUser) {
    if (displayUser.isEmpty || displayUser == '***') return 'P';
    return displayUser.substring(0, 1).toUpperCase();
  }

  String _shareDisplaySiteName(SiteStatItem site) {
    if (!_config.showSiteName) return '站点信息已隐藏';
    if (_config.blurSensitiveInfo) return '***';
    return site.siteName.isEmpty ? '--' : site.siteName;
  }

  String _shareDisplayIdentity(SiteStatItem site) {
    if (_config.blurUserInfo) return '*** <***> UID ***';
    final parts = <String>[
      if (site.username.isNotEmpty) site.username,
      if (site.userId.isNotEmpty) 'UID ${site.userId}',
      if (site.level.isNotEmpty) '<${site.level}>',
      if (site.email.isNotEmpty) _shareDisplayEmail(site.email),
    ];
    return parts.isEmpty ? '--' : parts.join('  ');
  }

  String _shareDisplayEmail(String email) {
    if (_config.blurUserInfo) return '***';
    final at = email.indexOf('@');
    if (at <= 0) return email;
    final local = email.substring(0, at);
    final domain = email.substring(at + 1);
    final suffix = domain.contains('.') ? domain.split('.').last : '';
    return '${local.substring(0, 1)}***@${suffix.isEmpty ? '***' : '***.$suffix'}';
  }

  @override
  Widget build(BuildContext context) {
    final wide = isWideScreen(context);
    return PTPageScaffold(
      title: '仪表盘',
      toolbarHeight: wide ? 44 : 40,
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
      return const Center(child: CircularProgressIndicator());
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
          _metricGrid(context),
          SizedBox(height: isWideScreen(context) ? 14 : 10),
          _sitesSection(context),
          const SizedBox(height: 4),
        ],
      ),
    );
  }

  // 首登向导：无站点且无下载器时置顶，3 步引导（加站点→配下载器→同步），每步直达对应页。
  Widget _onboardingCard(BuildContext context) {
    final c = HomeShellScope.of(context);
    int indexOf(String glyph) => navItems.indexWhere((n) => n.glyph == glyph);
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(
                Icons.rocket_launch,
                color: PTColors.primary,
                size: 20,
              ),
              const SizedBox(width: 8),
              const Text(
                '欢迎使用 PTPatronus',
                style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
              ),
            ],
          ),
          const SizedBox(height: 4),
          const Text(
            '跟着 3 步，开启账号保号自动化：',
            style: TextStyle(fontSize: 12, color: Colors.grey),
          ),
          const SizedBox(height: 10),
          _onboardStep(
            '1',
            '添加 PT 站点',
            '粘贴 cookie 或 WebView 登录',
            Icons.add_link,
            () => c.select(indexOf('sites')),
          ),
          _onboardStep(
            '2',
            '配置下载器',
            'qBittorrent / Transmission',
            Icons.download_outlined,
            () => c.select(indexOf('downloaders')),
          ),
          _onboardStep('3', '同步统计', '一键拉取，签到 / 辅种自动运行', Icons.sync, _refresh),
        ],
      ),
    );
  }

  Widget _onboardStep(
    String no,
    String title,
    String desc,
    IconData icon,
    VoidCallback onTap,
  ) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: InkWell(
        borderRadius: BorderRadius.circular(10),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
          child: Row(
            children: [
              Container(
                width: 26,
                height: 26,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: PTColors.primary.withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(
                  no,
                  style: const TextStyle(
                    color: PTColors.primary,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Icon(icon, size: 18, color: Colors.grey),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: const TextStyle(fontWeight: FontWeight.w600),
                    ),
                    Text(
                      desc,
                      style: const TextStyle(fontSize: 11, color: Colors.grey),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right, color: Colors.grey, size: 18),
            ],
          ),
        ),
      ),
    );
  }

  // 5 张指标卡（上传/下载/做种/做种体积/魔力），对齐 Web metric-grid（auto-fit minmax 210）。
  // 每卡内嵌近 14 天 sparkline；上传/下载额外显日环比徽标。
  // 窄屏 2 列紧凑（minItemWidth 150）/ 宽屏对齐 Web（210）自然撑高——弃固定 aspect-ratio
  // （曾与 minItemWidth:200 合用致窄屏单列每卡高 ~370px、Spacer 撑出大块留白）。
  Widget _metricGrid(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: EdgeInsets.only(
            left: 2,
            bottom: isWideScreen(context) ? 8 : 6,
          ),
          child: Text(
            '站点数据统计',
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: isWideScreen(context) ? 15 : 14,
            ),
          ),
        ),
        _totalStatsOverviewCard(context),
      ],
    );
  }

  Widget _totalStatsOverviewCard(BuildContext context) {
    final sites = _visibleSites();
    final compact = _config.compactMode || !isWideScreen(context);
    final gap = compact ? 4.0 : 8.0;
    final totalUploaded = _sumInt(sites, (s) => s.uploaded);
    final totalDownloaded = _sumInt(sites, (s) => s.downloaded);
    final totalSeeding = _sumInt(sites, (s) => s.seeding);
    final totalSeedingSize = _sumInt(sites, (s) => s.seedingSize);
    final totalUploads = _sumInt(sites, (s) => s.uploads);
    final totalBonus = _sumDouble(sites, (s) => s.bonus);
    final totalBonusPerHour = _sumDouble(sites, (s) => s.bonusPerHour);
    final ratio = _totalRatio(sites);

    return GlassCard(
      padding: EdgeInsets.all(compact ? 12 : 16),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              children:
                  [
                      if (_config.showTotalSites)
                        _statLabelValue('总站点数', '${sites.length}'),
                      if (_config.showUploadCount)
                        _statLabelValue('总发布数', '$totalUploads'),
                      if (_config.showUpload)
                        _statLabelValue('总上传量', _fmtBytes(totalUploaded)),
                      if (_config.showDownload)
                        _statLabelValue('总下载量', _fmtBytes(totalDownloaded)),
                      if (_config.showSeedingCount)
                        _statLabelValue('总做种数', '$totalSeeding'),
                      if (_config.showSeedingVolume)
                        _statLabelValue('总做种量', _fmtBytes(totalSeedingSize)),
                      if (_config.showBonus)
                        _statLabelValue('总积分值', _fmtCompact(totalBonus)),
                      if (_config.showBonusPerHour)
                        _statLabelValue('小时积分', _fmtCompact(totalBonusPerHour)),
                      if (_config.showRatio)
                        _statLabelValue('总分享率', _fmtRatio(ratio)),
                    ].expand((w) => [w, SizedBox(height: gap)]).toList()
                    ..removeLast(),
            ),
          ),
          if (_config.showRankPanel) ...[
            Container(
              width: 1,
              height: compact ? 150 : 190,
              margin: EdgeInsets.symmetric(horizontal: compact ? 8 : 12),
              color: Theme.of(
                context,
              ).colorScheme.outlineVariant.withValues(alpha: 0.35),
            ),
            Expanded(child: _rankPanel(compact)),
          ],
        ],
      ),
    );
  }

  Widget _statLabelValue(String label, String value) {
    return Row(
      children: [
        Expanded(
          child: Text(
            '$label：',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        Flexible(
          child: Text(
            value,
            textAlign: TextAlign.right,
            style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }

  Widget _rankPanel(bool compact) {
    final rows = <Widget?>[
      if (_config.showUpload)
        _rankRow(
          '上传',
          Icons.upload,
          _topSiteBy((s) => s.uploaded),
          (s) => _fmtBytes(s.uploaded),
          compact,
        ),
      if (_config.showDownload)
        _rankRow(
          '下载',
          Icons.download,
          _topSiteBy((s) => s.downloaded),
          (s) => _fmtBytes(s.downloaded),
          compact,
        ),
      if (_config.showUploadCount)
        _rankRow(
          '发布',
          Icons.publish,
          _topSiteBy((s) => s.uploads),
          (s) => '${s.uploads}',
          compact,
        ),
      if (_config.showSeedingCount)
        _rankRow(
          '做种',
          Icons.park,
          _topSiteBy((s) => s.seeding),
          (s) => '${s.seeding}',
          compact,
        ),
      if (_config.showRatio)
        _rankRow(
          '分享率',
          Icons.equalizer,
          _topSiteBy((s) => s.ratio),
          (s) => _fmtRatio(s.ratio),
          compact,
        ),
      if (_config.showBonus)
        _rankRow(
          '积分',
          Icons.star,
          _topSiteBy((s) => s.bonus),
          (s) => _fmtCompact(s.bonus),
          compact,
        ),
      if (_config.showBonusPerHour)
        _rankRow(
          '小时积分',
          Icons.bolt,
          _topSiteBy((s) => s.bonusPerHour),
          (s) => _fmtCompact(s.bonusPerHour),
          compact,
        ),
    ].whereType<Widget>().toList();
    if (rows.isEmpty) {
      return const Text(
        '暂无排行',
        style: TextStyle(color: Colors.grey, fontSize: 12),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children:
          rows.expand((w) => [w, SizedBox(height: compact ? 4 : 7)]).toList()
            ..removeLast(),
    );
  }

  Widget? _rankRow(
    String label,
    IconData icon,
    SiteStatItem? site,
    String Function(SiteStatItem) value,
    bool compact,
  ) {
    if (site == null) return null;
    return Row(
      children: [
        Icon(icon, size: compact ? 13 : 15, color: PTColors.primary),
        const SizedBox(width: 4),
        SizedBox(
          width: compact ? 42 : 52,
          child: Text(
            label,
            style: TextStyle(color: Colors.grey, fontSize: compact ? 11 : 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        Expanded(
          child: Text(
            value(site),
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: compact ? 11 : 12,
            ),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        if (_config.showSiteName) ...[
          const SizedBox(width: 4),
          Flexible(
            child: Text(
              _displaySiteName(site),
              style: TextStyle(color: Colors.grey, fontSize: compact ? 10 : 11),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ],
    );
  }

  // 站点明细：宽屏表格（对齐 Web el-table），窄屏卡片。操作列含「历史数据」「刷新数据」。
  Widget _sitesSection(BuildContext context) {
    if (!_config.showSiteList) return const SizedBox.shrink();
    final sites = _visibleSites();
    final columns = <PTColumn<SiteStatItem>>[
      PTColumn(
        label: '站点',
        flex: 2,
        cell: (_, s) => Text(
          s.siteName,
          style: const TextStyle(fontWeight: FontWeight.w600),
        ),
      ),
      PTColumn(label: '等级', flex: 1, cell: (_, s) => _levelTag(s.level)),
      PTColumn(
        label: '上传',
        flex: 1,
        numeric: true,
        cell: (_, s) => Text(_fmtBytes(s.uploaded)),
      ),
      PTColumn(
        label: '下载',
        flex: 1,
        numeric: true,
        cell: (_, s) => Text(_fmtBytes(s.downloaded)),
      ),
      PTColumn(
        label: '分享率',
        flex: 1,
        numeric: true,
        cell: (ctx, s) => Text(
          s.ratio > 0 ? s.ratio.toStringAsFixed(2) : '-',
          style: TextStyle(
            color: s.ratio <= 0
                ? (Theme.of(ctx).brightness == Brightness.dark
                      ? Colors.white38
                      : Colors.black38)
                : (s.ratio >= 1 ? PTColors.success : PTColors.danger),
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
      PTColumn(
        label: '做种',
        flex: 1,
        numeric: true,
        cell: (_, s) => Text('${s.seeding}'),
      ),
      PTColumn(label: '操作', flex: 1, cell: (ctx, s) => _rowActions(ctx, s)),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: EdgeInsets.only(
            left: 2,
            bottom: isWideScreen(context) ? 8 : 6,
          ),
          child: Text(
            '站点明细（${sites.length}）',
            style: TextStyle(
              fontWeight: FontWeight.w600,
              fontSize: isWideScreen(context) ? 15 : 14,
            ),
          ),
        ),
        PTAdaptiveTable<SiteStatItem>(
          items: sites,
          columns: columns,
          emptyText: '暂无数据，等待统计同步',
          narrowCard: (c, s) => _siteStatCard(c, s),
          // 宽屏表格最小宽 860：窗口更窄时整表横向滚动（对齐 Web el-table min-width + 横滚），
          // 而非把 7 列硬塞进窗口宽度——否则操作列(flex:1)塞不下两个按钮→黄黑溢出斜纹，
          // 相邻站点列被压到极窄→英文站名逐字竖排（曾见于馒头/MTeam 行的刷新按钮处）。
          // 860÷8(flex 总和)=107.5：操作列够放两枚 40px 按钮，站点列(flex:2)≈215 不压字。
          minTableWidth: 860,
          embedded: true, // 嵌在外层 ListView：窄屏卡片列表 shrinkWrap，避免塌缩空白
        ),
      ],
    );
  }

  Widget _rowActions(BuildContext context, SiteStatItem s) {
    final refreshing = _refreshing.contains(s.siteId);
    final wide = isWideScreen(context);
    // 显式 40×40 命中区 + 零 padding：裸 IconButton 默认 48×48（visualDensity:compact 并不可靠地压到 40），
    // 两枚并排放进表格操作列(flex:1)会溢出→黄黑斜纹。40 在桌面表格（鼠标场景）够用且两枚=80px<列宽。
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        IconButton(
          tooltip: '用户历史数据',
          visualDensity: VisualDensity.compact,
          iconSize: wide ? 18 : 17,
          padding: EdgeInsets.zero,
          constraints: BoxConstraints(
            minWidth: wide ? 40 : 34,
            minHeight: wide ? 40 : 34,
          ),
          onPressed: () => _viewHistory(s),
          icon: const Icon(Icons.history, color: PTColors.primary2),
        ),
        IconButton(
          tooltip: refreshing ? '刷新中…' : '刷新数据',
          visualDensity: VisualDensity.compact,
          iconSize: wide ? 18 : 17,
          padding: EdgeInsets.zero,
          constraints: BoxConstraints(
            minWidth: wide ? 40 : 34,
            minHeight: wide ? 40 : 34,
          ),
          onPressed: refreshing ? null : () => _syncOne(s),
          icon: refreshing
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.refresh, color: PTColors.success),
        ),
      ],
    );
  }

  Widget _levelTag(String level) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: const Color(0x286EA8FE),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        level.isNotEmpty ? level : '-',
        style: const TextStyle(color: Color(0xFF6EA8FE), fontSize: 11),
      ),
    );
  }

  // 字节 → 人类可读（1024 进制，对齐后端 humanBytes）。
  String _fmtBytes(int b) {
    const unit = 1024.0;
    if (b <= 0) return '0 B';
    if (b < unit) return '$b B';
    double n = b / unit;
    int exp = 0;
    while (n >= unit && exp < 5) {
      n /= unit;
      exp++;
    }
    const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
    return '${n.toStringAsFixed(1)} ${units[exp]}';
  }

  String _fmtCompact(double value) {
    if (value <= 0) return '-';
    if (value >= 1000000000) {
      return '${(value / 1000000000).toStringAsFixed(2)} G';
    }
    if (value >= 1000000) return '${(value / 1000000).toStringAsFixed(2)} M';
    if (value >= 1000) return '${(value / 1000).toStringAsFixed(2)} K';
    return value.toStringAsFixed(value >= 100 ? 0 : 1);
  }

  String _fmtRatio(double ratio) {
    if (ratio <= 0) return '-';
    if (ratio >= 10000) return '∞';
    if (ratio >= 100) return ratio.toStringAsFixed(0);
    return ratio.toStringAsFixed(2);
  }

  // RFC3339 →「MM-dd HH:mm」（对齐 Web 站点明细更新时间）。
  String _fmtDate(String s) {
    if (s.isEmpty) {
      return '-';
    }
    final d = DateTime.tryParse(s);
    if (d == null) {
      return s;
    }
    String p(int n) => n.toString().padLeft(2, '0');
    return '${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  // RFC3339 →「YYYY-MM-DD HH:mm」（历史快照时间，对齐 Web fmtTime）。
  String _fmtTimeFull(String s) {
    if (s.isEmpty) {
      return '-';
    }
    final d = DateTime.tryParse(s);
    if (d == null) {
      return s;
    }
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.year}-${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  Widget _statCell(String label, String value) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: const TextStyle(color: Colors.grey, fontSize: 11)),
        const SizedBox(height: 2),
        Text(
          value,
          style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
        ),
      ],
    );
  }

  // 窄屏站点卡：站点名 + 等级 + 分享率 + 三行统计 + 加入/更新 + 操作（历史/刷新）。
  Widget _siteStatCard(BuildContext context, SiteStatItem s) {
    Widget cell(String label, String value) =>
        Expanded(child: _statCell(label, value));
    final metrics = <MapEntry<String, String>>[
      if (_config.showUpload) MapEntry('上传', _fmtBytes(s.uploaded)),
      if (_config.showDownload) MapEntry('下载', _fmtBytes(s.downloaded)),
      if (_config.showSeedingCount) MapEntry('做种', '${s.seeding}'),
      if (_config.showSeedingVolume)
        MapEntry('做种体积', s.seedingSize > 0 ? _fmtBytes(s.seedingSize) : '-'),
      if (_config.showUploadCount)
        MapEntry('发布', s.uploads > 0 ? '${s.uploads}' : '-'),
      MapEntry('下载中', '${s.leeching}'),
      if (_config.showBonus)
        MapEntry('魔力', s.bonus > 0 ? _fmtCompact(s.bonus) : '-'),
      if (_config.showBonusPerHour)
        MapEntry(
          '时魔/h',
          s.bonusPerHour > 0 ? _fmtCompact(s.bonusPerHour) : '-',
        ),
      if (_config.showRatio) MapEntry('分享率', _fmtRatio(s.ratio)),
    ];
    Widget row(List<MapEntry<String, String>> items) => Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final item in items) cell(item.key, item.value),
          for (var i = items.length; i < 3; i++)
            const Expanded(child: SizedBox()),
        ],
      ),
    );
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(12, 10, 10, 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  _displaySiteName(s).isEmpty ? '站点信息已隐藏' : _displaySiteName(s),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontWeight: FontWeight.w700,
                    fontSize: 14,
                  ),
                ),
              ),
              const SizedBox(width: 6),
              if (_config.showUsername && s.level.isNotEmpty)
                _levelTag(_config.blurUserInfo ? '***' : s.level),
              const SizedBox(width: 6),
              if (_config.showRatio)
                Text(
                  '分享率 ${_fmtRatio(s.ratio)}',
                  style: TextStyle(
                    color: s.ratio >= 1 ? PTColors.success : PTColors.danger,
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                  ),
                ),
            ],
          ),
          if (_config.showUsername) ...[
            const SizedBox(height: 3),
            Text(
              _displayUserIdentity(s),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(color: PTColors.primary, fontSize: 11),
            ),
          ],
          const SizedBox(height: 8),
          for (final chunk in metrics.take(9).toList().chunked(3)) row(chunk),
          Row(
            children: [
              Expanded(
                child: Text(
                  '加入 ${s.joinDate.isNotEmpty ? s.joinDate : '-'} · 更新 ${_fmtDate(s.updatedAt)}',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(color: Colors.grey, fontSize: 11),
                ),
              ),
              const SizedBox(width: 8),
              _rowActions(context, s),
            ],
          ),
        ],
      ),
    );
  }

  // 告警卡：Cookie 失效 / H&R 危险（最高优先级，红色左边框，对齐 Web .alert-danger）。
  Widget _alertCard(AlertKind kind) {
    final isCookie = kind == AlertKind.cookie;
    final list = isCookie ? _cookieAlerts : <String>[];
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 14, 14, 14),
      tint: PTColors.danger,
      child: DecoratedBox(
        decoration: BoxDecoration(
          border: Border(left: BorderSide(color: PTColors.danger, width: 3)),
        ),
        child: Padding(
          padding: const EdgeInsets.only(left: 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Icon(
                    isCookie
                        ? Icons.error_outline
                        : Icons.warning_amber_rounded,
                    color: PTColors.danger,
                    size: 20,
                  ),
                  const SizedBox(width: 6),
                  Text(
                    isCookie
                        ? 'Cookie 失效：${list.length} 个站点'
                        : 'H&R 告警：${_hrAlerts.length} 条需关注',
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      color: PTColors.danger,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 6),
              if (isCookie) ...[
                const Text(
                  '以下站点 cookie 已失效，签到/统计/H&R 自动化已停摆，请尽快重新登录：',
                  style: TextStyle(color: Colors.grey, fontSize: 12),
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: list
                      .map(
                        (s) => Chip(
                          label: Text(s),
                          labelStyle: const TextStyle(
                            color: Colors.white,
                            fontSize: 12,
                          ),
                          backgroundColor: PTColors.danger,
                          visualDensity: VisualDensity.compact,
                          padding: EdgeInsets.zero,
                        ),
                      )
                      .toList(),
                ),
              ] else ...[
                ..._hrAlerts
                    .take(5)
                    .map(
                      (a) => Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Container(
                              margin: const EdgeInsets.only(top: 6),
                              width: 6,
                              height: 6,
                              decoration: const BoxDecoration(
                                color: Colors.redAccent,
                                shape: BoxShape.circle,
                              ),
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    a.title,
                                    maxLines: 1,
                                    overflow: TextOverflow.ellipsis,
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                      fontSize: 13,
                                    ),
                                  ),
                                  Text(
                                    '${a.site} · ${a.status}${a.deadline.isNotEmpty ? ' · 剩 ${a.deadline}' : ''}',
                                    style: const TextStyle(
                                      color: Colors.grey,
                                      fontSize: 11,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                if (_hrAlerts.length > 5)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(
                      '…及其它 ${_hrAlerts.length - 5} 条',
                      style: const TextStyle(color: Colors.grey, fontSize: 11),
                    ),
                  ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _TotalDashboardSettingsSheet extends StatefulWidget {
  final TotalDashboardConfig initial;
  final List<SiteStatItem> sites;
  final Future<void> Function(TotalDashboardConfig) onSave;

  const _TotalDashboardSettingsSheet({
    required this.initial,
    required this.sites,
    required this.onSave,
  });

  @override
  State<_TotalDashboardSettingsSheet> createState() =>
      _TotalDashboardSettingsSheetState();
}

class _TotalDashboardSettingsSheetState
    extends State<_TotalDashboardSettingsSheet> {
  late TotalDashboardConfig _config = widget.initial;

  @override
  Widget build(BuildContext context) {
    final maxSites = widget.sites.isEmpty ? 1 : widget.sites.length;
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.fromLTRB(
          16,
          10,
          16,
          16 + MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.sizeOf(context).height * 0.86,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  const Icon(Icons.tune, color: PTColors.primary, size: 20),
                  const SizedBox(width: 8),
                  const Expanded(
                    child: Text(
                      '总仪表盘设置',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 16,
                      ),
                    ),
                  ),
                  IconButton(
                    tooltip: '关闭',
                    onPressed: () => Navigator.pop(context),
                    icon: const Icon(Icons.close),
                  ),
                ],
              ),
              const SizedBox(height: 4),
              Expanded(
                child: ListView(
                  children: [
                    _section('显示内容'),
                    _switch(
                      '总站点数',
                      _config.showTotalSites,
                      (v) => _set(_config.copyWith(showTotalSites: v)),
                    ),
                    _switch(
                      '发布数',
                      _config.showUploadCount,
                      (v) => _set(_config.copyWith(showUploadCount: v)),
                    ),
                    _switch(
                      '上传量',
                      _config.showUpload,
                      (v) => _set(_config.copyWith(showUpload: v)),
                    ),
                    _switch(
                      '下载量',
                      _config.showDownload,
                      (v) => _set(_config.copyWith(showDownload: v)),
                    ),
                    _switch(
                      '做种数',
                      _config.showSeedingCount,
                      (v) => _set(_config.copyWith(showSeedingCount: v)),
                    ),
                    _switch(
                      '做种体积',
                      _config.showSeedingVolume,
                      (v) => _set(_config.copyWith(showSeedingVolume: v)),
                    ),
                    _switch(
                      '积分值/魔力值',
                      _config.showBonus,
                      (v) => _set(_config.copyWith(showBonus: v)),
                    ),
                    _switch(
                      '每小时积分',
                      _config.showBonusPerHour,
                      (v) => _set(_config.copyWith(showBonusPerHour: v)),
                    ),
                    _switch(
                      '分享率',
                      _config.showRatio,
                      (v) => _set(_config.copyWith(showRatio: v)),
                    ),
                    _section('排行榜'),
                    _switch(
                      '显示冠军排行',
                      _config.showRankPanel,
                      (v) => _set(_config.copyWith(showRankPanel: v)),
                    ),
                    _section('站点列表'),
                    _switch(
                      '显示站点详情列表',
                      _config.showSiteList,
                      (v) => _set(_config.copyWith(showSiteList: v)),
                    ),
                    _countSlider(maxSites),
                    _switch(
                      '显示站点名称',
                      _config.showSiteName,
                      (v) => _set(_config.copyWith(showSiteName: v)),
                    ),
                    _switch(
                      '显示用户名',
                      _config.showUsername,
                      (v) => _set(_config.copyWith(showUsername: v)),
                    ),
                    _switch(
                      '模糊站点信息（站名/Logo）',
                      _config.blurSensitiveInfo,
                      (v) => _set(_config.copyWith(blurSensitiveInfo: v)),
                    ),
                    _switch(
                      '模糊用户信息（用户名/UID/等级）',
                      _config.blurUserInfo,
                      (v) => _set(_config.copyWith(blurUserInfo: v)),
                    ),
                    _section('样式'),
                    _switch(
                      '紧凑模式',
                      _config.compactMode,
                      (v) => _set(_config.copyWith(compactMode: v)),
                    ),
                    if (widget.sites.isNotEmpty) ...[
                      _section('显示站点'),
                      _sitePickerSummary(),
                    ],
                  ],
                ),
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  TextButton(
                    onPressed: () => _set(TotalDashboardConfig.defaults),
                    child: const Text('恢复默认'),
                  ),
                  const Spacer(),
                  TextButton(
                    onPressed: () => Navigator.pop(context),
                    child: const Text('取消'),
                  ),
                  const SizedBox(width: 8),
                  FilledButton(
                    onPressed: () async {
                      final navigator = Navigator.of(context);
                      await widget.onSave(_config);
                      if (mounted) navigator.pop();
                    },
                    child: const Text('确定'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _set(TotalDashboardConfig config) {
    setState(() => _config = config);
  }

  Widget _section(String title) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 10, 4, 2),
      child: Text(
        title,
        style: const TextStyle(
          color: PTColors.primary,
          fontWeight: FontWeight.w700,
          fontSize: 13,
        ),
      ),
    );
  }

  Widget _switch(String label, bool value, ValueChanged<bool> onChanged) {
    return SwitchListTile(
      value: value,
      onChanged: onChanged,
      title: Text(label, style: const TextStyle(fontSize: 14)),
      dense: true,
      visualDensity: VisualDensity.compact,
      contentPadding: const EdgeInsets.symmetric(horizontal: 4),
    );
  }

  Widget _countSlider(int maxSites) {
    final value = _config.siteDataLimit.clamp(0, maxSites).toInt();
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 4),
      child: Column(
        children: [
          Row(
            children: [
              const Expanded(
                child: Text('各站点数据数量', style: TextStyle(fontSize: 14)),
              ),
              Text(
                value == 0 ? '全部' : '$value',
                style: const TextStyle(
                  color: PTColors.primary,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
          Slider(
            value: value.toDouble(),
            min: 0,
            max: maxSites.toDouble(),
            divisions: maxSites,
            onChanged: _config.showSiteList
                ? (v) => _set(_config.copyWith(siteDataLimit: v.round()))
                : null,
          ),
        ],
      ),
    );
  }

  Widget _sitePickerSummary() {
    final selected = _config.selectedSiteIds;
    final label = selected.contains(-1)
        ? '全不选'
        : selected.isEmpty
        ? '全部站点'
        : '已选择 ${selected.length} 个站点';
    return ListTile(
      dense: true,
      contentPadding: const EdgeInsets.symmetric(horizontal: 4),
      title: const Text('显示站点', style: TextStyle(fontSize: 14)),
      subtitle: Text(label, maxLines: 1, overflow: TextOverflow.ellipsis),
      trailing: const Icon(Icons.chevron_right),
      onTap: _openSitePicker,
    );
  }

  void _openSitePicker() {
    final selected = _config.selectedSiteIds.toSet();
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => StatefulBuilder(
        builder: (context, setSheetState) {
          return SafeArea(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 10, 16, 16),
              child: SizedBox(
                height: MediaQuery.sizeOf(context).height * 0.72,
                child: Column(
                  children: [
                    Row(
                      children: [
                        const Expanded(
                          child: Text(
                            '选择显示站点',
                            style: TextStyle(
                              fontWeight: FontWeight.w700,
                              fontSize: 16,
                            ),
                          ),
                        ),
                        TextButton(
                          onPressed: () {
                            selected.clear();
                            setSheetState(() {});
                          },
                          child: const Text('全选'),
                        ),
                        TextButton(
                          onPressed: () {
                            selected
                              ..clear()
                              ..add(-1);
                            setSheetState(() {});
                          },
                          child: const Text('全不选'),
                        ),
                      ],
                    ),
                    const SizedBox(height: 6),
                    Expanded(
                      child: ListView.builder(
                        itemCount: widget.sites.length,
                        itemBuilder: (_, i) {
                          final site = widget.sites[i];
                          final none = selected.contains(-1);
                          final checked =
                              !none &&
                              (selected.isEmpty ||
                                  selected.contains(site.siteId));
                          return CheckboxListTile(
                            dense: true,
                            visualDensity: VisualDensity.compact,
                            contentPadding: EdgeInsets.zero,
                            value: checked,
                            title: Text(
                              site.siteName,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                            ),
                            onChanged: (v) {
                              if (selected.contains(-1)) selected.clear();
                              if (selected.isEmpty) {
                                selected.addAll(
                                  widget.sites.map((s) => s.siteId),
                                );
                              }
                              if (v == true) {
                                selected.add(site.siteId);
                              } else {
                                selected.remove(site.siteId);
                              }
                              setSheetState(() {});
                            },
                          );
                        },
                      ),
                    ),
                    FilledButton(
                      onPressed: () {
                        final ids = selected.contains(-1)
                            ? <int>[-1]
                            : selected.length == widget.sites.length
                            ? <int>[]
                            : (selected.toList()..sort());
                        _set(_config.copyWith(selectedSiteIds: ids));
                        Navigator.pop(context);
                      },
                      child: const Text('确定'),
                    ),
                  ],
                ),
              ),
            ),
          );
        },
      ),
    );
  }
}

class _ShareMetric {
  final String label;
  final String value;
  final Color color;

  const _ShareMetric(this.label, this.value, this.color);
}

class _ShareRank {
  final String label;
  final String value;
  final String siteName;
  final Color color;

  const _ShareRank(this.label, this.value, this.siteName, this.color);
}

enum AlertKind { cookie, hr }

// 历史数据浮层内容：宽屏表格 / 窄屏卡片（自适应，对齐 Web 用户历史数据弹窗的 el-table）。
class _HistoryBody extends StatelessWidget {
  final List<SiteHistoryPoint> points;
  final bool loading;
  final String Function(int) fmtBytes;
  final String Function(String) fmtTime;
  const _HistoryBody({
    required this.points,
    required this.loading,
    required this.fmtBytes,
    required this.fmtTime,
  });

  @override
  Widget build(BuildContext context) {
    if (loading) {
      return const Padding(
        padding: EdgeInsets.symmetric(vertical: 40),
        child: Center(child: CircularProgressIndicator()),
      );
    }
    if (points.isEmpty) {
      return const PTEmptyState(text: '暂无历史快照');
    }
    final columns = <PTColumn<SiteHistoryPoint>>[
      PTColumn(
        label: '时间',
        flex: 2,
        cell: (_, p) =>
            Text(fmtTime(p.snapAt), style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '等级',
        flex: 1,
        cell: (_, p) => Text(
          p.level.isNotEmpty ? p.level : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
      PTColumn(
        label: '上传',
        flex: 1,
        numeric: true,
        cell: (_, p) =>
            Text(fmtBytes(p.uploaded), style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '下载',
        flex: 1,
        numeric: true,
        cell: (_, p) =>
            Text(fmtBytes(p.downloaded), style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '分享率',
        flex: 1,
        numeric: true,
        cell: (_, p) => Text(
          p.ratio > 0 ? p.ratio.toStringAsFixed(2) : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
      PTColumn(
        label: '做种',
        flex: 1,
        numeric: true,
        cell: (_, p) =>
            Text('${p.seeding}', style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '发布',
        flex: 1,
        numeric: true,
        cell: (_, p) => Text(
          p.uploads > 0 ? '${p.uploads}' : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
      PTColumn(
        label: '魔力',
        flex: 1,
        numeric: true,
        cell: (_, p) => Text(
          p.bonus > 0 ? p.bonus.toStringAsFixed(0) : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
    ];
    return SizedBox(
      width: double.infinity,
      child: PTAdaptiveTable<SiteHistoryPoint>(
        items: points,
        columns: columns,
        emptyText: '暂无历史快照',
        narrowCard: (_, p) => GlassCard(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                fmtTime(p.snapAt),
                style: const TextStyle(
                  fontWeight: FontWeight.w600,
                  fontSize: 13,
                ),
              ),
              const SizedBox(height: 6),
              Wrap(
                spacing: 12,
                runSpacing: 4,
                children: [
                  Text(
                    '等级 ${p.level.isNotEmpty ? p.level : '-'}',
                    style: const TextStyle(fontSize: 12, color: Colors.grey),
                  ),
                  Text(
                    '上传 ${fmtBytes(p.uploaded)}',
                    style: const TextStyle(fontSize: 12, color: Colors.grey),
                  ),
                  Text(
                    '下载 ${fmtBytes(p.downloaded)}',
                    style: const TextStyle(fontSize: 12, color: Colors.grey),
                  ),
                  Text(
                    '分享率 ${p.ratio > 0 ? p.ratio.toStringAsFixed(2) : '-'}',
                    style: const TextStyle(fontSize: 12, color: Colors.grey),
                  ),
                  Text(
                    '做种 ${p.seeding}',
                    style: const TextStyle(fontSize: 12, color: Colors.grey),
                  ),
                  Text(
                    '魔力 ${p.bonus > 0 ? p.bonus.toStringAsFixed(0) : '-'}',
                    style: const TextStyle(fontSize: 12, color: Colors.grey),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
