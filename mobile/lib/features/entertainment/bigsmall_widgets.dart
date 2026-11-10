// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// BigSmallView 的展示层扩展（无 setState 段；从 bigsmall_view.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/entertainment/bigsmall_view.dart';

extension on _BigSmallViewState {
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

}
