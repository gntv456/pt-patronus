import 'dart:async';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/features/home/home_nav_items.dart';
import 'package:flutter/rendering.dart' show RenderRepaintBoundary;
import 'package:flutter/services.dart' show MissingPluginException;
import 'package:fl_chart/fl_chart.dart';
import 'package:ptpatronus/api/dashboard.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:share_plus/share_plus.dart';

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
  final _ws = WsClient();
  final _shareKey = GlobalKey(); // 仪表盘分享图 RepaintBoundary（toImage 截图）
  DashboardOverview? _overview;
  List<DashboardTrendPoint> _trend = [];
  List<HRAlert> _hrAlerts = [];
  List<String> _cookieAlerts = [];
  final Set<int> _refreshing = {}; // 单站刷新中的 site_id 集合
  bool _syncing = false; // 全站同步中（刷新统计按钮 loading）
  bool _loading = true;
  String? _error;
  StreamSubscription? _hrSub;
  StreamSubscription? _cookieSub;

  @override
  void initState() {
    super.initState();
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

  @override
  void dispose() {
    _hrSub?.cancel();
    _cookieSub?.cancel();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final results = await Future.wait([_api.overview(), _api.trend(14)]);
      _overview = results[0] as DashboardOverview;
      _trend = results[1] as List<DashboardTrendPoint>;
      _error = null;
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _refresh() async {
    try {
      final results = await Future.wait([_api.overview(), _api.trend(14)]);
      if (!mounted) return;
      setState(() {
        _overview = results[0] as DashboardOverview;
        _trend = results[1] as List<DashboardTrendPoint>;
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
    m?.showSnackBar(SnackBar(content: Text(msg), duration: const Duration(seconds: 2)));
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
      builder: (_) => _HistoryBody(points: history, loading: loading, fmtBytes: _fmtBytes, fmtTime: _fmtTimeFull),
    );
  }

  // 环比百分比：最后一天 vs 前一天；数据不足或分母为 0 返回 null（仅上传/下载卡用）。
  double? _pct(double Function(DashboardTrendPoint) pick) {
    if (_trend.length < 2) return null;
    final last = pick(_trend.last);
    final prev = pick(_trend[_trend.length - 2]);
    if (prev == 0) return null;
    return (last - prev) / prev * 100;
  }

  List<double> _series(double Function(DashboardTrendPoint) pick) =>
      _trend.map((p) => pick(p).toDouble()).toList();

  String _nowStamp() {
    final d = DateTime.now();
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.year}-${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  void _exportImage() {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          const Text('仪表盘分享图预览', style: TextStyle(fontWeight: FontWeight.w600)),
          const SizedBox(height: 12),
          RepaintBoundary(
            key: _shareKey,
            child: Material(color: Colors.transparent, child: _shareCard()),
          ),
          const SizedBox(height: 14),
          FilledButton.icon(
            onPressed: _share,
            icon: const Icon(Icons.share),
            label: const Text('分享 / 保存图片'),
          ),
        ]),
      ),
    );
  }

  Future<void> _share() async {
    final ctx = _shareKey.currentContext;
    final boundary = ctx?.findRenderObject() is RenderRepaintBoundary
        ? ctx!.findRenderObject() as RenderRepaintBoundary
        : null;
    if (boundary == null) return;
    try {
      final img = await boundary.toImage(pixelRatio: 3.0);
      final byteData = await img.toByteData(format: ui.ImageByteFormat.png);
      if (byteData == null) return;
      final png = byteData.buffer.asUint8List(byteData.offsetInBytes, byteData.lengthInBytes);
      await Share.shareXFiles([
        XFile.fromData(png, mimeType: 'image/png', name: 'ptpatronus-dashboard.png'),
      ]);
      if (mounted) Navigator.pop(context);
    } on MissingPluginException {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(const SnackBar(content: Text('当前平台不支持分享/保存，请用 Web 端导出')));
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    }
  }

  Widget _shareCard() {
    final o = _overview!;
    final upPct = _pct((p) => p.uploaded.toDouble());
    final dlPct = _pct((p) => p.downloaded.toDouble());
    String fmtPct(double? p) => p == null ? '' : '${p >= 0 ? '+' : ''}${p.toStringAsFixed(1)}%';
    return Container(
      width: double.infinity,
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF1A1A2E), Color(0xFF0F0F1E)],
        ),
      ),
      padding: const EdgeInsets.all(20),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('PTPatronus · 仪表盘',
              style: TextStyle(color: Colors.white, fontSize: 22, fontWeight: FontWeight.w700)),
          const SizedBox(height: 4),
          Text('${_nowStamp()} · ${o.siteCount} 站点 · ${o.downloaderCount} 下载器',
              style: const TextStyle(color: Color(0xFF9AA0B4), fontSize: 12)),
          const SizedBox(height: 16),
          GridView.count(
            crossAxisCount: 2,
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            mainAxisSpacing: 10,
            crossAxisSpacing: 10,
            childAspectRatio: 2.6,
            children: [
              _shareTile('总上传', _fmtBytes(o.uploaded), fmtPct(upPct), const Color(0xFF2ECC71)),
              _shareTile('总下载', _fmtBytes(o.downloaded), fmtPct(dlPct), const Color(0xFF4285F4)),
              _shareTile('做种数', '${o.seeding}', '', const Color(0xFF38EF7D)),
              _shareTile('魔力值', o.bonus.toStringAsFixed(1), '', const Color(0xFFF7971E)),
            ],
          ),
          const SizedBox(height: 16),
          const Text('近 14 天上传趋势',
              style: TextStyle(color: Colors.white, fontSize: 15, fontWeight: FontWeight.w600)),
          const SizedBox(height: 8),
          _shareTrend(),
          if (o.sites.isNotEmpty) ...[
            const SizedBox(height: 14),
            const Text('站点身份',
                style: TextStyle(color: Colors.white, fontSize: 15, fontWeight: FontWeight.w600)),
            const SizedBox(height: 8),
            ...o.sites.take(8).map((s) => Padding(
                  padding: const EdgeInsets.only(bottom: 6),
                  child: Row(children: [
                    Flexible(
                      child: Text(s.siteName,
                          maxLines: 1, overflow: TextOverflow.ellipsis,
                          style: const TextStyle(color: Colors.white, fontSize: 13, fontWeight: FontWeight.w600)),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(_identity(s),
                          style: const TextStyle(color: Color(0xFF9AA0B4), fontSize: 11),
                          overflow: TextOverflow.ellipsis),
                    ),
                  ]),
                )),
            if (o.sites.length > 8)
              Padding(
                padding: const EdgeInsets.only(top: 2),
                child: Text('…及其它 ${o.sites.length - 8} 站',
                    style: const TextStyle(color: Color(0xFF6B7088), fontSize: 11)),
              ),
          ],
          const SizedBox(height: 14),
          const Row(children: [
            Text('PTPatronus', style: TextStyle(color: Color(0xFF6B7088), fontSize: 11)),
            Spacer(),
            Text('客户端导出', style: TextStyle(color: Color(0xFF6B7088), fontSize: 11)),
          ]),
        ],
      ),
    );
  }

  String _identity(SiteStatItem s) {
    final parts = <String>[];
    if (s.username.isNotEmpty) parts.add(s.username);
    if (s.userId.isNotEmpty) parts.add('ID ${s.userId}');
    if (s.email.isNotEmpty) parts.add(s.email);
    return parts.join('   ·   ');
  }

  Widget _shareTile(String label, String value, String trend, Color color) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: const Color(0x14FFFFFF),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: const Color(0x1AFFFFFF)),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisAlignment: MainAxisAlignment.center, children: [
        Row(children: [
          Container(width: 4, height: 12, decoration: BoxDecoration(color: color, borderRadius: BorderRadius.circular(2))),
          const SizedBox(width: 6),
          Text(label, style: const TextStyle(color: Color(0xFF9AA0B4), fontSize: 12)),
          const Spacer(),
          if (trend.isNotEmpty) Text(trend, style: const TextStyle(color: Colors.white54, fontSize: 11)),
        ]),
        const SizedBox(height: 6),
        Text(value, style: const TextStyle(color: Colors.white, fontSize: 18, fontWeight: FontWeight.w700)),
      ]),
    );
  }

  // 手动柱状图（不依赖 fl_chart，分享卡内可靠渲染）。
  Widget _shareTrend() {
    if (_trend.isEmpty) {
      return const Padding(padding: EdgeInsets.symmetric(vertical: 16), child: Center(child: Text('暂无趋势数据', style: TextStyle(color: Colors.grey))));
    }
    final data = _trend.map((p) => p.uploaded.toDouble()).toList();
    final peak = data.fold<double>(0, (a, b) => a > b ? a : b);
    final maxY = peak <= 0 ? 100.0 : peak * 1.2;
    return SizedBox(
      height: 110,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: List.generate(_trend.length, (i) {
          final ratio = maxY > 0 ? data[i] / maxY : 0.0;
          final double h = data[i] > 0 ? (ratio * 90).clamp(3.0, 90.0) : 0.0;
          final label = _trend[i].day.length >= 10 ? _trend[i].day.substring(5) : _trend[i].day;
          return Expanded(
            child: Column(mainAxisAlignment: MainAxisAlignment.end, children: [
              Container(
                width: double.infinity,
                margin: const EdgeInsets.symmetric(horizontal: 4),
                height: h,
                decoration: BoxDecoration(
                  gradient: const LinearGradient(
                    colors: [PTColors.primary, PTColors.primary2],
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                  ),
                  borderRadius: const BorderRadius.vertical(top: Radius.circular(4)),
                ),
              ),
              const SizedBox(height: 4),
              Text(label, style: const TextStyle(color: Color(0xFF9AA0B4), fontSize: 11)),
            ]),
          );
        }),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: GlassBackground(
        child: SafeArea(
          child: Padding(
            padding: EdgeInsets.all(isWideScreen(context) ? 20 : 14),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(children: [
                  const HomeMenuButton(),
                  const Text('仪表盘', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
                  const Spacer(),
                  PTPageHeaderActions(actions: [
                    PTHeaderAction(icon: Icons.image_outlined, label: '导出图片', onPressed: (_loading || _overview == null) ? null : _exportImage),
                    PTHeaderAction(icon: Icons.sync, label: '刷新统计', onPressed: _syncing ? null : _syncAll),
                  ]),
                ]),
                const SizedBox(height: 12),
                Expanded(child: _body(context)),
              ],
            ),
          ),
        ),
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
          if (fresh) ...[_onboardingCard(context), const SizedBox(height: 14)],
          if (_cookieAlerts.isNotEmpty) ...[_alertCard(AlertKind.cookie), const SizedBox(height: 14)],
          if (_hrAlerts.isNotEmpty) ...[_alertCard(AlertKind.hr), const SizedBox(height: 14)],
          _metricGrid(context),
          const SizedBox(height: 14),
          _sitesSection(context),
          const SizedBox(height: 8),
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
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.rocket_launch, color: PTColors.primary, size: 20),
          const SizedBox(width: 8),
          const Text('欢迎使用 PTPatronus', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
        ]),
        const SizedBox(height: 4),
        const Text('跟着 3 步，开启账号保号自动化：', style: TextStyle(fontSize: 12, color: Colors.grey)),
        const SizedBox(height: 10),
        _onboardStep('1', '添加 PT 站点', '粘贴 cookie 或 WebView 登录', Icons.add_link, () => c.select(indexOf('sites'))),
        _onboardStep('2', '配置下载器', 'qBittorrent / Transmission', Icons.download_outlined, () => c.select(indexOf('downloaders'))),
        _onboardStep('3', '同步统计', '一键拉取，签到 / 辅种自动运行', Icons.sync, _refresh),
      ]),
    );
  }

  Widget _onboardStep(String no, String title, String desc, IconData icon, VoidCallback onTap) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: InkWell(
        borderRadius: BorderRadius.circular(10),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
          child: Row(children: [
            Container(
              width: 26,
              height: 26,
              alignment: Alignment.center,
              decoration: BoxDecoration(color: PTColors.primary.withValues(alpha: 0.14), borderRadius: BorderRadius.circular(8)),
              child: Text(no, style: const TextStyle(color: PTColors.primary, fontWeight: FontWeight.w700)),
            ),
            const SizedBox(width: 10),
            Icon(icon, size: 18, color: Colors.grey),
            const SizedBox(width: 8),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(title, style: const TextStyle(fontWeight: FontWeight.w600)),
                Text(desc, style: const TextStyle(fontSize: 11, color: Colors.grey)),
              ]),
            ),
            const Icon(Icons.chevron_right, color: Colors.grey, size: 18),
          ]),
        ),
      ),
    );
  }

  // 5 张指标卡（上传/下载/做种/做种体积/魔力），对齐 Web metric-grid（auto-fit minmax 210）。
  // 每卡内嵌近 14 天 sparkline；上传/下载额外显日环比徽标。
  // 窄屏 2 列紧凑（minItemWidth 150）/ 宽屏对齐 Web（210）自然撑高——弃固定 aspect-ratio
  // （曾与 minItemWidth:200 合用致窄屏单列每卡高 ~370px、Spacer 撑出大块留白）。
  Widget _metricGrid(BuildContext context) {
    final o = _overview!;
    final wide = isWideScreen(context);
    final iconSize = wide ? 38.0 : 32.0;
    final upPct = _pct((p) => p.uploaded.toDouble());
    final dlPct = _pct((p) => p.downloaded.toDouble());
    String fmtPct(double? p) => p == null ? '' : '${p >= 0 ? '+' : ''}${p.toStringAsFixed(1)}%';

    final cards = <Widget>[
      _metricCard(
        wide: wide,
        icon: GlassIcon(glyph: 'upload', from: const Color(0xFF2ECC71), to: const Color(0xFF1ABC9C), size: iconSize),
        value: _fmtBytes(o.uploaded),
        label: '总上传',
        trend: fmtPct(upPct),
        trendUp: (upPct ?? 0) >= 0,
        spark: _series((p) => p.uploaded.toDouble()),
        sparkColor: PTColors.primary,
      ),
      _metricCard(
        wide: wide,
        icon: GlassIcon(glyph: 'download', from: const Color(0xFF4285F4), to: const Color(0xFF22D3EE), size: iconSize),
        value: _fmtBytes(o.downloaded),
        label: '总下载',
        trend: fmtPct(dlPct),
        trendUp: (dlPct ?? 0) >= 0,
        spark: _series((p) => p.downloaded.toDouble()),
        sparkColor: PTColors.primary2,
      ),
      _metricCard(
        wide: wide,
        icon: GlassIcon(glyph: 'seeding', from: const Color(0xFF38EF7D), to: const Color(0xFF11998E), size: iconSize),
        value: '${o.seeding}',
        label: '总做种',
        spark: _series((p) => p.seeding.toDouble()),
        sparkColor: PTColors.success,
      ),
      _metricCard(
        wide: wide,
        icon: GlassIcon(glyph: 'backup', from: const Color(0xFF43CEA2), to: const Color(0xFF185A9D), size: iconSize),
        value: _fmtBytes(o.seedingSize),
        label: '总做种体积',
        spark: _series((p) => p.seedingSize.toDouble()),
        sparkColor: const Color(0xFF43CEA2),
      ),
      _metricCard(
        wide: wide,
        icon: GlassIcon(glyph: 'bonus', from: const Color(0xFFF7971E), to: const Color(0xFFFFD200), size: iconSize),
        value: o.bonus.toStringAsFixed(0),
        label: '总魔力',
        spark: _series((p) => p.bonus),
        sparkColor: PTColors.warn,
      ),
    ];

    // 纯 Wrap 非滚动（嵌在外层 ListView 安全），列数随宽度自适应，高度由内容自然决定。
    return LayoutBuilder(builder: (context, c) {
      final cols = ptAutoColumns(c.maxWidth, wide ? 210 : 150);
      final itemW = cols <= 1 ? c.maxWidth : (c.maxWidth - 12 * (cols - 1)) / cols;
      return Wrap(
        spacing: 12,
        runSpacing: 12,
        children: cards.map((card) => SizedBox(width: itemW, child: card)).toList(),
      );
    });
  }

  Widget _metricCard({
    required bool wide,
    required Widget icon,
    required String value,
    required String label,
    String? trend,
    bool trendUp = true,
    required List<double> spark,
    required Color sparkColor,
  }) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              icon,
              if (trend != null && trend.isNotEmpty)
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                  decoration: BoxDecoration(
                    color: (trendUp ? PTColors.success : PTColors.danger).withValues(alpha: 0.14),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(trend,
                      style: TextStyle(
                        color: trendUp ? PTColors.success : PTColors.danger,
                        fontWeight: FontWeight.w600,
                        fontSize: 12,
                      )),
                ),
            ],
          ),
          const SizedBox(height: 10),
          Text(value, style: TextStyle(fontSize: wide ? 24 : 20, fontWeight: FontWeight.w700)),
          const SizedBox(height: 2),
          Text(label, style: const TextStyle(color: Colors.grey, fontSize: 13)),
          const SizedBox(height: 8),
          SizedBox(height: 30, child: _Sparkline(values: spark, color: sparkColor)),
        ],
      ),
    );
  }

  // 站点明细：宽屏表格（对齐 Web el-table），窄屏卡片。操作列含「历史数据」「刷新数据」。
  Widget _sitesSection(BuildContext context) {
    final sites = _overview!.sites;
    final columns = <PTColumn<SiteStatItem>>[
      PTColumn(label: '站点', flex: 2, cell: (_, s) => Text(s.siteName, style: const TextStyle(fontWeight: FontWeight.w600))),
      PTColumn(label: '等级', flex: 1, cell: (_, s) => _levelTag(s.level)),
      PTColumn(label: '上传', flex: 1, numeric: true, cell: (_, s) => Text(_fmtBytes(s.uploaded))),
      PTColumn(label: '下载', flex: 1, numeric: true, cell: (_, s) => Text(_fmtBytes(s.downloaded))),
      PTColumn(
        label: '分享率',
        flex: 1,
        numeric: true,
        cell: (ctx, s) => Text(
          s.ratio > 0 ? s.ratio.toStringAsFixed(2) : '-',
          style: TextStyle(
            color: s.ratio <= 0
                ? (Theme.of(ctx).brightness == Brightness.dark ? Colors.white38 : Colors.black38)
                : (s.ratio >= 1 ? PTColors.success : PTColors.danger),
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
      PTColumn(label: '做种', flex: 1, numeric: true, cell: (_, s) => Text('${s.seeding}')),
      PTColumn(label: '操作', flex: 1, cell: (ctx, s) => _rowActions(ctx, s)),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(left: 2, bottom: 8),
          child: Text('站点明细（${_overview!.siteCount}）',
              style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
        ),
        PTAdaptiveTable<SiteStatItem>(
          items: sites,
          columns: columns,
          emptyText: '暂无数据，等待统计同步',
          narrowCard: (c, s) => _siteStatCard(c, s),
          embedded: true, // 嵌在外层 ListView：窄屏卡片列表 shrinkWrap，避免塌缩空白
        ),
      ],
    );
  }

  Widget _rowActions(BuildContext context, SiteStatItem s) {
    final refreshing = _refreshing.contains(s.siteId);
    return Row(mainAxisSize: MainAxisSize.min, children: [
      IconButton(
        tooltip: '用户历史数据',
        visualDensity: VisualDensity.compact,
        iconSize: 18,
        onPressed: () => _viewHistory(s),
        icon: const Icon(Icons.history, color: PTColors.primary2),
      ),
      IconButton(
        tooltip: refreshing ? '刷新中…' : '刷新数据',
        visualDensity: VisualDensity.compact,
        iconSize: 18,
        onPressed: refreshing ? null : () => _syncOne(s),
        icon: refreshing
            ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
            : const Icon(Icons.refresh, color: PTColors.success),
      ),
    ]);
  }

  Widget _levelTag(String level) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(color: const Color(0x286EA8FE), borderRadius: BorderRadius.circular(10)),
      child: Text(level.isNotEmpty ? level : '-', style: const TextStyle(color: Color(0xFF6EA8FE), fontSize: 11)),
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

  // RFC3339 →「MM-dd HH:mm」（对齐 Web 站点明细更新时间）。
  String _fmtDate(String s) {
    if (s.isEmpty) return '-';
    final d = DateTime.tryParse(s);
    if (d == null) return s;
    String p(int n) => n.toString().padLeft(2, '0');
    return '${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  // RFC3339 →「YYYY-MM-DD HH:mm」（历史快照时间，对齐 Web fmtTime）。
  String _fmtTimeFull(String s) {
    if (s.isEmpty) return '-';
    final d = DateTime.tryParse(s);
    if (d == null) return s;
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.year}-${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  Widget _statCell(String label, String value) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: const TextStyle(color: Colors.grey, fontSize: 11)),
        const SizedBox(height: 2),
        Text(value, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
      ],
    );
  }

  // 窄屏站点卡：站点名 + 等级 + 分享率 + 三行统计 + 加入/更新 + 操作（历史/刷新）。
  Widget _siteStatCard(BuildContext context, SiteStatItem s) {
    Widget cell(String label, String value) => Expanded(child: _statCell(label, value));
    Widget row(List<Widget> cs) => Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: cs),
        );
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Text(s.siteName, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
            const SizedBox(width: 8),
            if (s.level.isNotEmpty) _levelTag(s.level),
            const Spacer(),
            Text('分享率 ${s.ratio > 0 ? s.ratio.toStringAsFixed(2) : '-'}',
                style: TextStyle(
                  color: s.ratio >= 1 ? PTColors.success : PTColors.danger,
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                )),
          ]),
          const SizedBox(height: 10),
          row([cell('上传', _fmtBytes(s.uploaded)), cell('下载', _fmtBytes(s.downloaded)), cell('做种', '${s.seeding}')]),
          row([
            cell('做种体积', s.seedingSize > 0 ? _fmtBytes(s.seedingSize) : '-'),
            cell('发布', s.uploads > 0 ? '${s.uploads}' : '-'),
            cell('下载中', '${s.leeching}'),
          ]),
          row([
            cell('魔力', s.bonus > 0 ? s.bonus.toStringAsFixed(0) : '-'),
            cell('时魔/h', s.bonusPerHour > 0 ? s.bonusPerHour.toStringAsFixed(1) : '-'),
            const Expanded(child: SizedBox()),
          ]),
          Row(children: [
            Text('加入 ${s.joinDate.isNotEmpty ? s.joinDate : '-'} · 更新 ${_fmtDate(s.updatedAt)}', style: const TextStyle(color: Colors.grey, fontSize: 11)),
            const Spacer(),
            _rowActions(context, s),
          ]),
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
        decoration: BoxDecoration(border: Border(left: BorderSide(color: PTColors.danger, width: 3))),
        child: Padding(
          padding: const EdgeInsets.only(left: 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(children: [
                Icon(isCookie ? Icons.error_outline : Icons.warning_amber_rounded, color: PTColors.danger, size: 20),
                const SizedBox(width: 6),
                Text(isCookie ? 'Cookie 失效：${list.length} 个站点' : 'H&R 告警：${_hrAlerts.length} 条需关注',
                    style: const TextStyle(fontWeight: FontWeight.w700, color: PTColors.danger)),
              ]),
              const SizedBox(height: 6),
              if (isCookie) ...[
                const Text('以下站点 cookie 已失效，签到/统计/H&R 自动化已停摆，请尽快重新登录：',
                    style: TextStyle(color: Colors.grey, fontSize: 12)),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: list
                      .map((s) => Chip(
                            label: Text(s),
                            labelStyle: const TextStyle(color: Colors.white, fontSize: 12),
                            backgroundColor: PTColors.danger,
                            visualDensity: VisualDensity.compact,
                            padding: EdgeInsets.zero,
                          ))
                      .toList(),
                ),
              ] else ...[
                ..._hrAlerts.take(5).map((a) => Padding(
                      padding: const EdgeInsets.only(top: 6),
                      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                        Container(margin: const EdgeInsets.only(top: 6), width: 6, height: 6, decoration: const BoxDecoration(color: Colors.redAccent, shape: BoxShape.circle)),
                        const SizedBox(width: 8),
                        Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Text(a.title, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13)),
                          Text('${a.site} · ${a.status}${a.deadline.isNotEmpty ? ' · 剩 ${a.deadline}' : ''}', style: const TextStyle(color: Colors.grey, fontSize: 11)),
                        ])),
                      ]),
                    )),
                if (_hrAlerts.length > 5)
                  Padding(padding: const EdgeInsets.only(top: 6), child: Text('…及其它 ${_hrAlerts.length - 5} 条', style: const TextStyle(color: Colors.grey, fontSize: 11))),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

enum AlertKind { cookie, hr }

// 指标卡内嵌迷你折线（近 14 天，无坐标轴），对齐 Web components/dashboard/Sparkline.vue。
class _Sparkline extends StatelessWidget {
  final List<double> values;
  final Color color;
  const _Sparkline({required this.values, required this.color});

  @override
  Widget build(BuildContext context) {
    if (values.length < 2) return const SizedBox.shrink();
    final minV = values.reduce((a, b) => a < b ? a : b);
    final maxV = values.reduce((a, b) => a > b ? a : b);
    final span = (maxV - minV) == 0 ? 1.0 : (maxV - minV).toDouble();
    return LineChart(
      LineChartData(
        minX: 0,
        maxX: (values.length - 1).toDouble(),
        minY: 0,
        maxY: 1,
        lineTouchData: const LineTouchData(enabled: false),
        titlesData: const FlTitlesData(show: false),
        gridData: const FlGridData(show: false),
        borderData: FlBorderData(show: false),
        lineBarsData: [
          LineChartBarData(
            isCurved: true,
            curveSmoothness: 0.3,
            preventCurveOverShooting: true,
            barWidth: 2,
            dotData: const FlDotData(show: false),
            belowBarData: BarAreaData(
              show: true,
              color: color.withValues(alpha: 0.16),
            ),
            gradient: LinearGradient(colors: [color.withValues(alpha: 0.7), color]),
            spots: [
              for (int i = 0; i < values.length; i++)
                FlSpot(i.toDouble(), (values[i] - minV) / span),
            ],
          ),
        ],
        extraLinesData: const ExtraLinesData(),
      ),
    );
  }
}

// 历史数据浮层内容：宽屏表格 / 窄屏卡片（自适应，对齐 Web 用户历史数据弹窗的 el-table）。
class _HistoryBody extends StatelessWidget {
  final List<SiteHistoryPoint> points;
  final bool loading;
  final String Function(int) fmtBytes;
  final String Function(String) fmtTime;
  const _HistoryBody({required this.points, required this.loading, required this.fmtBytes, required this.fmtTime});

  @override
  Widget build(BuildContext context) {
    if (loading) {
      return const Padding(padding: EdgeInsets.symmetric(vertical: 40), child: Center(child: CircularProgressIndicator()));
    }
    if (points.isEmpty) {
      return const PTEmptyState(text: '暂无历史快照');
    }
    final columns = <PTColumn<SiteHistoryPoint>>[
      PTColumn(label: '时间', flex: 2, cell: (_, p) => Text(fmtTime(p.snapAt), style: const TextStyle(fontSize: 12))),
      PTColumn(label: '等级', flex: 1, cell: (_, p) => Text(p.level.isNotEmpty ? p.level : '-', style: const TextStyle(fontSize: 12))),
      PTColumn(label: '上传', flex: 1, numeric: true, cell: (_, p) => Text(fmtBytes(p.uploaded), style: const TextStyle(fontSize: 12))),
      PTColumn(label: '下载', flex: 1, numeric: true, cell: (_, p) => Text(fmtBytes(p.downloaded), style: const TextStyle(fontSize: 12))),
      PTColumn(label: '分享率', flex: 1, numeric: true, cell: (_, p) => Text(p.ratio > 0 ? p.ratio.toStringAsFixed(2) : '-', style: const TextStyle(fontSize: 12))),
      PTColumn(label: '做种', flex: 1, numeric: true, cell: (_, p) => Text('${p.seeding}', style: const TextStyle(fontSize: 12))),
      PTColumn(label: '发布', flex: 1, numeric: true, cell: (_, p) => Text(p.uploads > 0 ? '${p.uploads}' : '-', style: const TextStyle(fontSize: 12))),
      PTColumn(label: '魔力', flex: 1, numeric: true, cell: (_, p) => Text(p.bonus > 0 ? p.bonus.toStringAsFixed(0) : '-', style: const TextStyle(fontSize: 12))),
    ];
    return SizedBox(
      width: double.infinity,
      child: PTAdaptiveTable<SiteHistoryPoint>(
        items: points,
        columns: columns,
        emptyText: '暂无历史快照',
        narrowCard: (_, p) => GlassCard(
          padding: const EdgeInsets.all(12),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(fmtTime(p.snapAt), style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13)),
            const SizedBox(height: 6),
            Wrap(spacing: 12, runSpacing: 4, children: [
              Text('等级 ${p.level.isNotEmpty ? p.level : '-'}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
              Text('上传 ${fmtBytes(p.uploaded)}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
              Text('下载 ${fmtBytes(p.downloaded)}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
              Text('分享率 ${p.ratio > 0 ? p.ratio.toStringAsFixed(2) : '-'}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
              Text('做种 ${p.seeding}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
              Text('魔力 ${p.bonus > 0 ? p.bonus.toStringAsFixed(0) : '-'}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
            ]),
          ]),
        ),
      ),
    );
  }
}
