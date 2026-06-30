import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// 通用抽奖 View（批1）：覆盖多站幸运转盘/老虎机/抽奖。按 game 调 play/playMulti/stats/prizes/history。
class SpinnerGameView extends StatefulWidget {
  final int siteId;
  final String game;
  final String name;
  const SpinnerGameView({super.key, required this.siteId, required this.game, required this.name});
  @override
  State<SpinnerGameView> createState() => _SpinnerGameViewState();
}

class _SpinnerGameViewState extends State<SpinnerGameView> {
  final _api = EntertainmentApi();
  GameStats? _stats;
  List<PrizeDef> _prizes = [];
  List<GameRecord> _records = [];
  bool _playing = false;
  PlayResult? _last;

  @override
  void initState() {
    super.initState();
    _load();
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
    try {
      final res = count == 1
          ? await _api.play(widget.siteId, widget.game)
          : await _api.playMulti(widget.siteId, widget.game, count);
      _last = res;
      if (res.success) {
        final wins = res.prizes.where((p) => p.isWin).length;
        _toast(wins > 0 ? '🎉 中奖 $wins 次' : (res.prize.isNotEmpty ? res.prize : '完成'));
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
          Text('余额 ${_stats != null ? _stats!.balance.toStringAsFixed(0) : '—'}',
              style: const TextStyle(fontSize: 13, color: Colors.white)),
        ]),
      );

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
            if (_last!.prize.isNotEmpty) Text(_last!.prize, style: const TextStyle(fontWeight: FontWeight.w700, color: Color(0xFF16A34A))),
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
