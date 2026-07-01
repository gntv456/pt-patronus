import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// 通用抽奖 View（批1）：覆盖多站幸运转盘/老虎机/抽奖。
// 关键：永远画出可见转盘——有 prizes 画真扇区，没有画 8 格装饰扇区。
class SpinnerGameView extends StatefulWidget {
  final int siteId;
  final String game;
  final String name;
  const SpinnerGameView({super.key, required this.siteId, required this.game, required this.name});
  @override
  State<SpinnerGameView> createState() => _SpinnerGameViewState();
}

class _SpinnerGameViewState extends State<SpinnerGameView>
    with SingleTickerProviderStateMixin {
  final _api = EntertainmentApi();
  GameStats? _stats;
  List<PrizeDef> _prizes = [];
  List<GameRecord> _records = [];
  bool _playing = false;
  PlayResult? _last;

  late final AnimationController _ctrl;
  double _baseTurns = 0; // 累计旋转圈数（叠加，避免每次回零）

  static const _colors = [
    Color(0xFFFF6B6B), Color(0xFFFFD93D), Color(0xFF6BCB77), Color(0xFF4D96FF),
    Color(0xFF9D65C9), Color(0xFFFF9F43), Color(0xFF1ABC9C), Color(0xFFE84393),
    Color(0xFFFDCB6E), Color(0xFF74B9FF), Color(0xFFA29BFE), Color(0xFF55EFC4),
  ];
  // 无奖品数据时的装饰扇区
  static const _decor = ['🎁', '🪙', '✨', '🎫', '🍀', '💎', '👑', '🎊'];

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(vsync: this, duration: const Duration(seconds: 4));
    _load();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  List<String> get _segments =>
      _prizes.isNotEmpty ? _prizes.map((p) => p.name).toList() : _decor.toList();

  // 连抽结果按奖品名分组计数（对齐躺平站点「88 魔力 ×12」展示）
  List<({String name, int n, bool win})> get _grouped {
    final order = <String>[];
    final map = <String, ({String name, int n, bool win})>{};
    for (final p in _last?.prizes ?? <PrizeOutcome>[]) {
      final key = p.prizeName.isEmpty ? '未知' : p.prizeName;
      final e = map[key] ?? (name: key, n: 0, win: false);
      final ne = (name: key, n: e.n + 1, win: e.win || p.isWin);
      map[key] = ne;
      if (!order.contains(key)) order.add(key);
    }
    final list = order.map((k) => map[k]!).toList();
    list.sort((a, b) => (b.win ? 1 : 0) - (a.win ? 1 : 0));
    return list;
  }

  // hero 余额：stats 缺失时用最近一次结果余额兜底
  String get _heroBalance {
    final b = _stats?.balance ?? _last?.balance ?? 0;
    return b > 0 ? b.toStringAsFixed(0) : '—';
  }

  Future<void> _load() async {
    try {
      _prizes = await _api.prizes(widget.siteId, widget.game);
    } catch (_) {}
    await _refresh();
    if (mounted) setState(() {});
  }

  Future<void> _refresh() async {
    try {
      _stats = await _api.stats(widget.siteId, widget.game);
    } catch (_) {}
    try {
      _records = await _api.history(widget.siteId, widget.game);
    } catch (_) {}
  }

  Future<void> _draw(int count) async {
    if (_playing) return;
    setState(() => _playing = true);
    _ctrl.forward(from: 0); // 装饰旋转动画
    try {
      final res = count == 1
          ? await _api.play(widget.siteId, widget.game)
          : await _api.playMulti(widget.siteId, widget.game, count);
      _last = res;
      // 累计 5-7 圈（伪随机靠 count 与时间戳，避免 Random）
      _baseTurns += 5 + (count % 3);
      if (res.success) {
        final g = _grouped;
        if (g.length > 1 || (g.length == 1 && g.first.n > 1)) {
          final summary = g.map((e) => '${e.name} ×${e.n}').join('，');
          _toast(summary);
        } else {
          _toast(res.prize.isNotEmpty ? '🎉 ${res.prize}' : '完成');
        }
      } else {
        _toast(res.message.isNotEmpty ? res.message : '抽奖失败');
      }
      await _refresh();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _playing = false);
    }
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
        _hero(),
        const SizedBox(height: 12),
        _wheel(),
        const SizedBox(height: 12),
        _ops(),
        if (_last != null) ...[const SizedBox(height: 12), _resultView()],
        if (_prizes.isNotEmpty) ...[const SizedBox(height: 12), _prizesSection()],
        if (_records.isNotEmpty) ...[const SizedBox(height: 12), _recordsView()],
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF6366F1), Color(0xFF8B5CF6)]),
        ),
        child: Column(children: [
          Text('🎰 ${widget.name}', style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text('余额 $_heroBalance', style: const TextStyle(fontSize: 13, color: Colors.white)),
        ]),
      );

  Widget _wheel() {
    final turns = _baseTurns + (_playing ? 5 : 0);
    return Center(
      child: SizedBox(
        width: 300,
        height: 300,
        child: Stack(alignment: Alignment.center, children: [
          // 指针
          Positioned(
            top: 0,
            child: CustomPaint(size: const Size(32, 30), painter: _PointerPainter()),
          ),
          // 转盘（AnimatedBuilder 驱动旋转；静态时停在累计角度）
          AnimatedBuilder(
            animation: _ctrl,
            builder: (context, child) {
              final t = _playing ? _ctrl.value : 0;
              // easeOut 让减速更自然
              final eased = _playing ? (1 - math.pow(1 - t, 3)) : 0.0;
              return Transform.rotate(angle: (turns + eased * 2) * 2 * math.pi, child: child);
            },
            child: CustomPaint(size: const Size(280, 280), painter: _WheelPainter(_segments, _colors)),
          ),
          // 中心 hub 按钮
          GestureDetector(
            onTap: _playing ? null : () => _draw(1),
            child: Container(
              width: 76, height: 76, alignment: Alignment.center,
              decoration: BoxDecoration(shape: BoxShape.circle, color: const Color(0xFF6366F1),
                border: Border.all(color: Colors.white, width: 4),
                boxShadow: const [BoxShadow(color: Color(0x4D6366F1), blurRadius: 14, spreadRadius: 2)]),
              child: _playing
                  ? const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(color: Colors.white, strokeWidth: 3))
                  : Column(mainAxisAlignment: MainAxisAlignment.center, children: const [
                      Text('🎲', style: TextStyle(fontSize: 22)),
                      Text('开始\n抽奖', style: TextStyle(color: Colors.white, fontSize: 11, fontWeight: FontWeight.w700, height: 1.1)),
                    ]),
            ),
          ),
        ]),
      ),
    );
  }

  Widget _ops() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Wrap(
          alignment: WrapAlignment.center,
          spacing: 8,
          runSpacing: 8,
          children: [
            _btn('立即抽奖', () => _draw(1), primary: true),
            _btn('连抽 10 次', () => _draw(10)),
            _btn('连抽 20 次', () => _draw(20)),
            _btn('连抽 50 次', () => _draw(50)),
          ],
        ),
      );

  Widget _btn(String label, VoidCallback onTap, {bool primary = false}) => OutlinedButton(
        onPressed: _playing ? null : onTap,
        style: OutlinedButton.styleFrom(
          foregroundColor: const Color(0xFF6366F1),
          backgroundColor: primary ? const Color(0xFF6366F1) : null,
          side: const BorderSide(color: Color(0xFF6366F1)),
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 9),
        ),
        child: Text(label, style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: primary ? Colors.white : const Color(0xFF6366F1))),
      );

  Widget _resultView() => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: _last!.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(_last!.success ? '🎉' : '✗', style: const TextStyle(fontSize: 16)),
          const SizedBox(width: 8),
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            if (_last!.prizes.isNotEmpty)
              Wrap(spacing: 6, runSpacing: 4, children: _grouped.map((g) => Container(
                    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                    decoration: BoxDecoration(
                      color: g.win ? PTColors.success.withValues(alpha: 0.18) : Colors.white.withValues(alpha: 0.08),
                      borderRadius: BorderRadius.circular(8)),
                    child: Text('${g.name} ×${g.n}',
                        style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600,
                            color: g.win ? const Color(0xFF16A34A) : Colors.grey)),
                  )).toList())
            else if (_last!.prize.isNotEmpty)
                Text(_last!.prize, style: const TextStyle(fontWeight: FontWeight.w700, color: Color(0xFF16A34A))),
            if (_last!.balance > 0) Text('余额 ${_last!.balance.toStringAsFixed(0)}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
            if (_last!.message.isNotEmpty) Text(_last!.message, style: const TextStyle(fontSize: 12, color: Colors.grey)),
          ])),
        ]),
      );

  Widget _prizesSection() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('奖品', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          Wrap(spacing: 8, runSpacing: 8, children: _prizes.map((p) => Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(10)),
            child: Text(p.name, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
          )).toList()),
        ]),
      );

  Widget _recordsView() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('抽奖记录', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          ..._records.take(10).map((r) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  Text(r.prizeName, style: TextStyle(fontSize: 13, color: r.prizeName == '未中奖' ? Colors.grey : null, fontWeight: FontWeight.w600)),
                  if (r.amount > 0) Padding(padding: const EdgeInsets.only(left: 6), child: Text('×${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Color(0xFF22C55E), fontSize: 13))),
                  const Spacer(),
                  Text(r.createdAt, style: const TextStyle(fontSize: 11, color: Colors.grey)),
                ]),
              )),
        ]),
      );
}

// 转盘扇区绘制：N 段彩色扇区 + 文字（奖品名或装饰 emoji）。
class _WheelPainter extends CustomPainter {
  final List<String> segments;
  final List<Color> colors;
  _WheelPainter(this.segments, this.colors);

  @override
  void paint(Canvas canvas, Size size) {
    final cx = size.width / 2, cy = size.height / 2;
    final R = math.min(cx, cy) - 6;
    final n = segments.isEmpty ? 1 : segments.length;
    final sector = 2 * math.pi / n;
    for (var i = 0; i < n; i++) {
      final a0 = -math.pi / 2 + i * sector;
      final paint = Paint()..color = colors[i % colors.length]..style = PaintingStyle.fill;
      final path = Path()
        ..moveTo(cx, cy)
        ..arcTo(Rect.fromCircle(center: Offset(cx, cy), radius: R), a0, sector, false)
        ..close();
      canvas.drawPath(path, paint);
      canvas.drawPath(path, Paint()..color = Colors.white.withValues(alpha: 0.5)..style = PaintingStyle.stroke..strokeWidth = 1);
      // 文字
      final tp = TextPainter(
        text: TextSpan(text: segments.isEmpty ? '' : segments[i], style: TextStyle(
          color: const Color(0xFF3A2A1A),
          fontSize: segments.length <= 8 ? 22 : 13,
          fontWeight: FontWeight.bold,
        )),
        textDirection: TextDirection.ltr,
        maxLines: 1,
        ellipsis: '…',
      )..layout();
      canvas.save();
      canvas.translate(cx, cy);
      canvas.rotate(a0 + sector / 2);
      // 文字沿径向：右对齐到接近边缘
      tp.paint(canvas, Offset(R - tp.width - 10, -tp.height / 2 + (segments.length <= 8 ? 4 : 0)));
      canvas.restore();
    }
    // 外金边
    canvas.drawCircle(Offset(cx, cy), R, Paint()..color = const Color(0xFFFFD93D)..style = PaintingStyle.stroke..strokeWidth = 6);
  }

  @override
  bool shouldRepaint(covariant _WheelPainter old) => old.segments != segments;
}

// 顶部指针（红色三角）。
class _PointerPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final path = Path()
      ..moveTo(size.width / 2, size.height)
      ..lineTo(0, 0)
      ..lineTo(size.width, 0)
      ..close();
    canvas.drawPath(path, Paint()..color = const Color(0xFFFF3B3B));
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
