import 'dart:async';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart' show RenderRepaintBoundary;
import 'package:flutter/services.dart' show MissingPluginException;
import 'package:fl_chart/fl_chart.dart';
import 'package:ptpatronus/api/dashboard.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:share_plus/share_plus.dart';

// 仪表盘：跨站统计汇总（顶部 4 卡）+ 近 7 天趋势（柱状图，可切换 上传/下载/做种/做种体积/魔力/分享率）。
// 数据来自 GET /dashboard（overview）与 GET /dashboard/trend?days=7（趋势）。
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
  int _metricIdx = 0; // 趋势图当前指标（_metricDefs 下标）
  List<HRAlert> _hrAlerts = [];
  List<String> _cookieAlerts = [];
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
      // 并行拉 overview 与趋势，减少等待。
      final results = await Future.wait([_api.overview(), _api.trend(7)]);
      _overview = results[0] as DashboardOverview;
      _trend = results[1] as List<DashboardTrendPoint>;
      _error = null;
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  // 环比百分比：最后一天 vs 前一天；数据不足或分母为 0 返回 null。
  double? _pct(double Function(DashboardTrendPoint) pick) {
    if (_trend.length < 2) return null;
    final last = pick(_trend.last);
    final prev = pick(_trend[_trend.length - 2]);
    if (prev == 0) return null;
    return (last - prev) / prev * 100;
  }

  // 绝对增量：最后一天 - 前一天。
  double _delta(double Function(DashboardTrendPoint) pick) {
    if (_trend.length < 2) return 0;
    return pick(_trend.last) - pick(_trend[_trend.length - 2]);
  }

  String _nowStamp() {
    final d = DateTime.now();
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.year}-${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  // 导出仪表盘分享图：弹预览底栏（RepaintBoundary 包裹分享卡）→ 点「分享」toImage→share_plus。
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

  // 分享卡：深色品牌渐变 + 标题 + 4 指标 + 7 天趋势柱 + 页脚（toImage 截图）。
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
          const Text('近 7 天上传趋势',
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
                    Text(s.siteName,
                        style: const TextStyle(color: Colors.white, fontSize: 13, fontWeight: FontWeight.w600)),
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

  // 站点身份串：用户名 / ID / 邮箱（空值跳过，best-effort，抓不到的字段不显示）。
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
          if (trend.isNotEmpty)
            Text(trend, style: const TextStyle(color: Colors.white54, fontSize: 11)),
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
              Text(label, style: const TextStyle(color: Color(0xFF9AA0B4), fontSize: 10)),
            ]),
          );
        }),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final wide = MediaQuery.of(context).size.width > 720;
    return Scaffold(
      body: GlassBackground(
        child: SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(children: [
                  const Text('仪表盘',
                      style: TextStyle(fontSize: 24, fontWeight: FontWeight.w700)),
                  const Spacer(),
                  IconButton(
                    icon: const Icon(Icons.image_outlined),
                    onPressed: (_loading || _overview == null) ? null : _exportImage,
                    tooltip: '导出图片',
                  ),
                  IconButton(
                    icon: const Icon(Icons.refresh),
                    onPressed: _loading ? null : _load,
                    tooltip: '刷新',
                  ),
                ]),
                const SizedBox(height: 16),
                Expanded(child: _body(isDark, wide)),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _body(bool isDark, bool wide) {
    if (_loading && _overview == null) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_overview == null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(_error == null ? '暂无数据' : '加载失败：$_error',
                style: const TextStyle(color: Colors.grey)),
            const SizedBox(height: 12),
            FilledButton(onPressed: _load, child: const Text('重试')),
          ],
        ),
      );
    }
    return ListView(
      padding: EdgeInsets.zero,
      children: [
        if (_cookieAlerts.isNotEmpty) ...[
          _cookieAlertCard(),
          const SizedBox(height: 14),
        ],
        if (_hrAlerts.isNotEmpty) ...[
          _hrAlertCard(),
          const SizedBox(height: 14),
        ],
        GridView.count(
          crossAxisCount: wide ? 4 : 2,
          shrinkWrap: true,
          physics: const NeverScrollableScrollPhysics(),
          mainAxisSpacing: 14,
          crossAxisSpacing: 14,
          childAspectRatio: 1.1,
          children: _metrics(),
        ),
        const SizedBox(height: 18),
        SizedBox(
          height: 224,
          child: GlassCard(
            padding: const EdgeInsets.fromLTRB(16, 16, 24, 16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('近 7 天 ${_metricDefs[_metricIdx].label} 趋势',
                    style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
                const SizedBox(height: 8),
                _metricChips(),
                const SizedBox(height: 6),
                Expanded(child: _trendChart(isDark)),
              ],
            ),
          ),
        ),
        const SizedBox(height: 18),
        Padding(
          padding: const EdgeInsets.only(left: 2, bottom: 4),
          child: Text('站点明细（${_overview!.siteCount}）',
              style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
        ),
        if (_overview!.sites.isEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
                child: Text('暂无数据，等待统计同步',
                    style: TextStyle(color: Colors.grey))),
          )
        else
          ..._overview!.sites
              .map((s) => Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: _siteStatCard(s),
                  ))
              ,
        const SizedBox(height: 8),
      ],
    );
  }

  List<Widget> _metrics() {
    final o = _overview!;
    final upPct = _pct((p) => p.uploaded.toDouble());
    final dlPct = _pct((p) => p.downloaded.toDouble());
    final sdDelta = _delta((p) => p.seeding.toDouble()).toInt();
    final bnDelta = _delta((p) => p.bonus);

    String fmtPct(double? p) {
      if (p == null) return '—';
      return '${p >= 0 ? '+' : ''}${p.toStringAsFixed(1)}%';
    }

    String fmtDelta(double d) {
      if (d == 0) return '—';
      return '${d > 0 ? '+' : ''}${d.toStringAsFixed(d.abs() < 10 ? 1 : 0)}';
    }

    return [
      _card('⬆️', _fmtBytes(o.uploaded), '总上传', fmtPct(upPct), (upPct ?? 0) >= 0),
      _card('⬇️', _fmtBytes(o.downloaded), '总下载', fmtPct(dlPct), (dlPct ?? 0) >= 0),
      _card('🌱', '${o.seeding}', '做种数', fmtDelta(sdDelta.toDouble()), sdDelta >= 0),
      _card('✨', o.bonus.toStringAsFixed(1), '魔力值', fmtDelta(bnDelta), bnDelta >= 0),
    ];
  }

  Widget _card(String icon, String value, String label, String trend, bool up) {
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(icon, style: const TextStyle(fontSize: 18)),
              Text(trend,
                  style: TextStyle(
                    color: up ? PTColors.success : PTColors.danger,
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                  )),
            ],
          ),
          Text(value, style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w700)),
          Text(label, style: const TextStyle(color: Colors.grey, fontSize: 13)),
        ],
      ),
    );
  }

  // 趋势可选指标：label + 取值函数 + 是否字节量纲（轴标签格式化用）。
  // fl_chart 单 Y 轴，故移动端为「单指标切换」而非 Web 的多系列叠加；6 指标全可选。
  List<({String label, double Function(DashboardTrendPoint) pick, bool bytes})>
      get _metricDefs => [
            (label: '上传', pick: (p) => p.uploaded.toDouble(), bytes: true),
            (label: '下载', pick: (p) => p.downloaded.toDouble(), bytes: true),
            (label: '做种体积', pick: (p) => p.seedingSize.toDouble(), bytes: true),
            (label: '做种', pick: (p) => p.seeding.toDouble(), bytes: false),
            (label: '魔力', pick: (p) => p.bonus, bytes: false),
            (label: '分享率', pick: (p) => p.ratio, bytes: false),
          ];

  Widget _metricChips() {
    return Wrap(
      spacing: 6,
      runSpacing: 4,
      children: List.generate(_metricDefs.length, (i) {
        return ChoiceChip(
          label: Text(_metricDefs[i].label, style: const TextStyle(fontSize: 11)),
          selected: _metricIdx == i,
          visualDensity: VisualDensity.compact,
          materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
          onSelected: (_) => setState(() => _metricIdx = i),
        );
      }),
    );
  }

  Widget _trendChart(bool isDark) {
    if (_trend.isEmpty) {
      return const Center(child: Text('暂无趋势数据', style: TextStyle(color: Colors.grey)));
    }
    final pick = _metricDefs[_metricIdx].pick;
    final data = _trend.map(pick).toList();
    final labels = _trend.map((p) => p.day.length >= 10 ? p.day.substring(5) : p.day).toList();
    return _TrendChart(data: data, labels: labels, isDark: isDark);
  }

  // 字节 → 人类可读（1024 进制，对齐后端 humanBytes）。
  String _fmtBytes(int b) {
    const unit = 1024.0;
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

  // RFC3339 →「MM-dd HH:mm」（对齐后端 updated_at 时间）。
  String _fmtDate(String s) {
    if (s.isEmpty) return '-';
    final d = DateTime.tryParse(s);
    if (d == null) return s;
    String p(int n) => n.toString().padLeft(2, '0');
    return '${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  // 单格统计：灰标签 + 值（承载新同步的字段）。
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

  // 每站统计卡：站点名 + 等级 + 分享率 + 三行统计网格 + 加入/更新时间。
  Widget _siteStatCard(SiteStatItem s) {
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
          Row(
            children: [
              Text(s.siteName, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              const SizedBox(width: 8),
              if (s.level.isNotEmpty)
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                  decoration: BoxDecoration(
                    color: const Color(0x286EA8FE),
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: Text(s.level, style: const TextStyle(color: Color(0xFF6EA8FE), fontSize: 11)),
                ),
              const Spacer(),
              Text(
                '分享率 ${s.ratio > 0 ? s.ratio.toStringAsFixed(2) : '-'}',
                style: TextStyle(
                  color: s.ratio >= 1 ? PTColors.success : PTColors.danger,
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          row([
            cell('上传', _fmtBytes(s.uploaded)),
            cell('下载', _fmtBytes(s.downloaded)),
            cell('做种', '${s.seeding}'),
          ]),
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
          Row(
            children: [
              Text('加入 ${s.joinDate.isNotEmpty ? s.joinDate : '-'}',
                  style: const TextStyle(color: Colors.grey, fontSize: 11)),
              const Spacer(),
              Text('更新 ${_fmtDate(s.updatedAt)}',
                  style: const TextStyle(color: Colors.grey, fontSize: 11)),
            ],
          ),
        ],
      ),
    );
  }

  // Cookie 失效告警卡：最高优先级（danger），自动化已停摆，列失效站点。
  Widget _cookieAlertCard() {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            const Icon(Icons.error_outline, color: Colors.red, size: 20),
            const SizedBox(width: 6),
            Text('Cookie 失效：${_cookieAlerts.length} 个站点',
                style: const TextStyle(fontWeight: FontWeight.w700, color: Colors.red)),
          ]),
          const SizedBox(height: 6),
          const Text('以下站点 cookie 已失效，签到/统计/H&R 自动化已停摆，请尽快重新登录：',
              style: TextStyle(color: Colors.grey, fontSize: 12)),
          const SizedBox(height: 8),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: _cookieAlerts
                .map((s) => Chip(
                      label: Text(s),
                      labelStyle: const TextStyle(color: Colors.white, fontSize: 12),
                      backgroundColor: Colors.red,
                      visualDensity: VisualDensity.compact,
                      padding: EdgeInsets.zero,
                    ))
                .toList(),
          ),
        ],
      ),
    );
  }

  // H&R 告警卡：危险种子列表（站点/标题/状态/剩余），最多展示 5 条。
  Widget _hrAlertCard() {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            const Icon(Icons.warning_amber_rounded, color: Colors.redAccent, size: 20),
            const SizedBox(width: 6),
            Text('H&R 告警：${_hrAlerts.length} 条需关注',
                style: const TextStyle(fontWeight: FontWeight.w700, color: Colors.redAccent)),
          ]),
          const SizedBox(height: 8),
          ..._hrAlerts.take(5).map((a) => Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Container(
                      margin: const EdgeInsets.only(top: 6),
                      width: 6,
                      height: 6,
                      decoration: const BoxDecoration(color: Colors.redAccent, shape: BoxShape.circle),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(a.title, maxLines: 1, overflow: TextOverflow.ellipsis,
                              style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13)),
                          Text('${a.site} · ${a.status}${a.deadline.isNotEmpty ? ' · 剩 ${a.deadline}' : ''}',
                              style: const TextStyle(color: Colors.grey, fontSize: 11)),
                        ],
                      ),
                    ),
                  ],
                ),
              )),
          if (_hrAlerts.length > 5)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text('…及其它 ${_hrAlerts.length - 5} 条',
                  style: const TextStyle(color: Colors.grey, fontSize: 11)),
            ),
        ],
      ),
    );
  }
}

// 上传趋势柱状图：data 为每日上传量，labels 为日期（MM-DD）。
class _TrendChart extends StatelessWidget {
  final List<double> data;
  final List<String> labels;
  final bool isDark;
  const _TrendChart({required this.data, required this.labels, required this.isDark});

  @override
  Widget build(BuildContext context) {
    final gridColor = isDark ? Colors.white24 : Colors.black12;
    final peak = data.fold<double>(0, (a, b) => a > b ? a : b);
    final maxY = peak <= 0 ? 100.0 : peak * 1.2;
    return BarChart(
      BarChartData(
        alignment: BarChartAlignment.spaceAround,
        maxY: maxY,
        barTouchData: BarTouchData(enabled: false),
        titlesData: FlTitlesData(
          topTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
          rightTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
          leftTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
          bottomTitles: AxisTitles(
            sideTitles: SideTitles(
              showTitles: true,
              reservedSize: 28,
              getTitlesWidget: (v, _) {
                final i = v.toInt();
                if (i < 0 || i >= labels.length) return const SizedBox.shrink();
                return Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Text(labels[i], style: const TextStyle(fontSize: 10)),
                );
              },
            ),
          ),
        ),
        gridData: FlGridData(
          show: true,
          drawVerticalLine: false,
          getDrawingHorizontalLine: (v) => FlLine(color: gridColor, strokeWidth: 1),
        ),
        borderData: FlBorderData(show: false),
        barGroups: List.generate(data.length, (i) {
          return BarChartGroupData(
            x: i,
            barRods: [
              BarChartRodData(
                toY: data[i],
                width: 16,
                borderRadius: const BorderRadius.vertical(top: Radius.circular(6)),
                gradient: const LinearGradient(
                  colors: [PTColors.primary, PTColors.primary2],
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                ),
              ),
            ],
          );
        }),
      ),
    );
  }
}
