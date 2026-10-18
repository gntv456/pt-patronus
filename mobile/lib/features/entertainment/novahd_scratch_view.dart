import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'ent_read_fail.dart';

// NovaHD 魔力值刮刮乐：余额 + 购买刮奖资格（消耗 5000） + 6 档规则 + 记录。
// 后端 magic_scratch.php：buy(POST→HTML 抠 id) → scratch(POST→JSON) → settle。
class NovaHDScratchView extends StatefulWidget {
  final int siteId;
  const NovaHDScratchView({super.key, required this.siteId});
  @override
  State<NovaHDScratchView> createState() => _NovaHDScratchViewState();
}

class _NovaHDScratchViewState extends State<NovaHDScratchView> {
  static const _game = 'novahd_scratch';
  final _api = EntertainmentApi();
  GameStats? _stats;
  List<PrizeDef> _prizes = [];
  List<GameRecord> _records = [];
  bool _scratching = false;
  PlayResult? _last;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      _prizes = await _api.prizes(widget.siteId, _game);
    } catch (e) {
      if (mounted) entReadFailed(context, 'prizes', e);
    }
    await _refresh();
    if (mounted) setState(() {});
  }

  Future<void> _refresh() async {
    try {
      _stats = await _api.stats(widget.siteId, _game);
    } catch (e) {
      if (mounted) entReadFailed(context, 'stats', e);
    }
    try {
      _records = await _api.history(widget.siteId, _game);
    } catch (e) {
      if (mounted) entReadFailed(context, 'history', e);
    }
  }

  int get _remaining => _stats == null ? 0 : (_stats!.maxDaily - _stats!.todaySpins).clamp(0, _stats!.maxDaily);

  Future<void> _scratch() async {
    if (_scratching) return;
    setState(() => _scratching = true);
    try {
      final res = await _api.play(widget.siteId, _game);
      _last = res;
      _toast(res.success ? (res.prize.isNotEmpty ? '${res.prize}：+${res.amount.toStringAsFixed(0)} 魔力值' : '完成') : (res.message.isNotEmpty ? res.message : '刮奖失败'));
      await _refresh();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _scratching = false);
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
        _buySection(),
        if (_last != null) ...[const SizedBox(height: 12), _resultView()],
        const SizedBox(height: 12),
        _rulesSection(),
        if (_records.isNotEmpty) ...[const SizedBox(height: 12), _recordsView()],
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFFE67E22), Color(0xFFF59E0B)]),
        ),
        child: Column(children: [
          const Text('🎫 魔力值刮刮乐', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text('余额 ${_stats != null ? _stats!.balance.toStringAsFixed(1) : '—'} 魔力值',
              style: const TextStyle(fontSize: 13, color: Colors.white)),
        ]),
      );

  Widget _buySection() => GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(children: [
          const Text('刮奖中心', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 6),
          Text('购买资格消耗 5000 魔力值 · 今日剩余 $_remaining/${_stats?.maxDaily ?? 5}',
              style: TextStyle(fontSize: 12, color: context.dimText)),
          const SizedBox(height: 14),
          FilledButton(
            onPressed: _scratching ? null : _scratch,
            style: FilledButton.styleFrom(
              padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
              backgroundColor: const Color(0xFF16A34A),
            ),
            child: _scratching
                ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(color: Colors.white, strokeWidth: 2))
                : const Text('购买刮奖资格（消耗 5000）'),
          ),
        ]),
      );

  Widget _rulesSection() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('中奖规则', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 10),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: _prizes.map((p) {
              final miss = p.type == '99';
              return Container(
                width: (MediaQuery.of(context).size.width - 60) / 3,
                padding: const EdgeInsets.symmetric(vertical: 10),
                decoration: BoxDecoration(
                  color: miss ? const Color(0x1E94A3B8) : const Color(0x1EF59E0B),
                  borderRadius: BorderRadius.circular(10),
                ),
                child: Text(p.name, textAlign: TextAlign.center,
                    style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: miss ? context.dimText : const Color(0xFFB45309))),
              );
            }).toList(),
          ),
        ]),
      );

  Widget _resultView() => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: _last!.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Row(children: [
          Text(_last!.success ? '🎉' : '✗', style: const TextStyle(fontSize: 16)),
          const SizedBox(width: 8),
          Expanded(
            child: Text(_last!.success ? '${_last!.prize} · 获得 ${_last!.amount.toStringAsFixed(0)} 魔力值' : _last!.message,
                style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
          ),
        ]),
      );

  Widget _recordsView() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('我的刮奖记录', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          ..._records.take(10).map((r) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  Text(r.prizeName, style: TextStyle(fontSize: 13, color: r.prizeName == '未中奖' ? context.dimText : null, fontWeight: FontWeight.w600)),
                  if (r.amount > 0) Padding(padding: const EdgeInsets.only(left: 6), child: Text('+${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Color(0xFF22C55E), fontSize: 13))),
                  const Spacer(),
                  Text(r.createdAt, style: TextStyle(fontSize: 11, color: context.dimText)),
                ]),
              )),
        ]),
      );
}
