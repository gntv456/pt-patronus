import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// 通用下注 View（批2）：按 game 后缀渲染玩法（押大小/双色球选号/翻面/红包/赛事押注）。
class BetGameView extends StatefulWidget {
  final int siteId;
  final String game;
  final String name;
  const BetGameView({super.key, required this.siteId, required this.game, required this.name});
  @override
  State<BetGameView> createState() => _BetGameViewState();
}

class _BetGameViewState extends State<BetGameView> {
  final _api = EntertainmentApi();
  GameStats? _stats;
  bool _playing = false;
  PlayResult? _last;
  double _amount = 100;
  final _reds = TextEditingController(text: '1,2,3,4,5,6');
  final _blue = TextEditingController(text: '7');

  String get _mode {
    final g = widget.game;
    if (g.endsWith('_bigsmall') || g.endsWith('_sicbo')) return 'bigsmall';
    if (g.endsWith('_unionlotto')) return 'lotto';
    if (g.endsWith('_flip')) return 'flip';
    if (g.endsWith('_redpacket')) return 'redpacket';
    if (g.endsWith('_bet')) return 'bet';
    return 'generic';
  }

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    try {
      _stats = await _api.stats(widget.siteId, widget.game);
    } catch (_) {}
    if (mounted) setState(() {});
  }

  Future<void> _doBet(BetRequest req) async {
    if (_playing) return;
    setState(() => _playing = true);
    try {
      final res = await _api.bet(widget.siteId, widget.game, req);
      _last = res;
      _toast(res.success
          ? (res.prize.isNotEmpty ? res.prize : (res.message.isNotEmpty ? res.message : '完成'))
          : (res.message.isNotEmpty ? res.message : '操作失败'));
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
        ..._bodyByMode(),
        if (_last != null) ...[const SizedBox(height: 12), _resultView()],
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFFDB2777), Color(0xFFF59E0B)]),
        ),
        child: Column(children: [
          Text('🎲 ${widget.name}', style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text('余额 ${_stats != null ? _stats!.balance.toStringAsFixed(0) : '—'}', style: const TextStyle(fontSize: 13, color: Colors.white)),
        ]),
      );

  List<Widget> _bodyByMode() {
    switch (_mode) {
      case 'bigsmall':
        return [
          GlassCard(
            padding: const EdgeInsets.all(16),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Text('押注金额', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              const SizedBox(height: 8),
              SizedBox(width: 180, child: TextField(
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(hintText: '金额', isDense: true),
                onChanged: (v) => _amount = double.tryParse(v) ?? 100,
              )),
              const SizedBox(height: 14),
              Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                _opBtn('押 大', const Color(0xFFDC2626), () => _doBet(BetRequest(selection: 'big', amount: _amount))),
                const SizedBox(width: 12),
                _opBtn('押 小', const Color(0xFF2563EB), () => _doBet(BetRequest(selection: 'small', amount: _amount))),
              ]),
            ]),
          )
        ];
      case 'lotto':
        return [
          GlassCard(
            padding: const EdgeInsets.all(16),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Text('选号（6 红 1 蓝，每注 166）', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              const SizedBox(height: 8),
              TextField(controller: _reds, decoration: const InputDecoration(labelText: '红球 (逗号分隔)')),
              const SizedBox(height: 8),
              TextField(controller: _blue, decoration: const InputDecoration(labelText: '蓝球')),
              const SizedBox(height: 14),
              Center(child: _opBtn('投注', const Color(0xFFDB2777), () => _doBet(BetRequest(selection: '', amount: 166, extra: {'reds': _reds.text, 'blue': _blue.text})))),
            ]),
          )
        ];
      case 'flip':
        return [GlassCard(padding: const EdgeInsets.all(24), child: Center(child: _opBtn('🃏 翻一张', const Color(0xFFDB2777), () => _doBet(BetRequest(selection: '0', amount: 0)))))];
      case 'redpacket':
        return [GlassCard(padding: const EdgeInsets.all(24), child: Center(child: _opBtn('🧧 领取红包', const Color(0xFFDB2777), () => _doBet(BetRequest(selection: 'claim', amount: 0)))))];
      case 'bet':
        return [GlassCard(padding: const EdgeInsets.all(20), child: const Text('押注表单待补抓 bet_odds.php，暂未支持自动押注。', style: TextStyle(color: Colors.grey, fontSize: 13)))];
      default:
        return [GlassCard(padding: const EdgeInsets.all(20), child: Center(child: _opBtn('操作', const Color(0xFFDB2777), () => _doBet(BetRequest(selection: '', amount: _amount)))))];
    }
  }

  Widget _opBtn(String label, Color color, VoidCallback onTap) => FilledButton(
        onPressed: _playing ? null : onTap,
        style: FilledButton.styleFrom(backgroundColor: color, padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 12)),
        child: Text(label, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
      );

  Widget _resultView() => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: _last!.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(_last!.success ? '✓' : '✗', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13)),
          const SizedBox(width: 8),
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            if (_last!.prize.isNotEmpty) Text(_last!.prize, style: const TextStyle(fontWeight: FontWeight.w700, color: Color(0xFF16A34A))),
            if (_last!.balance > 0) Text('余额 ${_last!.balance.toStringAsFixed(0)}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
            if (_last!.message.isNotEmpty) Text(_last!.message, style: const TextStyle(fontSize: 12, color: Colors.grey)),
          ])),
        ]),
      );
}
