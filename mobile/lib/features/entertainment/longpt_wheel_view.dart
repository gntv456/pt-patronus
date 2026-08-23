import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// LongPT 幸运大转盘：Canvas 圆形扇区（奖品来自站点 API）+ 旋转动画。
// 单抽取 prizeId 定位扇区；连抽转几圈后展示聚合结果。
class LongPTWheelView extends StatefulWidget {
  final int siteId;
  const LongPTWheelView({super.key, required this.siteId});
  @override
  State<LongPTWheelView> createState() => _LongPTWheelViewState();
}

// 扇区颜色循环（对齐 LongPT 原图明亮配色）。
const _wheelColors = <int>[
  0xFFFF6B6B, 0xFFFFD93D, 0xFF6BCB77, 0xFF4D96FF, 0xFF9D65C9, 0xFFFF9F43,
  0xFF1ABC9C, 0xFFE84393, 0xFFFDCB6E, 0xFF74B9FF, 0xFFA29BFE, 0xFF55EFC4,
];

class _LongPTWheelViewState extends State<LongPTWheelView>
    with SingleTickerProviderStateMixin {
  static const _game = 'longpt_wheel';
  static const _multi = 10;

  final _api = EntertainmentApi();
  List<PrizeDef> _prizes = [];
  GameStats? _stats;
  List<GameRecord> _records = [];
  bool _spinning = false;
  PlayResult? _result;
  int _mode = 1;

  late AnimationController _ctrl;
  double _rotation = 0; // 累计旋转角度（弧度）
  double _spinFrom = 0;
  double _spinTo = 0;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(vsync: this, duration: const Duration(seconds: 4));
    _ctrl.addListener(() {
      if (mounted) setState(() {});
    });
    _ctrl.addStatusListener((s) {
      if (s == AnimationStatus.completed) _onSpinEnd();
    });
    _load();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      _prizes = await _api.prizes(widget.siteId, _game);
    } catch (_) {}
    await _refresh();
    if (mounted) setState(() {});
  }

  Future<void> _refresh() async {
    try {
      _stats = await _api.stats(widget.siteId, _game);
    } catch (_) {}
    try {
      _records = await _api.history(widget.siteId, _game);
    } catch (_) {}
  }

  int _idToIndex(int id) {
    final i = _prizes.indexWhere((p) => p.id == id);
    return i >= 0 ? i : 0;
  }

  double _targetForIndex(int index) {
    final n = _prizes.isNotEmpty ? _prizes.length : 1;
    final sector = 2 * math.pi / n;
    final center = index * sector + sector / 2;
    final need = (2 * math.pi - center % (2 * math.pi)) % (2 * math.pi);
    final cur = _rotation % (2 * math.pi);
    var delta = need - cur;
    if (delta <= 0) delta += 2 * math.pi;
    return _rotation + 5 * 2 * math.pi + delta;
  }

  double get _displayRotation {
    if (!_spinning) return _rotation;
    final t = Curves.easeOutCubic.transform(_ctrl.value);
    return _spinFrom + (_spinTo - _spinFrom) * t;
  }

  Future<void> _spin(int count) async {
    if (_spinning) return;
    setState(() {
      _spinning = true;
      _mode = count;
      _result = null;
    });
    try {
      final res = count == 1
          ? await _api.play(widget.siteId, _game)
          : await _api.playMulti(widget.siteId, _game, count);
      if (res.success) {
        _spinFrom = _rotation;
        _spinTo = count == 1 ? _targetForIndex(_idToIndex(res.prizeId)) : _rotation + 3 * 2 * math.pi;
        _result = res; // 动画结束才展示
        _ctrl.forward(from: 0);
      } else {
        _result = res;
        _toast(res.message.isNotEmpty ? res.message : '抽奖失败');
        if (!mounted) return;
        setState(() => _spinning = false);
      }
    } catch (e) {
      _toast('请求失败：$e');
      if (mounted) setState(() => _spinning = false);
    }
  }

  Future<void> _onSpinEnd() async {
    _rotation = _spinTo;
    final res = _result;
    if (res != null && res.success) {
      if (_mode == 1) {
        final win = res.prize.isNotEmpty && res.prize != '未中奖';
        _toast(win ? '🎉 ${res.prize}' : (res.message.isNotEmpty ? res.message : '未中奖'));
      } else {
        final won = res.prizes.where((p) => p.isWin).length;
        _toast(won > 0 ? '$_mode 连抽中奖 $won 次' : '$_mode 连抽未中奖');
      }
    }
    await _refresh();
    if (mounted) setState(() => _spinning = false);
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _banner(),
        const SizedBox(height: 12),
        _statsBar(),
        const SizedBox(height: 14),
        _wheel(),
        const SizedBox(height: 14),
        _actions(),
        if (_result != null && !_spinning) ...[const SizedBox(height: 12), _resultView()],
        if (_records.isNotEmpty) ...[const SizedBox(height: 14), _recordsView()],
      ],
    );
  }

  Widget _banner() => Container(
        padding: const EdgeInsets.symmetric(vertical: 14),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF4D96FF), Color(0xFF9D65C9)]),
        ),
        child: Column(children: [
          const Text('🎰 幸运大转盘', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white, letterSpacing: 2)),
          const SizedBox(height: 3),
          Text('单次消耗 ${(_stats?.costPerPlay ?? 0).round()} 魔力${_stats != null && _stats!.discount > 0 ? ' · 连抽${_stats!.discount / 10}折' : ''}',
              style: const TextStyle(fontSize: 12, color: Colors.white70)),
        ]),
      );

  Widget _statsBar() => Row(children: [
        _statTile(_stats != null ? '${_stats!.balance.round()}' : '--', '魔力余额'),
        const SizedBox(width: 8),
        _statTile(_stats != null && _stats!.discount > 0 ? '${_stats!.discount / 10}' : '—', '连抽折扣'),
      ]);

  Widget _statTile(String value, String label) => Expanded(
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 10),
          decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(12)),
          child: Column(children: [
            Text(value, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: Color(0xFF4D96FF))),
            const SizedBox(height: 2),
            Text(label, style: TextStyle(fontSize: 11, color: context.dimText)),
          ]),
        ),
      );

  Widget _wheel() {
    final size = MediaQuery.of(context).size.width - 32;
    return Center(
      child: SizedBox(
        width: size,
        height: size,
        child: Stack(alignment: Alignment.center, children: [
          Transform.rotate(
            angle: _displayRotation,
            child: CustomPaint(size: Size(size, size), painter: _WheelPainter(_prizes)),
          ),
          // 顶部红指针
          Positioned(
            top: 0,
            child: CustomPaint(size: const Size(32, 30), painter: _PointerPainter()),
          ),
          // 中心按钮
          GestureDetector(
            onTap: _spinning ? null : () => _spin(1),
            child: Container(
              width: size * 0.26,
              height: size * 0.26,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: Colors.white,
                border: Border.all(color: const Color(0xFF4D96FF), width: 3),
              ),
              child: Center(
                child: _spinning
                    ? const SizedBox(width: 26, height: 26, child: CircularProgressIndicator(color: Color(0xFF4D96FF), strokeWidth: 3))
                    : Column(mainAxisSize: MainAxisSize.min, children: const [
                        Text('🎲', style: TextStyle(fontSize: 22)),
                        Text('开始\n抽奖', textAlign: TextAlign.center, style: TextStyle(color: Color(0xFF2563EB), fontWeight: FontWeight.w800, fontSize: 11, height: 1.2)),
                      ]),
              ),
            ),
          ),
        ]),
      ),
    );
  }

  Widget _actions() => Row(children: [
        Expanded(child: FilledButton(onPressed: _spinning ? null : () => _spin(1), child: const Text('单抽'))),
        const SizedBox(width: 10),
        Expanded(child: FilledButton(style: FilledButton.styleFrom(backgroundColor: Colors.orange), onPressed: _spinning ? null : () => _spin(_multi), child: Text('$_multi 连抽'))),
      ]);

  Widget _resultView() {
    final r = _result!;
    if (_mode == 1) {
      return Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: r.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Wrap(crossAxisAlignment: WrapCrossAlignment.center, spacing: 8, children: [
          Text(r.prize != '未中奖' ? '🎉 中奖' : '💫 未中奖', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13)),
          if (r.prize.isNotEmpty) Text(r.prize, style: const TextStyle(color: Color(0xFF4D96FF), fontWeight: FontWeight.w700, fontSize: 13)),
          if (r.amount > 0) Text('×${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Colors.orange, fontWeight: FontWeight.w700, fontSize: 13)),
        ]),
      );
    }
    // 连抽聚合
    final summary = <String, int>{};
    for (final p in r.prizes) {
      final k = p.prizeName.isNotEmpty ? p.prizeName : '未知';
      summary[k] = (summary[k] ?? 0) + 1;
    }
    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(color: PTColors.success.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(10)),
      child: Wrap(spacing: 8, runSpacing: 6, children: [
        const Text('$_multi 连抽结果', style: TextStyle(fontWeight: FontWeight.w800, fontSize: 13)),
        ...summary.entries.map((e) => Container(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 3),
              decoration: BoxDecoration(color: Colors.orange.withValues(alpha: 0.2), borderRadius: BorderRadius.circular(8)),
              child: Text('${e.key} ×${e.value}', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13)),
            )),
      ]),
    );
  }

  Widget _recordsView() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('抽奖记录', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          ..._records.take(8).map((r) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  Text(r.prizeName, style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13, color: r.prizeName == '未中奖' ? context.dimText : null)),
                  if (r.amount > 0) Padding(padding: const EdgeInsets.only(left: 6), child: Text('×${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Colors.orange, fontSize: 13))),
                  const Spacer(),
                  Text(r.createdAt, style: TextStyle(fontSize: 11, color: context.dimText)),
                ]),
              )),
        ]),
      );
}

// 转盘扇区绘制（静态；旋转靠外层 Transform.rotate）。
class _WheelPainter extends CustomPainter {
  final List<PrizeDef> prizes;
  _WheelPainter(this.prizes);

  @override
  void paint(Canvas canvas, Size size) {
    final cx = size.width / 2;
    final cy = size.height / 2;
    final R = size.width / 2 - 6;
    final n = prizes.isNotEmpty ? prizes.length : 1;
    final sector = 2 * math.pi / n;
    final paint = Paint()..style = PaintingStyle.fill;
    for (var i = 0; i < n; i++) {
      final a0 = -math.pi / 2 + i * sector;
      final a1 = a0 + sector;
      paint.color = Color(_wheelColors[i % _wheelColors.length]);
      final path = Path()
        ..moveTo(cx, cy)
        ..arcTo(Rect.fromCircle(center: Offset(cx, cy), radius: R), a0, a1 - a0, false)
        ..close();
      canvas.drawPath(path, paint);
      // 文字沿径向
      canvas.save();
      canvas.translate(cx, cy);
      canvas.rotate(a0 + sector / 2);
      final tp = TextPainter(
        text: TextSpan(text: prizes[i].name, style: const TextStyle(color: Color(0xFF3A2A1A), fontSize: 11, fontWeight: FontWeight.bold)),
        textDirection: TextDirection.ltr,
        maxLines: 1,
        ellipsis: '…',
      )..layout(maxWidth: R - 16);
      tp.paint(canvas, Offset(R - 14 - tp.width, -tp.height / 2));
      canvas.restore();
    }
    // 外金边
    paint
      ..style = PaintingStyle.stroke
      ..strokeWidth = 6
      ..color = const Color(0xFFFFD93D);
    canvas.drawCircle(Offset(cx, cy), R, paint);
  }

  @override
  bool shouldRepaint(covariant _WheelPainter old) => old.prizes != prizes;
}

// 顶部红色三角指针。
class _PointerPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..color = const Color(0xFFFF3B3B);
    final path = Path()
      ..moveTo(size.width / 2, size.height)
      ..lineTo(0, 0)
      ..lineTo(size.width, 0)
      ..close();
    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
