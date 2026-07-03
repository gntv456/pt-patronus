import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// NovaHD 幸运抽奖：抽奖信息（余额/消耗/今日）+ 奖品列表 + 立即抽奖/连抽 10·20·50 + 记录。
// 后端 fortune-wheel-spin.php：POST count → JSON {success, results:[{status,type,value,unit}]}。
class NovaHDLuckyView extends StatefulWidget {
  final int siteId;
  const NovaHDLuckyView({super.key, required this.siteId});
  @override
  State<NovaHDLuckyView> createState() => _NovaHDLuckyViewState();
}

class _NovaHDLuckyViewState extends State<NovaHDLuckyView> {
  static const _game = 'novahd_lucky';
  final _api = EntertainmentApi();
  GameStats? _stats;
  List<PrizeDef> _prizes = [];
  List<GameRecord> _records = [];
  bool _spinning = false;
  PlayResult? _last;

  @override
  void initState() {
    super.initState();
    _load();
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

  String _emoji(String name) {
    if (RegExp(r'VIP|会员').hasMatch(name)) return '👑';
    if (name.contains('彩虹')) return '🌈';
    if (name.contains('邀请')) return '✉️';
    if (name.contains('魔力')) return '🪙';
    return '🎁';
  }

  Future<void> _draw(int count) async {
    if (_spinning) return;
    setState(() => _spinning = true);
    try {
      final res = count == 1 ? await _api.play(widget.siteId, _game) : await _api.playMulti(widget.siteId, _game, count);
      _last = res;
      if (res.success) {
        final wins = res.prizes.where((p) => p.isWin).length;
        _toast(wins > 0 ? '🎉 中奖 $wins 次' : (res.message.isNotEmpty ? res.message : '完成'));
      } else {
        _toast(res.message.isNotEmpty ? res.message : '抽奖失败');
      }
      await _refresh();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _spinning = false);
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
        _infoSection(),
        if (_prizes.isNotEmpty) ...[const SizedBox(height: 12), _prizesSection()],
        const SizedBox(height: 12),
        _opsSection(),
        if (_last != null) ...[const SizedBox(height: 12), _resultView()],
        if (_records.isNotEmpty) ...[const SizedBox(height: 12), _recordsView()],
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF7C3AED), Color(0xFF2563EB)]),
        ),
        child: const Center(child: Text('🎡 幸运抽奖', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white, letterSpacing: 1))),
      );

  Widget _infoSection() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Row(children: [
          _infoCell(_stats != null ? _stats!.balance.toStringAsFixed(0) : '—', '当前魔力'),
          _infoCell(_stats != null ? _stats!.costPerPlay.toStringAsFixed(0) : '—', '每次消耗'),
          _infoCell('${_stats?.todaySpins ?? 0}/${_stats?.maxDaily ?? '?'}', '今日已抽'),
        ]),
      );

  Widget _infoCell(String value, String label) => Expanded(
        child: Column(children: [
          Text(value, style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w800, color: Color(0xFF2563EB))),
          const SizedBox(height: 2),
          Text(label, style: const TextStyle(fontSize: 11, color: Colors.grey)),
        ]),
      );

  Widget _prizesSection() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('奖品列表', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 10),
          SizedBox(
            height: 78,
            child: ListView.separated(
              scrollDirection: Axis.horizontal,
              itemCount: _prizes.length,
              separatorBuilder: (_, _) => const SizedBox(width: 8),
              itemBuilder: (_, i) {
                final p = _prizes[i];
                return Container(
                  width: 72,
                  decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(10)),
                  child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
                    Text(_emoji(p.name), style: const TextStyle(fontSize: 22)),
                    const SizedBox(height: 2),
                    Text(p.name, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600), maxLines: 2, overflow: TextOverflow.ellipsis, textAlign: TextAlign.center),
                  ]),
                );
              },
            ),
          ),
        ]),
      );

  Widget _opsSection() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Wrap(
          alignment: WrapAlignment.center,
          spacing: 8,
          runSpacing: 8,
          children: [
            _opBtn('立即抽奖', () => _draw(1), primary: true),
            _opBtn('连抽 10 次', () => _draw(10)),
            _opBtn('连抽 20 次', () => _draw(20)),
            _opBtn('连抽 50 次', () => _draw(50)),
          ],
        ),
      );

  Widget _opBtn(String label, VoidCallback onTap, {bool primary = false}) {
    return OutlinedButton(
      onPressed: _spinning ? null : onTap,
      style: OutlinedButton.styleFrom(
        foregroundColor: const Color(0xFF2563EB),
        backgroundColor: primary ? const Color(0xFF2563EB) : null,
        side: const BorderSide(color: Color(0xFF2563EB)),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 9),
      ),
      child: Text(label, style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: primary ? Colors.white : const Color(0xFF2563EB))),
    );
  }

  Widget _resultView() => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: _last!.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Text(_last!.success ? '🎉' : '✗', style: const TextStyle(fontSize: 16)),
            const SizedBox(width: 8),
            Text(_last!.success ? '抽奖结果' : '失败', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13)),
          ]),
          if (_last!.prizes.isNotEmpty) ...[
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: (_last!.success ? _last!.prizes.where((p) => p.isWin).toList() : _last!.prizes).take(10).map((p) => Container(
                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 3),
                    decoration: BoxDecoration(color: const Color(0x2922C55E), borderRadius: BorderRadius.circular(8)),
                    child: Text('${p.prizeName}${p.amount > 0 ? ' ×${p.amount.toStringAsFixed(0)}' : ''}',
                        style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: Color(0xFF16A34A))),
                  )).toList(),
            ),
            if (_last!.success && !_last!.prizes.any((p) => p.isWin)) const Padding(padding: EdgeInsets.only(top: 4), child: Text('很遗憾，未中奖', style: TextStyle(color: Colors.grey, fontSize: 12))),
          ] else if (_last!.message.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(_last!.message, style: const TextStyle(fontSize: 13)),
          ],
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
                  if (r.amount > 0) Padding(padding: const EdgeInsets.only(left: 6), child: Text('+${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Color(0xFF22C55E), fontSize: 13))),
                  const Spacer(),
                  Text(r.createdAt, style: const TextStyle(fontSize: 11, color: Colors.grey)),
                ]),
              )),
        ]),
      );
}
