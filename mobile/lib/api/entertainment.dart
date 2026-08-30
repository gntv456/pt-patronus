import '../core/api_client.dart';
import 'site.dart';

part 'entertainment_bigsmall_models.dart';
part 'entertainment_flip_models.dart';
part 'entertainment_farm_models.dart';
part 'entertainment_bet_models.dart';

part 'entertainment_api.dart';

// Game 是某站点可玩的娱乐游戏（对齐后端 entertainment.LotteryGame）。
class Game {
  final String id;
  final String name;

  Game({required this.id, required this.name});

  factory Game.fromJson(Map<String, dynamic> j) => Game(
        id: j['id']?.toString() ?? '',
        name: j['name'] as String? ?? '',
      );
}

// PrizeOutcome 单次抽奖结果（连抽时 PlayResult.prizes 放多份）。
class PrizeOutcome {
  final int prizeId;
  final String prizeName;
  final String prizeType;
  final double amount;
  final String unitName;
  final bool isWin;

  PrizeOutcome({
    required this.prizeId,
    required this.prizeName,
    required this.prizeType,
    required this.amount,
    required this.unitName,
    required this.isWin,
  });

  factory PrizeOutcome.fromJson(Map<String, dynamic> j) => PrizeOutcome(
        prizeId: (j['prize_id'] as num?)?.toInt() ?? 0,
        prizeName: j['prize_name'] as String? ?? '',
        prizeType: j['prize_type'] as String? ?? '',
        amount: (j['amount'] as num?)?.toDouble() ?? 0,
        unitName: j['unit_name'] as String? ?? '',
        isWin: j['is_win'] as bool? ?? false,
      );
}

// PlayResult 是玩一次的结果（对齐后端 entertainment.PlayResult）。
class PlayResult {
  final bool success;
  final String message; // 中奖信息 / 失败原因
  final String prize; // 奖项名
  final int prizeId; // 奖品 id（九宫格动画定位用）
  final double amount; // 奖励数值（火花/魔力）
  final double balance; // 剩余余额
  final List<PrizeOutcome> prizes; // 连抽多次结果
  final Map<String, dynamic>? raw; // 原始响应（猜大小验证码 captcha_required 等用）

  PlayResult({
    required this.success,
    required this.message,
    required this.prize,
    required this.prizeId,
    required this.amount,
    required this.balance,
    required this.prizes,
    this.raw,
  });

  factory PlayResult.fromJson(Map<String, dynamic> j) => PlayResult(
        success: j['success'] as bool? ?? false,
        message: j['message'] as String? ?? '',
        prize: j['prize'] as String? ?? '',
        prizeId: (j['prize_id'] as num?)?.toInt() ?? 0,
        amount: (j['amount'] as num?)?.toDouble() ?? 0,
        balance: (j['balance'] as num?)?.toDouble() ?? 0,
        prizes: ((j['prizes'] as List?) ?? const []).map((e) => PrizeOutcome.fromJson(e as Map<String, dynamic>)).toList(),
        raw: j['raw'] is Map<String, dynamic> ? j['raw'] as Map<String, dynamic> : null,
      );
}

// GameStats 游戏统计（余额/剩余刮卡/今日次数）。
class GameStats {
  final double balance;
  final int cardNumber;
  final int todaySpins;
  final int maxDaily;
  final double costPerPlay;
  // LongPT 等站点可选字段
  final int discount;
  final int participants;
  final String startTime;
  final String endTime;
  final String openTime;

  GameStats({
    required this.balance,
    required this.cardNumber,
    required this.todaySpins,
    required this.maxDaily,
    required this.costPerPlay,
    this.discount = 0,
    this.participants = 0,
    this.startTime = '',
    this.endTime = '',
    this.openTime = '',
  });

  factory GameStats.fromJson(Map<String, dynamic> j) => GameStats(
        balance: (j['balance'] as num?)?.toDouble() ?? 0,
        cardNumber: (j['card_number'] as num?)?.toInt() ?? 0,
        todaySpins: (j['today_spins'] as num?)?.toInt() ?? 0,
        maxDaily: (j['max_daily'] as num?)?.toInt() ?? 0,
        costPerPlay: (j['cost_per_play'] as num?)?.toDouble() ?? 0,
        discount: (j['discount'] as num?)?.toInt() ?? 0,
        participants: (j['participants'] as num?)?.toInt() ?? 0,
        startTime: j['start_time'] as String? ?? '',
        endTime: j['end_time'] as String? ?? '',
        openTime: j['open_time'] as String? ?? '',
      );
}

// BuyOption 购卡档位（财神刮刮乐）。
class BuyOption {
  final int type;
  final int buyCount;
  final int giftCount;
  final double unitPrice;

  BuyOption({required this.type, required this.buyCount, required this.giftCount, required this.unitPrice});

  factory BuyOption.fromJson(Map<String, dynamic> j) => BuyOption(
        type: (j['type'] as num?)?.toInt() ?? 0,
        buyCount: (j['buy_count'] as num?)?.toInt() ?? 0,
        giftCount: (j['gift_count'] as num?)?.toInt() ?? 0,
        unitPrice: (j['unit_price'] as num?)?.toDouble() ?? 0,
      );
}

// PrizeDef 奖品展示（转盘扇区 / 今日奖池）。
class PrizeDef {
  final int id;
  final String name;
  final String icon;
  final String color;
  final String type;

  PrizeDef({required this.id, required this.name, required this.icon, required this.color, required this.type});

  factory PrizeDef.fromJson(Map<String, dynamic> j) => PrizeDef(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        icon: j['icon'] as String? ?? '',
        color: j['color'] as String? ?? '',
        type: j['type'] as String? ?? '',
      );
}

// MedalNode 六十四卦勋章树节点（太极→两仪→四象→八卦→六十四卦 金字塔）。
class MedalNode {
  final String medalId; // 二进制串（卦象）
  final String parentId;
  final String type; // taiji/liangyi/sixiang/bagua/liushisi
  final String name;
  final int level;
  final String imageUrl;
  final bool owned;
  final int needYang;
  final int needYin;
  final int bigCost; // 大勋章合成魔力（碎片合成为 0）

  MedalNode({
    required this.medalId,
    required this.parentId,
    required this.type,
    required this.name,
    required this.level,
    required this.imageUrl,
    required this.owned,
    required this.needYang,
    required this.needYin,
    required this.bigCost,
  });

  factory MedalNode.fromJson(Map<String, dynamic> j) => MedalNode(
        medalId: j['medal_id']?.toString() ?? '',
        parentId: j['parent_id']?.toString() ?? '',
        type: j['type'] as String? ?? '',
        name: j['name'] as String? ?? '',
        level: (j['level'] as num?)?.toInt() ?? 0,
        imageUrl: j['image_url'] as String? ?? '',
        owned: j['owned'] as bool? ?? false,
        needYang: (j['need_yang'] as num?)?.toInt() ?? 0,
        needYin: (j['need_yin'] as num?)?.toInt() ?? 0,
        bigCost: (j['big_cost'] as num?)?.toInt() ?? 0,
      );
}

// MedalShard 阳爻 / 阴爻碎片余额。
class MedalShard {
  final int yangYao;
  final int yinYao;

  MedalShard({required this.yangYao, required this.yinYao});

  factory MedalShard.fromJson(Map<String, dynamic> j) => MedalShard(
        yangYao: (j['yang_yao'] as num?)?.toInt() ?? 0,
        yinYao: (j['yin_yao'] as num?)?.toInt() ?? 0,
      );
}

// GameRecord 抽奖/抽取记录。
class GameRecord {
  final String prizeName;
  final String prizeType;
  final double amount;
  final double cost;
  final String createdAt;

  GameRecord({required this.prizeName, required this.prizeType, required this.amount, required this.cost, required this.createdAt});

  factory GameRecord.fromJson(Map<String, dynamic> j) => GameRecord(
        prizeName: j['prize_name'] as String? ?? '',
        prizeType: j['prize_type'] as String? ?? '',
        amount: (j['amount'] as num?)?.toDouble() ?? 0,
        cost: (j['cost'] as num?)?.toDouble() ?? 0,
        createdAt: j['created_at'] as String? ?? '',
      );
}
