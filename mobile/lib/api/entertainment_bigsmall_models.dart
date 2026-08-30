// 从 entertainment.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/api/entertainment.dart';

// ── 猜大小（好学 hxpt_bigsmall + 熊猫 panda_bigsmall） ──
class BigSmallRound {
  final int id;
  final String gameNo;
  final String startTime;
  final String endTime;
  final String status; // betting | sealed | finished
  final int countdown;
  final int sealCountdown;
  final int playerCount;
  final double totalBet;
  BigSmallRound({required this.id, required this.gameNo, required this.startTime, required this.endTime, required this.status, required this.countdown, required this.sealCountdown, required this.playerCount, required this.totalBet});
  factory BigSmallRound.fromJson(Map<String, dynamic> j) => BigSmallRound(
        id: (j['id'] as num?)?.toInt() ?? 0,
        gameNo: j['game_no']?.toString() ?? '',
        startTime: j['start_time'] as String? ?? '',
        endTime: j['end_time'] as String? ?? '',
        status: j['status'] as String? ?? '',
        countdown: (j['countdown'] as num?)?.toInt() ?? 0,
        sealCountdown: (j['seal_countdown'] as num?)?.toInt() ?? 0,
        playerCount: (j['player_count'] as num?)?.toInt() ?? 0,
        totalBet: (j['total_bet'] as num?)?.toDouble() ?? 0,
      );
}

class BigSmallBetSpot {
  final String username;
  final double amount;
  BigSmallBetSpot({required this.username, required this.amount});
  factory BigSmallBetSpot.fromJson(Map<String, dynamic> j) => BigSmallBetSpot(
        username: j['username'] as String? ?? '',
        amount: (j['amount'] as num?)?.toDouble() ?? 0,
      );
}

class BigSmallMyBet {
  final String gameNo;
  final String betType;
  final double amount;
  final String result;
  final double netResult;
  BigSmallMyBet({required this.gameNo, required this.betType, required this.amount, required this.result, required this.netResult});
  factory BigSmallMyBet.fromJson(Map<String, dynamic> j) => BigSmallMyBet(
        gameNo: j['game_no']?.toString() ?? '',
        betType: j['bet_type'] as String? ?? '',
        amount: (j['amount'] as num?)?.toDouble() ?? 0,
        result: j['result'] as String? ?? '',
        netResult: (j['net_result'] as num?)?.toDouble() ?? 0,
      );
}

class BigSmallTrend {
  final int bigCount;
  final int smallCount;
  final int houseCount;
  final int bigPct;
  final int smallPct;
  BigSmallTrend({required this.bigCount, required this.smallCount, required this.houseCount, required this.bigPct, required this.smallPct});
  factory BigSmallTrend.fromJson(Map<String, dynamic> j) => BigSmallTrend(
        bigCount: (j['big_count'] as num?)?.toInt() ?? 0,
        smallCount: (j['small_count'] as num?)?.toInt() ?? 0,
        houseCount: (j['house_count'] as num?)?.toInt() ?? 0,
        bigPct: (j['big_pct'] as num?)?.toInt() ?? 0,
        smallPct: (j['small_pct'] as num?)?.toInt() ?? 0,
      );
}

class BigSmallRankEntry {
  final String username;
  final double amount;
  final int count;
  BigSmallRankEntry({required this.username, required this.amount, required this.count});
  factory BigSmallRankEntry.fromJson(Map<String, dynamic> j) => BigSmallRankEntry(
        username: j['username'] as String? ?? '',
        amount: (j['amount'] as num?)?.toDouble() ?? 0,
        count: (j['count'] as num?)?.toInt() ?? 0,
      );
}

class BigSmallRank {
  final String key;
  final String title;
  final List<BigSmallRankEntry> entries;
  BigSmallRank({required this.key, required this.title, required this.entries});
  factory BigSmallRank.fromJson(Map<String, dynamic> j) => BigSmallRank(
        key: j['key'] as String? ?? '',
        title: j['title'] as String? ?? '',
        entries: ((j['entries'] as List?) ?? []).map((e) => BigSmallRankEntry.fromJson(e as Map<String, dynamic>)).toList(),
      );
}

class BigSmallRecord {
  final String gameNo;
  final String startTime;
  final String endTime;
  final int playerCount;
  final double totalBet;
  final String result;
  final String myBetType;
  final double myBet;
  final double myResult;
  final String bets;
  BigSmallRecord({required this.gameNo, required this.startTime, required this.endTime, required this.playerCount, required this.totalBet, required this.result, required this.myBetType, required this.myBet, required this.myResult, required this.bets});
  factory BigSmallRecord.fromJson(Map<String, dynamic> j) => BigSmallRecord(
        gameNo: j['game_no']?.toString() ?? '',
        startTime: j['start_time'] as String? ?? '',
        endTime: j['end_time'] as String? ?? '',
        playerCount: (j['player_count'] as num?)?.toInt() ?? 0,
        totalBet: (j['total_bet'] as num?)?.toDouble() ?? 0,
        result: j['result'] as String? ?? '',
        myBetType: j['my_bet_type'] as String? ?? '',
        myBet: (j['my_bet'] as num?)?.toDouble() ?? 0,
        myResult: (j['my_result'] as num?)?.toDouble() ?? 0,
        bets: j['bets'] as String? ?? '',
      );
}

class BigSmallTask {
  final int id;
  final String title;
  final String desc;
  final String reward;
  final String progressText;
  final int progress;
  final String status; // idle | doing | rewardable | done
  BigSmallTask({required this.id, required this.title, required this.desc, required this.reward, required this.progressText, required this.progress, required this.status});
  factory BigSmallTask.fromJson(Map<String, dynamic> j) => BigSmallTask(
        id: (j['id'] as num?)?.toInt() ?? 0,
        title: j['title'] as String? ?? '',
        desc: j['desc'] as String? ?? '',
        reward: j['reward'] as String? ?? '',
        progressText: j['progress_text'] as String? ?? '',
        progress: (j['progress'] as num?)?.toInt() ?? 0,
        status: j['status'] as String? ?? '',
      );
}

class BigSmallProp {
  final String id;
  final String tag;
  final String name;
  final String desc;
  final int quantity;
  final bool usable;
  BigSmallProp({required this.id, required this.tag, required this.name, required this.desc, required this.quantity, required this.usable});
  factory BigSmallProp.fromJson(Map<String, dynamic> j) => BigSmallProp(
        id: j['id']?.toString() ?? '',
        tag: j['tag'] as String? ?? '',
        name: j['name'] as String? ?? '',
        desc: j['desc'] as String? ?? '',
        quantity: (j['quantity'] as num?)?.toInt() ?? 0,
        usable: j['usable'] as bool? ?? false,
      );
}

class BigSmallChat {
  final int id;
  final String username;
  final String message;
  final String time;
  BigSmallChat({required this.id, required this.username, required this.message, required this.time});
  factory BigSmallChat.fromJson(Map<String, dynamic> j) => BigSmallChat(
        id: (j['id'] as num?)?.toInt() ?? 0,
        username: j['username'] as String? ?? '',
        message: j['message'] as String? ?? '',
        time: j['time'] as String? ?? '',
      );
}

class BigSmallRedpacket {
  final String id;
  final String sender;
  final String greeting;
  final double total;
  final int count;
  BigSmallRedpacket({required this.id, required this.sender, required this.greeting, required this.total, required this.count});
  factory BigSmallRedpacket.fromJson(Map<String, dynamic> j) => BigSmallRedpacket(
        id: j['id']?.toString() ?? '',
        sender: j['sender'] as String? ?? '',
        greeting: j['greeting'] as String? ?? '',
        total: (j['total'] as num?)?.toDouble() ?? 0,
        count: (j['count'] as num?)?.toInt() ?? 0,
      );
}

class BigSmallState {
  final double balance;
  final double minBet;
  final double maxBet;
  final String currency;
  final String payout;
  final bool hasHouse;
  final BigSmallRound round;
  final BigSmallMyBet? myBet;
  final BigSmallMyBet? lastResult;
  final List<BigSmallBetSpot> bigBets;
  final List<BigSmallBetSpot> smallBets;
  final BigSmallTrend trend40;
  final BigSmallTrend trend10;
  final String trendText;
  final List<String> recentResults;
  final List<BigSmallRank> rankings;
  final List<BigSmallRecord> myRecords;
  final List<BigSmallRecord> allRecords;
  final List<BigSmallTask> tasks;
  final List<BigSmallProp> props;
  final List<BigSmallChat> chat;
  final List<BigSmallRedpacket> redpackets;
  BigSmallState({required this.balance, required this.minBet, required this.maxBet, required this.currency, required this.payout, required this.hasHouse, required this.round, required this.myBet, required this.lastResult, required this.bigBets, required this.smallBets, required this.trend40, required this.trend10, required this.trendText, required this.recentResults, required this.rankings, required this.myRecords, required this.allRecords, required this.tasks, required this.props, required this.chat, required this.redpackets});
  factory BigSmallState.fromJson(Map<String, dynamic> j) => BigSmallState(
        balance: (j['balance'] as num?)?.toDouble() ?? 0,
        minBet: (j['min_bet'] as num?)?.toDouble() ?? 0,
        maxBet: (j['max_bet'] as num?)?.toDouble() ?? 0,
        currency: j['currency'] as String? ?? '',
        payout: j['payout'] as String? ?? '',
        hasHouse: j['has_house'] as bool? ?? false,
        round: BigSmallRound.fromJson(j['round'] as Map<String, dynamic>? ?? const {}),
        myBet: j['my_bet'] == null ? null : BigSmallMyBet.fromJson(j['my_bet'] as Map<String, dynamic>),
        lastResult: j['last_result'] == null ? null : BigSmallMyBet.fromJson(j['last_result'] as Map<String, dynamic>),
        bigBets: ((j['big_bets'] as List?) ?? []).map((e) => BigSmallBetSpot.fromJson(e as Map<String, dynamic>)).toList(),
        smallBets: ((j['small_bets'] as List?) ?? []).map((e) => BigSmallBetSpot.fromJson(e as Map<String, dynamic>)).toList(),
        trend40: BigSmallTrend.fromJson(j['trend40'] as Map<String, dynamic>? ?? const {}),
        trend10: BigSmallTrend.fromJson(j['trend10'] as Map<String, dynamic>? ?? const {}),
        trendText: j['trend_text'] as String? ?? '',
        recentResults: ((j['recent_results'] as List?) ?? []).map((e) => e.toString()).toList(),
        rankings: ((j['rankings'] as List?) ?? []).map((e) => BigSmallRank.fromJson(e as Map<String, dynamic>)).toList(),
        myRecords: ((j['my_records'] as List?) ?? []).map((e) => BigSmallRecord.fromJson(e as Map<String, dynamic>)).toList(),
        allRecords: ((j['all_records'] as List?) ?? []).map((e) => BigSmallRecord.fromJson(e as Map<String, dynamic>)).toList(),
        tasks: ((j['tasks'] as List?) ?? []).map((e) => BigSmallTask.fromJson(e as Map<String, dynamic>)).toList(),
        props: ((j['props'] as List?) ?? []).map((e) => BigSmallProp.fromJson(e as Map<String, dynamic>)).toList(),
        chat: ((j['chat'] as List?) ?? []).map((e) => BigSmallChat.fromJson(e as Map<String, dynamic>)).toList(),
        redpackets: ((j['redpackets'] as List?) ?? []).map((e) => BigSmallRedpacket.fromJson(e as Map<String, dynamic>)).toList(),
      );
}
