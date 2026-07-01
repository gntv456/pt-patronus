import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// OurBits 赛事押注（菠菜）：列表 → 盘口（选项+赔率，按赔率升序热门在前）→ 下单 → 结果。
class EventBetView extends StatefulWidget {
  final int siteId;
  const EventBetView({super.key, required this.siteId});
  @override
  State<EventBetView> createState() => _EventBetViewState();
}

class _EventBetViewState extends State<EventBetView> {
  final _api = EntertainmentApi();
  static const _game = 'ourbits_bet';
  double? _balance;
  List<BetEvent> _events = [];
  BetEventDetail? _detail; // 非 null = 详情视图
  bool _loading = false;
  bool _detailLoading = false;
  bool _playing = false;
  String _selectedOpt = '';
  double _amount = 100;
  PlayResult? _last;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      final results = await Future.wait([
        _api.events(widget.siteId, _game).catchError((_) => <BetEvent>[]),
        _api.stats(widget.siteId, _game).catchError((_) => GameStats(balance: 0, cardNumber: 0, todaySpins: 0, maxDaily: 0, costPerPlay: 0)),
      ]);
      _events = results[0] as List<BetEvent>;
      _balance = (results[1] as GameStats).balance;
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _openEvent(BetEvent ev) async {
    setState(() {
      _detail = null;
      _detailLoading = true;
      _selectedOpt = '';
      _last = null;
    });
    try {
      _detail = await _api.eventDetail(widget.siteId, _game, ev.id);
    } catch (_) {
      _toast('加载盘口失败');
    } finally {
      if (mounted) setState(() => _detailLoading = false);
    }
  }

  void _back() {
    setState(() {
      _detail = null;
      _selectedOpt = '';
      _last = null;
    });
  }

  Future<void> _doBet() async {
    if (_selectedOpt.isEmpty || _amount <= 0) {
      _toast('请选择选项并输入押注金额');
      return;
    }
    if (_playing) return;
    setState(() => _playing = true);
    try {
      final res = await _api.bet(widget.siteId, _game, BetRequest(selection: _selectedOpt, amount: _amount));
      _last = res;
      _toast(res.success
          ? (res.message.isNotEmpty ? res.message : '押注成功')
          : (res.message.isNotEmpty ? res.message : '押注失败'));
      if (res.success && res.balance > 0) _balance = res.balance;
      await _refresh();
      if (_detail != null) {
        final d = await _api.eventDetail(widget.siteId, _game, _detail!.event.id).catchError((_) => _detail!);
        if (mounted) setState(() => _detail = d);
      }
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

  String _fmt(double n) => n.floor().toString();

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _hero(),
        const SizedBox(height: 12),
        if (_detail != null) ..._detailView() else ..._listView(),
        if (_last != null) ...[const SizedBox(height: 12), _resultView()],
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF059669), Color(0xFF0EA5E9)]),
        ),
        child: Column(children: [
          const Text('⚽ 赛事押注', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text('余额 ${_balance != null ? _fmt(_balance!) : '—'} 魔力', style: const TextStyle(fontSize: 13, color: Colors.white)),
        ]),
      );

  List<Widget> _listView() {
    if (_loading && _events.isEmpty) {
      return const [Center(child: Padding(padding: EdgeInsets.all(24), child: CircularProgressIndicator()))];
    }
    if (_events.isEmpty) {
      return [
        GlassCard(padding: const EdgeInsets.all(36), child: const Center(child: Text('暂无进行中的竞猜', style: TextStyle(color: Colors.grey))))
      ];
    }
    return _events.map(_eventCard).toList();
  }

  Widget _eventCard(BetEvent ev) => Padding(
        padding: const EdgeInsets.only(bottom: 10),
        child: GlassCard(
          padding: const EdgeInsets.all(14),
          child: InkWell(
            onTap: () => _openEvent(ev),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
                Expanded(child: Text(ev.title, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16))),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                  decoration: BoxDecoration(color: const Color(0x29059F69), borderRadius: BorderRadius.circular(8)),
                  child: Text(ev.status == 'open' ? '进行中' : ev.status,
                      style: const TextStyle(fontSize: 12, color: Color(0xFF059669), fontWeight: FontWeight.w600)),
                ),
              ]),
              const SizedBox(height: 6),
              Wrap(spacing: 14, runSpacing: 4, children: [
                Text('截止 ${ev.endTime}', style: const TextStyle(fontSize: 13, color: Colors.grey)),
                if (ev.remain.isNotEmpty)
                  Text('剩余 ${ev.remain}', style: const TextStyle(fontSize: 13, color: Color(0xFFF59E0B), fontWeight: FontWeight.w600)),
              ]),
              const SizedBox(height: 4),
              Wrap(spacing: 12, runSpacing: 4, children: [
                Text('庄家 ${ev.bookmaker.isEmpty ? '—' : ev.bookmaker}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
                Text('${ev.betCount} 人下注', style: const TextStyle(fontSize: 12, color: Colors.grey)),
                Text('投注 ${_fmt(ev.totalStake)} 魔力', style: const TextStyle(fontSize: 12, color: Colors.grey)),
              ]),
              const SizedBox(height: 4),
              const Text('查看盘口 ›', style: TextStyle(fontSize: 12, color: Color(0xFF6C5CE7), fontWeight: FontWeight.w600)),
            ]),
          ),
        ),
      );

  List<Widget> _detailView() {
    final d = _detail!;
    final options = [...d.options]..sort((a, b) => a.odds.compareTo(b.odds));
    return [
      GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
            Expanded(child: Text(d.event.title, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 17))),
            TextButton(onPressed: _back, child: const Text('‹ 返回列表')),
          ]),
          Wrap(spacing: 14, children: [
            Text('截止 ${d.event.endTime}', style: const TextStyle(fontSize: 13, color: Colors.grey)),
            if (d.event.remain.isNotEmpty)
              Text('剩余 ${d.event.remain}', style: const TextStyle(fontSize: 13, color: Color(0xFFF59E0B), fontWeight: FontWeight.w600)),
          ]),
          if (d.myOption.isNotEmpty) ...[
            const SizedBox(height: 6),
            Text('已押：${d.myOption}', style: const TextStyle(fontSize: 13, color: Color(0xFF6C5CE7), fontWeight: FontWeight.w600)),
          ],
        ]),
      ),
      const SizedBox(height: 12),
      GlassCard(
        padding: const EdgeInsets.all(16),
        child: _detailLoading
            ? const Center(child: Padding(padding: EdgeInsets.all(20), child: CircularProgressIndicator()))
            : Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                const Text('选项（按赔率，热门在前）', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
                const SizedBox(height: 10),
                Wrap(spacing: 8, runSpacing: 8, children: options.map((o) => _optChip(o, d.myOption)).toList()),
                if (d.myOption.isEmpty) ...[
                  const SizedBox(height: 14),
                  Row(children: [
                    const Text('押注魔力', style: TextStyle(fontSize: 13, color: Colors.grey)),
                    const SizedBox(width: 10),
                    SizedBox(
                      width: 130,
                      child: TextField(
                        keyboardType: TextInputType.number,
                        decoration: const InputDecoration(hintText: '金额', isDense: true),
                        onChanged: (v) => _amount = double.tryParse(v) ?? 100,
                      ),
                    ),
                    const Spacer(),
                    _opBtn(_playing ? '提交中…' : '押注', const Color(0xFF059669), _doBet),
                  ]),
                  const SizedBox(height: 6),
                  const Text('可获 = 押注 × 赔率（赛事结束后结算派彩）', style: TextStyle(fontSize: 12, color: Colors.grey)),
                ],
              ]),
      ),
    ];
  }

  Widget _optChip(BetOption o, String myOption) {
    final sel = _selectedOpt == o.id;
    final mine = myOption == o.name;
    return ChoiceChip(
      label: Text('${o.name}  ×${o.odds}', style: TextStyle(fontWeight: FontWeight.w600, color: sel ? Colors.white : null)),
      selected: sel,
      selectedColor: const Color(0xFF6C5CE7),
      backgroundColor: mine ? const Color(0x29F59E0B) : null,
      onSelected: myOption.isEmpty ? (_) => setState(() => _selectedOpt = o.id) : null,
    );
  }

  Widget _opBtn(String label, Color color, VoidCallback onTap) => FilledButton(
        onPressed: _playing ? null : onTap,
        style: FilledButton.styleFrom(backgroundColor: color, padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 12)),
        child: Text(label, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
      );

  Widget _resultView() => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: _last!.success ? const Color(0x1F059669) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(_last!.success ? '✓' : '✗', style: const TextStyle(fontWeight: FontWeight.w800)),
          const SizedBox(width: 8),
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            if (_last!.prize.isNotEmpty) Text(_last!.prize, style: const TextStyle(fontWeight: FontWeight.w700, color: Color(0xFF16A34A))),
            if (_last!.balance > 0) Text('余额 ${_fmt(_last!.balance)}', style: const TextStyle(fontSize: 12, color: Colors.grey)),
            if (_last!.message.isNotEmpty) Text(_last!.message, style: const TextStyle(fontSize: 12, color: Colors.grey)),
          ])),
        ]),
      );
}
