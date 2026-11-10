// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// BigSmallView 的 BigSmallOpsMixin mixin（含 setState 段；从 bigsmall_view.dart 拆出，行为不变）。
// entReadFailed 来自库文件 bigsmall_view.dart 的 import（part 文件自己不许有 import）。
part of 'package:ptpatronus/features/entertainment/bigsmall_view.dart';

mixin BigSmallOpsMixin on State<BigSmallView> {

  int _loadSeq = 0; // _load 单飞序号：丢弃过期响应，防并发竞态让旧数据覆盖新数据（误判封盘→误下注）

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

  // 宿主提供的钩子。
  void _toast(String? m);
  Future<void> _load({bool silent = false}) async {
    final seq = ++_loadSeq;
    if (!silent) setState(() => _loading = true);
    try {
      final s = await _api.bigsmallState(widget.siteId, widget.game);
      if (seq != _loadSeq) return; // 已有更新请求在飞，丢弃本次过期响应
      _state = s;
      _fetchedAt = DateTime.now();
      if (_amount < s.minBet) _amount = s.minBet;
      if (_chat.isEmpty && s.chat.isNotEmpty) {
        _chat = s.chat;
        _lastChatId = s.chat.fold<int>(0, (m, c) => c.id > m ? c.id : m);
      }
    } catch (e) {
      if (seq == _loadSeq && !silent) _toast('加载失败：$e');
    }
    if (seq == _loadSeq && mounted) setState(() {});
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
    } catch (e) {
      if (mounted) entReadFailed(context, 'bigsmallChat', e);
    }
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

  void _allIn() {
    final bal = _state?.balance ?? 0;
    final max = _state?.maxBet ?? bal;
    setState(() => _amount = (max > 0 ? bal < max ? bal : max : bal).floorToDouble());
  }
}
