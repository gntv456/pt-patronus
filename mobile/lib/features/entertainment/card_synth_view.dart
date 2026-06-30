import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';

// 卡牌合成：目标卡 + 碎片库存网格 + 单抽/十连/百连/合成 + 记录（对齐 Web 卡牌合成面板）。
class CardSynthView extends StatefulWidget {
  final int siteId;
  const CardSynthView({super.key, required this.siteId});
  @override
  State<CardSynthView> createState() => _CardSynthViewState();
}

class _CardSynthViewState extends State<CardSynthView> {
  final _api = EntertainmentApi();
  List<SynthesisTarget> _targets = [];
  GameStats? _stats;
  int? _selectedId;
  int _drawing = 0; // 正在抽取次数；0=空闲
  bool _synthesizing = false;
  bool _showHistory = false;
  bool _historyLoading = false;
  List<GameRecord> _history = [];
  String _lastDraw = '';

  @override
  void initState() {
    super.initState();
    _loadAll();
  }

  Future<void> _loadAll() async {
    try {
      final results = await Future.wait([
        _api.syntheses(widget.siteId, 'medal_synthesis').catchError((_) => <SynthesisTarget>[]),
        _api.stats(widget.siteId, 'medal_synthesis').catchError((_) => GameStats(balance: 0, cardNumber: 0, todaySpins: 0, maxDaily: 0, costPerPlay: 500)),
      ]);
      _targets = results[0] as List<SynthesisTarget>;
      _stats = results[1] as GameStats;
      if (_selectedId == null || !_targets.any((t) => t.id == _selectedId)) {
        _selectedId = _targets.isNotEmpty ? _targets.first.id : null;
      }
    } catch (_) {}
    if (mounted) setState(() {});
  }

  SynthesisTarget? get _selected => _targets.where((t) => t.id == _selectedId).firstOrNull;

  Future<void> _draw(int count) async {
    final sel = _selected;
    if (sel == null || _drawing != 0) return;
    setState(() {
      _drawing = count;
      _lastDraw = '';
    });
    try {
      final res = await _api.draw(widget.siteId, 'medal_synthesis', sel.id, count);
      if (res.success) {
        _lastDraw = res.message.isNotEmpty ? res.message : '$count 连完成';
        _toast(res.message.isNotEmpty ? res.message : '$count 连抽取完成');
      } else {
        _toast(res.message.isNotEmpty ? res.message : '抽取失败');
      }
      await _loadAll();
      if (_showHistory) _reloadHistory();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _drawing = 0);
    }
  }

  Future<void> _synthesize() async {
    final sel = _selected;
    if (sel == null || _synthesizing) return;
    setState(() => _synthesizing = true);
    try {
      final res = await _api.synthesize(widget.siteId, 'medal_synthesis', sel.id);
      _toast(res.success ? (res.prize.isNotEmpty ? '合成成功：${res.prize}' : '合成成功') : (res.message.isNotEmpty ? res.message : '合成失败'));
      await _loadAll();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _synthesizing = false);
    }
  }

  Future<void> _toggleHistory() async {
    setState(() => _showHistory = !_showHistory);
    if (_showHistory && _history.isEmpty) {
      setState(() => _historyLoading = true);
      try {
        _history = await _api.history(widget.siteId, 'medal_synthesis');
      } catch (_) {
        _toast('记录加载失败');
      } finally {
        if (mounted) setState(() => _historyLoading = false);
      }
    }
  }

  Future<void> _reloadHistory() async {
    try {
      _history = await _api.history(widget.siteId, 'medal_synthesis');
    } catch (_) {}
    if (mounted) setState(() {});
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }
  String _fmt(int n) => n.toString();

  @override
  Widget build(BuildContext context) {
    if (_targets.isEmpty) {
      return Center(child: Padding(padding: const EdgeInsets.all(30), child: Text('未解析到合成目标（可能未登录或页面结构变化）', textAlign: TextAlign.center, style: const TextStyle(color: Colors.grey, fontSize: 13))));
    }
    final sel = _selected;
    return ListView(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      children: [
        _topbar(),
        const SizedBox(height: 10),
        if (_targets.length > 1) ...[_targetTabs(), const SizedBox(height: 10)],
        if (sel != null) ...[
          IntrinsicHeight(child: Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            _targetCard(sel),
            const SizedBox(width: 10),
            Expanded(child: _right(sel)),
          ])),
        ],
      ],
    );
  }

  Widget _topbar() => Row(children: [
        const Text('卡牌合成', style: TextStyle(fontSize: 20, fontWeight: FontWeight.w800, color: Color(0xFFFF69B4), letterSpacing: 2)),
        const Spacer(),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
          decoration: BoxDecoration(color: const Color(0xFFFFC107), borderRadius: BorderRadius.circular(999)),
          child: Text('当前火花：${_stats != null ? _fmt(_stats!.balance.round()) : '--'}', style: const TextStyle(color: Color(0xFF4A2F00), fontWeight: FontWeight.w700, fontSize: 13)),
        ),
      ]);

  Widget _targetTabs() => Wrap(
        spacing: 6,
        children: _targets.map((t) => GestureDetector(
              onTap: () => setState(() => _selectedId = t.id),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
                decoration: BoxDecoration(
                  color: t.id == _selectedId ? const Color(0xFF6A5ACD) : Colors.white.withValues(alpha: 0.6),
                  borderRadius: BorderRadius.circular(999),
                  border: Border.all(color: const Color(0x4D6A5ACD)),
                ),
                child: Text(t.name, style: TextStyle(fontSize: 12, color: t.id == _selectedId ? Colors.white : const Color(0xFF6A5ACD))),
              ),
            )).toList(),
      );

  Widget _targetCard(SynthesisTarget t) {
    final pct = t.need > 0 ? (t.got * 100 ~/ t.need).clamp(0, 100) : 0;
    return Container(
      width: 130,
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(borderRadius: BorderRadius.circular(12), gradient: const LinearGradient(colors: [Color(0xFF6A5ACD), Color(0xFF8E6FD0)])),
      child: Column(children: [
        ClipRRect(borderRadius: BorderRadius.circular(8), child: t.image.isNotEmpty ? Image.network(t.image, height: 90, width: double.infinity, fit: BoxFit.cover, errorBuilder: (c, e, s) => _placeholder()) : _placeholder()),
        const SizedBox(height: 6),
        Text(t.name, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 14)),
        const SizedBox(height: 6),
        ClipRRect(borderRadius: BorderRadius.circular(3), child: LinearProgressIndicator(value: pct / 100, minHeight: 6, backgroundColor: Colors.white24, valueColor: const AlwaysStoppedAnimation(Color(0xFFFF69B4)))),
        const SizedBox(height: 3),
        Text('${t.got}/${t.need} · $pct%', style: const TextStyle(color: Colors.white70, fontSize: 10)),
        if (t.pity.isNotEmpty) ...[const SizedBox(height: 2), Text('保底 ${t.pity}', style: const TextStyle(color: Colors.white60, fontSize: 9))],
      ]),
    );
  }

  Widget _placeholder() => Container(height: 90, width: double.infinity, color: Colors.white24, child: const Center(child: Text('🏅', style: TextStyle(fontSize: 32))));

  Widget _right(SynthesisTarget t) => Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        _fragGrid(t),
        const SizedBox(height: 8),
        _ops(t),
        if (_lastDraw.isNotEmpty) ...[const SizedBox(height: 6), Text(_lastDraw, style: const TextStyle(fontSize: 11, color: Color(0xFF6A5ACD)))],
        if (_showHistory) ...[const SizedBox(height: 8), _historyView()],
      ]);

  Widget _fragGrid(SynthesisTarget t) => t.fragments.isEmpty
      ? const Padding(padding: EdgeInsets.all(12), child: Text('无碎片数据', style: TextStyle(color: Colors.grey, fontSize: 12)))
      : GridView.count(
          crossAxisCount: 4,
          shrinkWrap: true,
          physics: const NeverScrollableScrollPhysics(),
          mainAxisSpacing: 6,
          crossAxisSpacing: 6,
          childAspectRatio: 0.82,
          children: t.fragments.map((f) {
            final done = f.required > 0 && f.current >= f.required;
            return Container(
              padding: const EdgeInsets.all(4),
              decoration: BoxDecoration(
                color: done ? const Color(0x1F4CAF50) : Colors.white.withValues(alpha: 0.7),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: done ? const Color(0xFF4CAF50) : (f.isRare ? const Color(0xFFA855F7) : const Color(0xFFE0C0C8)), width: 2),
              ),
              child: Column(children: [
                Expanded(child: ClipRRect(borderRadius: BorderRadius.circular(5), child: f.image.isNotEmpty ? Image.network(f.image, fit: BoxFit.cover, errorBuilder: (c, e, s) => _fragPlaceholder()) : _fragPlaceholder())),
                const SizedBox(height: 2),
                Text('${f.current}/${f.required}', style: TextStyle(fontSize: 9, fontWeight: FontWeight.w700, color: done ? const Color(0xFF4CAF50) : const Color(0xFFFF69B4))),
              ]),
            );
          }).toList(),
        );

  Widget _fragPlaceholder() => Container(color: const Color(0x1A6A5ACD), child: const Center(child: Text('💠', style: TextStyle(fontSize: 16))));

  Widget _ops(SynthesisTarget t) => Row(children: [
        Expanded(child: _opBtn('单抽', _drawing, 1, t, const Color(0xFFFF69B4), Colors.white)),
        const SizedBox(width: 4),
        Expanded(child: _opBtn('十连', _drawing, 10, t, const Color(0xFFFFC107), const Color(0xFF4A2F00))),
        const SizedBox(width: 4),
        Expanded(child: _opBtn('百连', _drawing, 100, t, const Color(0xFFF44336), Colors.white)),
        const SizedBox(width: 4),
        Expanded(child: GestureDetector(
          onTap: (_synthesizing || !t.canSynthesize) ? null : _synthesize,
          child: Opacity(opacity: (_synthesizing || !t.canSynthesize) ? 0.4 : 1, child: _opBox(_synthesizing ? '…' : '合成', const Color(0xFFFFEbee), const Color(0xFFFF69B4))),
        )),
        const SizedBox(width: 4),
        Expanded(child: GestureDetector(onTap: _toggleHistory, child: _opBox('记录', _showHistory ? const Color(0xFF6A5ACD) : const Color(0xFF2196F3), Colors.white))),
      ]);

  Widget _opBtn(String label, int drawing, int count, SynthesisTarget t, Color bg, Color fg) => GestureDetector(
        onTap: drawing != 0 ? null : () => _draw(count),
        child: Opacity(opacity: drawing != 0 ? 0.4 : 1, child: _opBox(drawing == count ? '…' : label, bg, fg)),
      );

  Widget _opBox(String label, Color bg, Color fg) => Container(
        height: 40,
        decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(8)),
        child: Center(child: Text(label, style: TextStyle(color: fg, fontWeight: FontWeight.w700, fontSize: 12))),
      );

  Widget _historyView() => Container(
        padding: const EdgeInsets.all(8),
        decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.6), borderRadius: BorderRadius.circular(8)),
        child: _historyLoading
            ? const Center(child: SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2)))
            : Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('抽取记录（${_history.length}）', style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: Color(0xFF6A5ACD))),
                const SizedBox(height: 4),
                if (_history.isEmpty)
                  const Text('暂无记录', style: TextStyle(color: Colors.grey, fontSize: 12))
                else
                  Column(children: _history.take(10).map((r) => Padding(
                    padding: const EdgeInsets.only(top: 3),
                    child: Row(children: [
                      Expanded(child: Text(r.prizeName.isNotEmpty ? r.prizeName : '—', style: const TextStyle(color: Color(0xFFFF69B4), fontSize: 11, fontWeight: FontWeight.w600))),
                      if (r.cost > 0) Text('-${_fmt(r.cost.round())}', style: const TextStyle(color: Color(0xFFE67E22), fontSize: 10)),
                      const SizedBox(width: 6),
                      Text(r.createdAt, style: const TextStyle(color: Colors.grey, fontSize: 9)),
                    ]),
                  )).toList()),
              ]),
      );
}
