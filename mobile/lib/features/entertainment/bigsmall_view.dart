import 'dart:async';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// 猜大小（好学 hxpt_bigsmall + 熊猫 panda_bigsmall 共用）。
// 时间：后端 BigSmallState 给权威 countdown/seal_countdown/status 快照；前端按 fetchedAt 本地每秒递减
// （自动补偿往返耗时/时钟偏差），仅在新局(≤0)/封盘切换/下注后/25s 兜底回拉。封盘=status≠betting 或 seal_remaining≤0。
class BigSmallView extends StatefulWidget {
  final int siteId;
  final String game;
  final String name;
  const BigSmallView({super.key, required this.siteId, required this.game, required this.name});
  @override
  State<BigSmallView> createState() => _BigSmallViewState();
}

class _BigSmallViewState extends State<BigSmallView> {
  final _api = EntertainmentApi();
  BigSmallState? _state;
  bool _loading = true;
  bool _betting = false;
  DateTime _fetchedAt = DateTime.fromMillisecondsSinceEpoch(0);

  double _amount = 1000;
  String _propId = '0';
  int _rankTab = 0;
  bool _recMy = true;

  // 聊天
  final _chatCtrl = TextEditingController();
  List<BigSmallChat> _chat = [];
  int _lastChatId = 0;
  bool _sending = false;

  // 红包
  final _rpAmount = TextEditingController(text: '1000');
  final _rpCount = TextEditingController(text: '5');
  final _rpGreet = TextEditingController(text: '恭喜发财');
  bool _rpSending = false;

  Timer? _tick, _poll, _chatTimer;
  static const _quicks = [1000, 5000, 10000, 50000, 100000];

  bool get _isHxpt => widget.game.startsWith('hxpt');

  int get _remaining {
    final elapsed = DateTime.now().difference(_fetchedAt).inSeconds;
    return ((_state?.round.countdown ?? 0) - elapsed).clamp(0, 1 << 30);
  }

  int get _sealRemaining {
    final elapsed = DateTime.now().difference(_fetchedAt).inSeconds;
    return ((_state?.round.sealCountdown ?? 0) - elapsed).clamp(0, 1 << 30);
  }

  bool get _canBet => _state != null && _state!.round.status == 'betting' && _sealRemaining > 0 && !_betting;

  String get _statusText {
    final s = _state?.round.status ?? '';
    if (s == 'sealed') return '已封盘，即将开奖…';
    if (s == 'finished') return '本局已结束，下一局即将开始';
    if (s == 'betting' && _sealRemaining <= 3) return '即将封盘（${_sealRemaining}s）';
    return '下注中 · 剩余 ${_remaining}s';
  }

  @override
  void initState() {
    super.initState();
    _load();
    _tick = Timer.periodic(const Duration(seconds: 1), (_) {
      if (!mounted) return;
      setState(() {});
      if (_remaining <= 0 || (_state?.round.status == 'betting' && _sealRemaining <= 0)) {
        _load(silent: true);
      }
    });
    _poll = Timer.periodic(const Duration(seconds: 25), (_) {
      if (mounted && !_loading) _load(silent: true);
    });
    _chatTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      if (mounted) _refreshChat();
    });
  }

  @override
  void dispose() {
    _tick?.cancel();
    _poll?.cancel();
    _chatTimer?.cancel();
    _chatCtrl.dispose();
    _rpAmount.dispose();
    _rpCount.dispose();
    _rpGreet.dispose();
    super.dispose();
  }

  Future<void> _load({bool silent = false}) async {
    if (!silent) setState(() => _loading = true);
    try {
      final s = await _api.bigsmallState(widget.siteId, widget.game);
      _state = s;
      _fetchedAt = DateTime.now();
      if (_amount < s.minBet) _amount = s.minBet;
      if (_chat.isEmpty && s.chat.isNotEmpty) {
        _chat = s.chat;
        _lastChatId = s.chat.fold<int>(0, (m, c) => c.id > m ? c.id : m);
      }
    } catch (e) {
      if (!silent) _toast('加载失败：$e');
    }
    if (mounted) setState(() {});
  }

  Future<void> _refreshChat() async {
    try {
      final list = await _api.bigsmallChat(widget.siteId, widget.game, _lastChatId);
      if (list.isEmpty) return;
      final map = {for (final m in _chat) m.id: m};
      for (final m in list) {
        map[m.id] = m;
      }
      _chat = map.values.toList()..sort((a, b) => a.id.compareTo(b.id));
      if (_chat.length > 60) _chat = _chat.sublist(_chat.length - 60);
      _lastChatId = _chat.fold<int>(_lastChatId, (m, c) => c.id > m ? c.id : m);
      if (mounted) setState(() {});
    } catch (_) {}
  }

  Future<void> _doBet(String type) async {
    if (!_canBet) return;
    setState(() => _betting = true);
    try {
      final extra = <String, String>{};
      if (_isHxpt && _propId != '0') extra['prop_id'] = _propId;
      var res = await _api.bet(widget.siteId, widget.game, BetRequest(selection: type, amount: _amount, extra: extra));
      if (!res.success && res.raw?['captcha_required'] == true) {
        final code = await _askCaptcha();
        if (code != null) {
          extra['captcha_hash'] = (res.raw?['imagehash'] ?? '').toString();
          extra['captcha_code'] = code;
          res = await _api.bet(widget.siteId, widget.game, BetRequest(selection: type, amount: _amount, extra: extra));
        }
      }
      _toast(res.success ? (res.prize.isNotEmpty ? res.prize : res.message) : res.message);
      await _load(silent: true);
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _betting = false);
    }
  }

  Future<String?> _askCaptcha() async {
    final ctrl = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('安全验证'),
        content: TextField(controller: ctrl, autofocus: true, decoration: const InputDecoration(hintText: '请输入图中字符')),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, ctrl.text.trim()), child: const Text('确定')),
        ],
      ),
    );
  }

  Future<void> _sendChat() async {
    final msg = _chatCtrl.text.trim();
    if (msg.isEmpty || _sending) return;
    setState(() => _sending = true);
    try {
      final res = await _api.bigsmallSendChat(widget.siteId, widget.game, msg);
      if (res.success) {
        _chatCtrl.clear();
        await _refreshChat();
      } else {
        _toast(res.message);
      }
    } catch (e) {
      _toast('发送失败：$e');
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  Future<void> _doTask(BigSmallTask t, String action) async {
    try {
      final res = await _api.bigsmallTask(widget.siteId, widget.game, action, t.id);
      _toast(res.message);
      if (res.success) await _load(silent: true);
    } catch (e) {
      _toast('操作失败：$e');
    }
  }

  Future<void> _useRefund() async {
    try {
      final res = await _api.bigsmallTask(widget.siteId, widget.game, 'refund', 0);
      _toast(res.message);
      if (res.success) await _load(silent: true);
    } catch (e) {
      _toast('操作失败：$e');
    }
  }

  Future<void> _useProp(BigSmallProp p) async {
    try {
      final res = await _api.bigsmallItem(widget.siteId, widget.game, p.tag);
      _toast(res.message);
      if (res.success) await _load(silent: true);
    } catch (e) {
      _toast('使用失败：$e');
    }
  }

  Future<void> _claimRed(BigSmallRedpacket r) async {
    try {
      final res = await _api.bigsmallRedpacket(widget.siteId, widget.game, 'claim', {'id': r.id});
      _toast(res.message);
      if (res.success) await _load(silent: true);
    } catch (e) {
      _toast('操作失败：$e');
    }
  }

  Future<void> _sendRed() async {
    if (_rpSending) return;
    setState(() => _rpSending = true);
    try {
      final res = await _api.bigsmallRedpacket(widget.siteId, widget.game, 'send', {
        'amount': _rpAmount.text,
        'count': _rpCount.text,
        'greeting': _rpGreet.text,
      });
      _toast(res.message);
      if (res.success) await _load(silent: true);
    } catch (e) {
      _toast('发送失败：$e');
    } finally {
      if (mounted) setState(() => _rpSending = false);
    }
  }

  void _toast(String? m) {
    if (!mounted || m == null || m.isEmpty) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  String _fmt(double? n) => n == null ? '0' : n.toStringAsFixed(0);

  String _label(String r) => r == 'big' ? '大' : (r == 'house' ? '庄' : (r == 'small' ? '小' : '-'));

  Color _dotColor(String r) => r == 'big' ? const Color(0xFFEF4444) : (r == 'house' ? const Color(0xFF22C55E) : const Color(0xFF3B82F6));

  void _allIn() {
    final bal = _state?.balance ?? 0;
    final max = _state?.maxBet ?? bal;
    setState(() => _amount = (max > 0 ? bal < max ? bal : max : bal).floorToDouble());
  }

  @override
  Widget build(BuildContext context) {
    if (_loading && _state == null) {
      return const Center(child: Padding(padding: EdgeInsets.all(40), child: CircularProgressIndicator()));
    }
    final s = _state;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _hero(s),
        const SizedBox(height: 12),
        _statBar(s),
        if (s?.lastResult != null) ...[const SizedBox(height: 10), _lastResult(s!.lastResult!)],
        const SizedBox(height: 12),
        _trendPanel(s),
        const SizedBox(height: 12),
        _betPanel(s),
        const SizedBox(height: 12),
        _rankPanel(s),
        const SizedBox(height: 12),
        _taskPanel(s),
        const SizedBox(height: 12),
        _redpacketPanel(s),
        const SizedBox(height: 12),
        _recordsPanel(s),
        const SizedBox(height: 12),
        _chatPanel(),
      ],
    );
  }

  Widget _hero(BigSmallState? s) => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFFDB2777), Color(0xFFF59E0B)]),
        ),
        child: Column(children: [
          Text('🎲 ${widget.name}', style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text.rich(TextSpan(text: '余额 ', style: const TextStyle(color: Colors.white, fontSize: 13), children: [
            TextSpan(text: _fmt(s?.balance), style: const TextStyle(color: Color(0xFFFDE68A), fontSize: 16, fontWeight: FontWeight.bold)),
            if (s?.currency.isNotEmpty == true) TextSpan(text: ' ${s!.currency}', style: const TextStyle(color: Colors.white70, fontSize: 12)),
            if (s?.payout.isNotEmpty == true) TextSpan(text: '  · 赔率 ${s!.payout}×', style: const TextStyle(color: Colors.white70, fontSize: 12)),
          ])),
        ]),
      );

  Widget _statBar(BigSmallState? s) {
    final st = s?.round.status ?? '';
    return Row(children: [
      _statTile('$_remaining s', '倒计时', st != 'betting' ? PTColors.warn : PTColors.primary),
      const SizedBox(width: 8),
      _statTile(st == 'betting' ? '可下注' : (st == 'sealed' ? '封盘' : '已结束'), _statusText, PTColors.primary, flex: 2),
      const SizedBox(width: 8),
      _statTile('第 ${s?.round.gameNo ?? '?'} 局', '${s?.round.playerCount ?? 0}人 · 总注 ${_fmt(s?.round.totalBet)}', PTColors.primary, flex: 2),
    ]);
  }

  Widget _statTile(String val, String lbl, Color color, {int flex = 1}) => Expanded(
        flex: flex,
        child: GlassCard(
          padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 4),
          child: Column(children: [
            Text(val, style: TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: color)),
            const SizedBox(height: 2),
            Text(lbl, textAlign: TextAlign.center, style: const TextStyle(fontSize: 11, color: Colors.grey)),
          ]),
        ),
      );

  Widget _lastResult(BigSmallMyBet lr) => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: (lr.netResult >= 0 ? PTColors.success : PTColors.danger).withValues(alpha: 0.12),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Text(
          '上局 #${lr.gameNo} 开 ${_label(lr.result)}，你押 ${_label(lr.betType)} ${_fmt(lr.amount)} → ${lr.netResult >= 0 ? '+' : ''}${_fmt(lr.netResult)}',
          style: const TextStyle(fontSize: 13),
        ),
      );

  Widget _trendPanel(BigSmallState? s) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('📊 走势分析', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          _trendBlock('40场', s?.trend40, s?.hasHouse ?? false),
          const SizedBox(height: 6),
          _trendBlock('最近10场', s?.trend10, false),
          if (s?.recentResults.isNotEmpty == true) ...[
            const SizedBox(height: 8),
            Wrap(spacing: 4, runSpacing: 4, children: [
              for (final r in s!.recentResults.take(40))
                Container(width: 20, height: 20, alignment: Alignment.center, decoration: BoxDecoration(color: _dotColor(r), shape: BoxShape.circle), child: Text(_label(r), style: const TextStyle(color: Colors.white, fontSize: 10, fontWeight: FontWeight.bold))),
            ]),
          ],
          if (s?.trendText.isNotEmpty == true) ...[const SizedBox(height: 6), Center(child: Text(s!.trendText, style: const TextStyle(fontSize: 12, color: PTColors.primary)))],
        ]),
      );

  Widget _trendBlock(String cap, BigSmallTrend? t, bool showHouse) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text(cap, style: const TextStyle(fontSize: 11, color: Colors.grey)),
        const SizedBox(height: 3),
        Row(children: [
          Expanded(flex: t?.bigPct ?? 1, child: Container(height: 16, color: const Color(0xFFEF4444), alignment: Alignment.center, child: Text('大 ${t?.bigCount ?? 0}', style: const TextStyle(color: Colors.white, fontSize: 10)))),
          Expanded(flex: t?.smallPct ?? 1, child: Container(height: 16, color: const Color(0xFF3B82F6), alignment: Alignment.center, child: Text('小 ${t?.smallCount ?? 0}', style: const TextStyle(color: Colors.white, fontSize: 10)))),
        ]),
        if (showHouse && (t?.houseCount ?? 0) > 0) Text('通杀 ${t!.houseCount}', style: const TextStyle(fontSize: 10, color: Color(0xFF22C55E))),
      ]);

  Widget _betPanel(BigSmallState? s) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('💰 下注', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          if (s?.myBet != null) ...[const SizedBox(height: 4), Text('本局已押 ${_label(s!.myBet!.betType)} ${_fmt(s.myBet!.amount)}', style: const TextStyle(fontSize: 12, color: Colors.grey))],
          const SizedBox(height: 8),
          Row(children: [
            SizedBox(width: 120, child: TextField(keyboardType: TextInputType.number, decoration: const InputDecoration(hintText: '金额', isDense: true), onChanged: (v) => _amount = double.tryParse(v) ?? _amount)),
            const SizedBox(width: 8),
            Text('最低 ${_fmt(s?.minBet)}', style: const TextStyle(fontSize: 11, color: Colors.grey)),
            const Spacer(),
            FilledButton(onPressed: _allIn, style: FilledButton.styleFrom(backgroundColor: PTColors.warn, padding: const EdgeInsets.symmetric(horizontal: 16)), child: const Text('梭哈')),
          ]),
          const SizedBox(height: 8),
          Wrap(spacing: 6, runSpacing: 6, children: [
            for (final q in _quicks)
              OutlinedButton(onPressed: () => setState(() => _amount = q.toDouble()), child: Text(q.toString())),
          ]),
          if (_isHxpt && (s?.props.any((p) => p.usable) ?? false)) ...[
            const SizedBox(height: 8),
            DropdownButton<String>(value: _propId, isExpanded: true, items: [
              const DropdownMenuItem(value: '0', child: Text('不使用道具')),
              for (final p in s!.props.where((p) => p.usable)) DropdownMenuItem(value: p.id, child: Text('${p.name} ×${p.quantity}')),
            ], onChanged: (v) => setState(() => _propId = v ?? '0')),
          ],
          const SizedBox(height: 10),
          Row(children: [
            Expanded(child: FilledButton(onPressed: _canBet ? () => _doBet('big') : null, style: FilledButton.styleFrom(backgroundColor: const Color(0xFFDC2626), padding: const EdgeInsets.symmetric(vertical: 14)), child: const Text('押 大', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w800)))),
            const SizedBox(width: 10),
            Expanded(child: FilledButton(onPressed: _canBet ? () => _doBet('small') : null, style: FilledButton.styleFrom(backgroundColor: const Color(0xFF2563EB), padding: const EdgeInsets.symmetric(vertical: 14)), child: const Text('押 小', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w800)))),
          ]),
          const SizedBox(height: 8),
          Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Expanded(child: _bettorCol('押大', s?.bigBets ?? [], const Color(0xFFEF4444))),
            const SizedBox(width: 8),
            Expanded(child: _bettorCol('押小', s?.smallBets ?? [], const Color(0xFF3B82F6))),
          ]),
        ]),
      );

  Widget _bettorCol(String cap, List<BigSmallBetSpot> list, Color c) => Container(
        padding: const EdgeInsets.all(8),
        decoration: BoxDecoration(color: c.withValues(alpha: 0.1), borderRadius: BorderRadius.circular(8)),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(cap, style: TextStyle(fontWeight: FontWeight.bold, color: c, fontSize: 12)),
          if (list.isEmpty) const Text('暂无', style: TextStyle(fontSize: 11, color: Colors.grey))
          else for (final b in list.take(6)) Text('${b.username} ${_fmt(b.amount)}', style: const TextStyle(fontSize: 10, color: Colors.grey)),
        ]),
      );

  Widget _rankPanel(BigSmallState? s) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('🏆 用户排行', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          if (s == null || s.rankings.isEmpty)
            const Text('暂无排行', style: TextStyle(color: Colors.grey, fontSize: 13))
          else ...[
            Wrap(spacing: 4, runSpacing: 4, children: [
              for (var i = 0; i < s.rankings.length; i++)
                ChoiceChip(label: Text(s.rankings[i].title, style: const TextStyle(fontSize: 11)), selected: _rankTab == i, onSelected: (_) => setState(() => _rankTab = i)),
            ]),
            const SizedBox(height: 6),
            if (_rankTab < s.rankings.length)
              for (final e in s.rankings[_rankTab].entries.asMap().entries)
                _rankRow(e.key + 1, e.value),
          ],
        ]),
      );

  Widget _rankRow(int no, BigSmallRankEntry e) {
    final medal = no == 1 ? PTColors.warn : (no == 2 ? Colors.grey : (no == 3 ? const Color(0xFFB45309) : Colors.transparent));
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(children: [
        Container(width: 18, height: 18, alignment: Alignment.center, decoration: BoxDecoration(color: medal, shape: BoxShape.circle), child: Text('$no', style: const TextStyle(fontSize: 10, color: Colors.white))),
        const SizedBox(width: 6),
        Expanded(child: Text(e.username, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 12))),
        Text(_fmt(e.amount.abs()), style: const TextStyle(color: PTColors.success, fontWeight: FontWeight.w600, fontSize: 12)),
        if (e.count > 0) Text(' ${e.count}次', style: const TextStyle(fontSize: 10, color: Colors.grey)),
      ]),
    );
  }

  Widget _taskPanel(BigSmallState? s) {
    final tasks = s?.tasks ?? [];
    final props = s?.props ?? [];
    if (tasks.isEmpty && props.isEmpty) return const SizedBox.shrink();
    final refundable = _isHxpt && props.any((p) => !p.usable);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        const Text('🎯 任务 / 道具', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
        const SizedBox(height: 6),
        for (final t in tasks)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 3),
            child: Row(children: [
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(t.title, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 12)),
                if (t.progressText.isNotEmpty) Text(t.progressText, style: const TextStyle(fontSize: 10, color: Colors.grey)),
              ])),
              if (t.status == 'rewardable')
                _mini('领奖', PTColors.success, () => _doTask(t, 'reward'))
              else if (t.status == 'idle')
                _mini('接取', PTColors.primary, () => _doTask(t, 'claim'))
              else
                Text(t.status == 'doing' ? '进行中' : '已完成', style: const TextStyle(fontSize: 11, color: Colors.grey)),
            ]),
          ),
        if (refundable) ...[const SizedBox(height: 6), SizedBox(width: double.infinity, child: _mini('💳 使用亏返卡', PTColors.warn, _useRefund))],
        if (!_isHxpt) for (final p in props) Padding(padding: const EdgeInsets.symmetric(vertical: 3), child: Row(children: [Expanded(child: Text('${p.name} ×${p.quantity}', style: const TextStyle(fontSize: 12))), _mini('使用', PTColors.primary, () => _useProp(p))])),
      ]),
    );
  }

  Widget _mini(String label, Color c, VoidCallback onTap) => FilledButton(
        onPressed: onTap,
        style: FilledButton.styleFrom(backgroundColor: c, padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4), minimumSize: const Size(0, 28)),
        child: Text(label, style: const TextStyle(fontSize: 11)),
      );

  Widget _redpacketPanel(BigSmallState? s) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('🧧 红包', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          Row(children: [
            Expanded(child: TextField(controller: _rpAmount, keyboardType: TextInputType.number, decoration: const InputDecoration(hintText: '总额', isDense: true))),
            const SizedBox(width: 8),
            Expanded(child: TextField(controller: _rpCount, keyboardType: TextInputType.number, decoration: const InputDecoration(hintText: '个数', isDense: true))),
          ]),
          const SizedBox(height: 6),
          TextField(controller: _rpGreet, decoration: const InputDecoration(hintText: '祝福语', isDense: true)),
          const SizedBox(height: 8),
          SizedBox(width: double.infinity, child: FilledButton(onPressed: _rpSending ? null : _sendRed, child: Text(_rpSending ? '发送中…' : '发红包'))),
          if (s?.redpackets.isNotEmpty == true) ...[
            const SizedBox(height: 8),
            for (final r in s!.redpackets)
              Padding(padding: const EdgeInsets.symmetric(vertical: 2), child: OutlinedButton(onPressed: () => _claimRed(r), child: Text('🧧 ${r.count}个 / ${_fmt(r.total)}'))),
          ],
        ]),
      );

  Widget _recordsPanel(BigSmallState? s) {
    final list = _recMy ? (s?.myRecords ?? []) : (s?.allRecords ?? []);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Text('📋 游戏记录', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const Spacer(),
          ChoiceChip(label: const Text('我的', style: TextStyle(fontSize: 11)), selected: _recMy, onSelected: (_) => setState(() => _recMy = true)),
          const SizedBox(width: 4),
          ChoiceChip(label: const Text('所有人', style: TextStyle(fontSize: 11)), selected: !_recMy, onSelected: (_) => setState(() => _recMy = false)),
        ]),
        const SizedBox(height: 8),
        if (list.isEmpty)
          const Text('暂无记录', style: TextStyle(color: Colors.grey, fontSize: 13))
        else
          for (final r in list)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 3),
              child: Row(children: [
                Text('#${r.gameNo}', style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600)),
                const SizedBox(width: 8),
                Text(_label(r.result), style: TextStyle(fontSize: 11, fontWeight: FontWeight.bold, color: _dotColor(r.result))),
                const Spacer(),
                if (_recMy) Text('${_label(r.myBetType)} ${_fmt(r.myBet)} ', style: const TextStyle(fontSize: 11, color: Colors.grey)),
                if (_recMy) Text('${r.myResult >= 0 ? '+' : ''}${_fmt(r.myResult)}', style: TextStyle(fontSize: 11, color: r.myResult >= 0 ? PTColors.success : PTColors.danger, fontWeight: FontWeight.bold)),
                if (!_recMy && r.bets.isNotEmpty) Flexible(child: Text(r.bets, style: const TextStyle(fontSize: 10, color: Colors.grey), overflow: TextOverflow.ellipsis)),
              ]),
            ),
      ]),
    );
  }

  Widget _chatPanel() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('💬 聊天', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 6),
          Container(
            constraints: const BoxConstraints(maxHeight: 180),
            child: ListView.builder(
              shrinkWrap: true,
              itemCount: _chat.length,
              itemBuilder: (_, i) {
                final m = _chat[i];
                return Padding(padding: const EdgeInsets.symmetric(vertical: 1), child: Text.rich(TextSpan(children: [
                  TextSpan(text: m.username, style: const TextStyle(fontWeight: FontWeight.bold, color: PTColors.primary, fontSize: 12)),
                  TextSpan(text: '  ${m.message}', style: const TextStyle(fontSize: 12)),
                ])));
              },
            ),
          ),
          const SizedBox(height: 8),
          Row(children: [
            Expanded(child: TextField(controller: _chatCtrl, decoration: const InputDecoration(hintText: '发条消息…', isDense: true), onSubmitted: (_) => _sendChat())),
            const SizedBox(width: 8),
            FilledButton(onPressed: _sending ? null : _sendChat, child: const Text('发送')),
          ]),
        ]),
      );
}
