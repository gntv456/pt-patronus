// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// 二十一点状态机（批3）：new/hit/stand/double，渲染牌面与结果。
class BlackjackGameView extends StatefulWidget {
  final int siteId;
  final String game;
  final String name;
  const BlackjackGameView({super.key, required this.siteId, required this.game, required this.name});
  @override
  State<BlackjackGameView> createState() => _BlackjackGameViewState();
}

class _BlackjackGameViewState extends State<BlackjackGameView> {
  final _api = EntertainmentApi();
  BlackjackResult? _state;
  bool _acting = false;

  @override
  void initState() {
    super.initState();
    _act('new');
  }

  Future<void> _act(String action) async {
    if (_acting) return;
    setState(() => _acting = true);
    try {
      _state = await _api.blackjack(widget.siteId, widget.game, action);
      if (!mounted) return;
      if (_state?.message.isNotEmpty == true) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_state!.message)));
      }
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _acting = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  bool get _finished => _state?.stage == 'finished';

  String get _resultText {
    switch (_state?.result) {
      case 'win':
        return '🎉 赢';
      case 'blackjack':
        return '🎉 Blackjack!';
      case 'lose':
        return '✗ 输';
      case 'push':
        return '平局';
      default:
        return '';
    }
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _hero(),
        const SizedBox(height: 12),
        if (_state != null) _table(),
        const SizedBox(height: 12),
        _ops(),
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF059669), Color(0xFF14B8A6)]),
        ),
        child: Column(children: [
          Text('🃏 ${widget.name}', style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text('余额 ${_state != null ? _state!.balance.toStringAsFixed(0) : '—'}', style: const TextStyle(fontSize: 13, color: Colors.white)),
        ]),
      );

  Widget _table() => GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          _cardsRow('庄家', _state!.dealerCards),
          const SizedBox(height: 12),
          _cardsRow('玩家', _state!.playerCards),
          if (_resultText.isNotEmpty) ...[
            const SizedBox(height: 12),
            Text(_resultText, style: TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: _state!.result == 'lose' ? Colors.redAccent : const Color(0xFF22C55E))),
          ],
          if (_state!.message.isNotEmpty) ...[const SizedBox(height: 6), Text(_state!.message, style: TextStyle(fontSize: 12, color: context.dimText))],
        ]),
      );

  Widget _cardsRow(String label, List<String> cards) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text(label, style: TextStyle(fontSize: 13, color: context.dimText, fontWeight: FontWeight.w600)),
        const SizedBox(height: 4),
        if (cards.isEmpty)
          Text('—', style: TextStyle(color: context.dimText))
        else
          Wrap(spacing: 6, runSpacing: 6, children: cards.map((c) => Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
                decoration: BoxDecoration(color: Colors.white, borderRadius: BorderRadius.circular(8), boxShadow: const [BoxShadow(color: Colors.black26, blurRadius: 2)]),
                child: Text(c, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16, color: Color(0xFF111111))),
              )).toList()),
      ]);

  Widget _ops() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Wrap(
          alignment: WrapAlignment.center,
          spacing: 8,
          runSpacing: 8,
          children: [
            _btn('新一局', const Color(0xFF6366F1), () => _act('new'), disabled: !_finished),
            _btn('要牌', const Color(0xFF2563EB), () => _act('hit'), disabled: _finished),
            _btn('停牌', const Color(0xFFDC2626), () => _act('stand'), disabled: _finished),
            _btn('加倍', const Color(0xFFF59E0B), () => _act('double'), disabled: _finished),
          ],
        ),
      );

  Widget _btn(String label, Color color, VoidCallback onTap, {bool disabled = false}) => FilledButton(
        onPressed: (_acting || disabled) ? null : onTap,
        style: FilledButton.styleFrom(backgroundColor: color, padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10)),
        child: Text(label, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13)),
      );
}
