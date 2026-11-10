// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 entertainment.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/api/entertainment.dart';

// BetRequest 通用下注请求（selection=big/small/点数/赛事id/tile_index；extra=号码等）。
class BetRequest {
  final String selection;
  final double amount;
  final Map<String, String> extra;
  BetRequest({required this.selection, required this.amount, Map<String, String>? extra}) : extra = extra ?? const {};
  Map<String, dynamic> toJson() {
    final m = <String, dynamic>{'selection': selection, 'amount': amount};
    if (extra.isNotEmpty) m['extra'] = extra;
    return m;
  }
}

// BetOption 赛事押注一个选项（队名/赔率，OurBits 菠菜）。
class BetOption {
  final String id; // 选项 id（下单 POST 用；非主题 id）
  final String name;
  final double odds;
  final double stake;
  BetOption({required this.id, required this.name, required this.odds, this.stake = 0});
  factory BetOption.fromJson(Map<String, dynamic> j) => BetOption(
        id: j['id']?.toString() ?? '',
        name: j['name'] as String? ?? '',
        odds: (j['odds'] as num?)?.toDouble() ?? 0,
        stake: (j['stake'] as num?)?.toDouble() ?? 0,
      );
}

// BetEvent 一场竞猜主题（bet.php 列表行）。
class BetEvent {
  final int id;
  final String title;
  final String endTime;
  final String remain;
  final String bookmaker;
  final int betCount;
  final double totalStake;
  final String myStake;
  final String status; // open / ended
  BetEvent({
    required this.id,
    required this.title,
    required this.endTime,
    required this.remain,
    required this.bookmaker,
    required this.betCount,
    required this.totalStake,
    required this.myStake,
    required this.status,
  });
  factory BetEvent.fromJson(Map<String, dynamic> j) => BetEvent(
        id: (j['id'] as num?)?.toInt() ?? 0,
        title: j['title'] as String? ?? '',
        endTime: j['end_time'] as String? ?? '',
        remain: j['remain'] as String? ?? '',
        bookmaker: j['bookmaker'] as String? ?? '',
        betCount: (j['bet_count'] as num?)?.toInt() ?? 0,
        totalStake: (j['total_stake'] as num?)?.toDouble() ?? 0,
        myStake: j['my_stake'] as String? ?? '',
        status: j['status'] as String? ?? '',
      );
}

// BetEventDetail 某主题盘口（bet_odds.php?id=N：主题 + 选项赔率列表）。
class BetEventDetail {
  final BetEvent event;
  final List<BetOption> options;
  final String myOption;
  final String note;
  BetEventDetail({required this.event, required this.options, this.myOption = '', this.note = ''});
  factory BetEventDetail.fromJson(Map<String, dynamic> j) => BetEventDetail(
        event: BetEvent.fromJson(j['event'] as Map<String, dynamic>? ?? const {}),
        options: ((j['options'] as List?) ?? const []).map((e) => BetOption.fromJson(e as Map<String, dynamic>)).toList(),
        myOption: j['my_option'] as String? ?? '',
        note: j['note'] as String? ?? '',
      );
}

// BlackjackResult 二十一点状态。
class BlackjackResult {
  final List<String> playerCards;
  final List<String> dealerCards;
  final int playerScore;
  final int dealerScore;
  final String stage;
  final String result;
  final double balance;
  final String message;

  BlackjackResult({
    required this.playerCards,
    required this.dealerCards,
    required this.playerScore,
    required this.dealerScore,
    required this.stage,
    required this.result,
    required this.balance,
    required this.message,
  });

  factory BlackjackResult.fromJson(Map<String, dynamic> j) => BlackjackResult(
        playerCards: ((j['player_cards'] as List?) ?? const []).map((e) => e.toString()).toList(),
        dealerCards: ((j['dealer_cards'] as List?) ?? const []).map((e) => e.toString()).toList(),
        playerScore: (j['player_score'] as num?)?.toInt() ?? 0,
        dealerScore: (j['dealer_score'] as num?)?.toInt() ?? 0,
        stage: j['stage'] as String? ?? '',
        result: j['result'] as String? ?? '',
        balance: (j['balance'] as num?)?.toDouble() ?? 0,
        message: j['message'] as String? ?? '',
      );
}
