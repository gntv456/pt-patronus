// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// MonitorDetailPage 的 MonitorDetailOpsMixin mixin（含 setState 段；从 monitor_detail_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/monitor/monitor_detail_page.dart';

mixin MonitorDetailOpsMixin on State<MonitorDetailPage> {
  // 宿主 _MonitorDetailPageState 提供的钩子。
  void _toast(String m);

  final _api = MonitorApi();
  Metrics? _cur;
  List<MetricSnapshot> _history = const [];

  // 趋势图取数窗口：后端一次只给最近 limit 条，被截断时要在图上说清楚。
  TrendWindow _trendWindow = const TrendWindow(shown: 0);
  List<MonitorAlert> _alerts = const [];
  bool _loading = false;
  bool _probing = false;
  // 摄像头预览
  Player? _player;
  VideoController? _controller;
  String? _playerErr; // 鸿蒙等无 media_kit 原生时的降级文案
  bool get _isCamera => widget.target.type == 'camera';
  String get _resolvedProtocol {
    final t = widget.target;
    if (t.protocol.isNotEmpty) return t.protocol;
    if (t.type == 'nas') return t.host.isEmpty ? 'local' : 'api';
    if (t.type == 'camera') return 'onvif';
    return t.host.isEmpty ? 'local' : 'ssh';
  }

  bool get _canPower {
    final t = widget.target;
    if (t.type == 'camera') return false;
    if (t.type == 'host') {
      return _resolvedProtocol == 'local' || _resolvedProtocol == 'ssh';
    }
    if (t.type == 'nas') {
      return _resolvedProtocol == 'local' || _resolvedProtocol == 'api';
    }
    return false;
  }

  Future<void> _initPlayer() async {
    try {
      final p = Player();
      final c = VideoController(p);
      await p.open(Media(_api.streamURL(widget.target.id)));
      if (!mounted) {
        p.dispose(); // 页面已退出，回收播放器避免泄漏
        return;
      }
      setState(() {
        _player = p;
        _controller = c;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() => _playerErr = '当前平台不支持实时画面（约 5-10s 延迟），请在 Web 端查看');
    }
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final results = await Future.wait([
        _api.collect(widget.target.id),
        _api.metrics(widget.target.id, limit: 60),
        _api.alerts(targetId: widget.target.id),
      ]);
      if (!mounted) return;
      final page = results[1] as MetricsPage;
      final collected = results[0] as CollectResult;
      // 采集失败时后端回的是 200 + {ok:false, error:'中文原因'}。从前这里只取
      // metrics，ok/error 一字不读 ⇒「这次没采到」和「刚采到新数」在页面上同形（#251）。
      if (!collected.ok) {
        _toast('本次采集失败：${collected.error ?? "未知原因"}（下方为历史数据）');
      }
      setState(() {
        _cur = collected.metrics;
        _history = page.snapshots;
        _trendWindow = TrendWindow(
          shown: page.snapshots.length,
          total: page.total,
          truncated: page.truncated,
        );
        _alerts = results[2] as List<MonitorAlert>;
        _loading = false;
      });
    } catch (e) {
      if (mounted) {
        _toast(e.toString().replaceFirst('Exception: ', ''));
        setState(() => _loading = false);
      }
    }
  }

  Future<void> _probe() async {
    setState(() => _probing = true);
    try {
      final r = await _api.probe(widget.target.id);
      _toast(r.ok && r.online != false ? '在线' : (r.error ?? '离线/失败'));
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _probing = false);
    }
  }
}
