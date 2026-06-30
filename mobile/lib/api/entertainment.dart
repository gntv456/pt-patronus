import '../core/api_client.dart';

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

  PlayResult({
    required this.success,
    required this.message,
    required this.prize,
    required this.prizeId,
    required this.amount,
    required this.balance,
    required this.prizes,
  });

  factory PlayResult.fromJson(Map<String, dynamic> j) => PlayResult(
        success: j['success'] as bool? ?? false,
        message: j['message'] as String? ?? '',
        prize: j['prize'] as String? ?? '',
        prizeId: (j['prize_id'] as num?)?.toInt() ?? 0,
        amount: (j['amount'] as num?)?.toDouble() ?? 0,
        balance: (j['balance'] as num?)?.toDouble() ?? 0,
        prizes: ((j['prizes'] as List?) ?? const []).map((e) => PrizeOutcome.fromJson(e as Map<String, dynamic>)).toList(),
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

// SynthFragment 卡牌合成单个碎片库存。
class SynthFragment {
  final String name;
  final String image;
  final int current;
  final int required;
  final bool isRare;

  SynthFragment({required this.name, required this.image, required this.current, required this.required, required this.isRare});

  factory SynthFragment.fromJson(Map<String, dynamic> j) => SynthFragment(
        name: j['name'] as String? ?? '',
        image: j['image'] as String? ?? '',
        current: (j['current'] as num?)?.toInt() ?? 0,
        required: (j['required'] as num?)?.toInt() ?? 0,
        isRare: j['is_rare'] as bool? ?? false,
      );
}

// SynthesisTarget 卡牌合成目标。
class SynthesisTarget {
  final int id;
  final String name;
  final String image;
  final int got;
  final int need;
  final List<SynthFragment> fragments;
  final int drawPrice;
  final int tenPrice;
  final int hundredPrice;
  final bool canSynthesize;
  final String pity;

  SynthesisTarget({required this.id, required this.name, required this.image, required this.got, required this.need, required this.fragments, required this.drawPrice, required this.tenPrice, required this.hundredPrice, required this.canSynthesize, required this.pity});

  factory SynthesisTarget.fromJson(Map<String, dynamic> j) => SynthesisTarget(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        image: j['image'] as String? ?? '',
        got: (j['got'] as num?)?.toInt() ?? 0,
        need: (j['need'] as num?)?.toInt() ?? 0,
        fragments: ((j['fragments'] as List?) ?? const []).map((e) => SynthFragment.fromJson(e as Map<String, dynamic>)).toList(),
        drawPrice: (j['draw_price'] as num?)?.toInt() ?? 0,
        tenPrice: (j['ten_price'] as num?)?.toInt() ?? 0,
        hundredPrice: (j['hundred_price'] as num?)?.toInt() ?? 0,
        canSynthesize: j['can_synthesize'] as bool? ?? false,
        pity: j['pity'] as String? ?? '',
      );
}

// FarmItem 农场单项（NovaHD 魔力农场：农作物 / 动物）。
class FarmItem {
  final int id;
  final String name;
  final String icon;
  final String section; // crop | animal
  final int growHours;
  final double price;
  final int expireDays;
  final String status; // idle | growing | ready
  final String remainTime;

  FarmItem({
    required this.id,
    required this.name,
    required this.icon,
    required this.section,
    required this.growHours,
    required this.price,
    required this.expireDays,
    required this.status,
    required this.remainTime,
  });

  factory FarmItem.fromJson(Map<String, dynamic> j) => FarmItem(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        icon: j['icon'] as String? ?? '',
        section: j['section'] as String? ?? '',
        growHours: (j['grow_hours'] as num?)?.toInt() ?? 0,
        price: (j['price'] as num?)?.toDouble() ?? 0,
        expireDays: (j['expire_days'] as num?)?.toInt() ?? 0,
        status: j['status'] as String? ?? '',
        remainTime: j['remain_time'] as String? ?? '',
      );
}

// StorageItem 仓库物品（含出售 key 与市场价）。
class StorageItem {
  final String key;
  final String name;
  final String icon;
  final int count;
  final double marketPrice;

  StorageItem({required this.key, required this.name, required this.icon, required this.count, required this.marketPrice});

  factory StorageItem.fromJson(Map<String, dynamic> j) => StorageItem(
        key: j['key']?.toString() ?? '',
        name: j['name'] as String? ?? '',
        icon: j['icon'] as String? ?? '',
        count: (j['count'] as num?)?.toInt() ?? 0,
        marketPrice: (j['market_price'] as num?)?.toDouble() ?? 0,
      );
}

// MarketItem 菜市场行情（24h ±20% 波动）。
class MarketItem {
  final int id;
  final String name;
  final String section;
  final double marketPrice;

  MarketItem({required this.id, required this.name, required this.section, required this.marketPrice});

  factory MarketItem.fromJson(Map<String, dynamic> j) => MarketItem(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        section: j['section'] as String? ?? '',
        marketPrice: (j['market_price'] as num?)?.toDouble() ?? 0,
      );
}

// FarmState 农场全景：余额 + 种植区 + 养殖区 + 仓库 + 菜市场。
class FarmState {
  final double balance;
  final String farmName;
  final List<FarmItem> crops;
  final List<FarmItem> animals;
  final List<StorageItem> warehouse;
  final List<MarketItem> market;

  FarmState({required this.balance, required this.farmName, required this.crops, required this.animals, required this.warehouse, required this.market});

  factory FarmState.fromJson(Map<String, dynamic> j) => FarmState(
        balance: (j['balance'] as num?)?.toDouble() ?? 0,
        farmName: j['farm_name'] as String? ?? '',
        crops: ((j['crops'] as List?) ?? const []).map((e) => FarmItem.fromJson(e as Map<String, dynamic>)).toList(),
        animals: ((j['animals'] as List?) ?? const []).map((e) => FarmItem.fromJson(e as Map<String, dynamic>)).toList(),
        warehouse: ((j['warehouse'] as List?) ?? const []).map((e) => StorageItem.fromJson(e as Map<String, dynamic>)).toList(),
        market: ((j['market'] as List?) ?? const []).map((e) => MarketItem.fromJson(e as Map<String, dynamic>)).toList(),
      );
}

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

class EntertainmentApi {
  final _api = ApiClient();

  Future<List<Game>> games(int siteId) async {
    final r = await _api.get('/entertainment/$siteId/games');
    final list = (r['games'] as List?) ?? const [];
    return list.map((e) => Game.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<PlayResult> play(int siteId, String game) async {
    final r = await _api.post('/entertainment/$siteId/play', {'game': game});
    return PlayResult.fromJson(r);
  }

  /// 连抽（仅 LongPT 大转盘等支持）
  Future<PlayResult> playMulti(int siteId, String game, int count) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/play-multi', {'count': count});
    return PlayResult.fromJson(r);
  }

  Future<GameStats> stats(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/stats');
    return GameStats.fromJson(r);
  }

  /// 奖品展示（转盘扇区 / 今日奖池）
  Future<List<PrizeDef>> prizes(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/prizes');
    final list = (r['prizes'] as List?) ?? const [];
    return list.map((e) => PrizeDef.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<List<BuyOption>> buyOptions(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/buy-options');
    final list = (r['options'] as List?) ?? const [];
    return list.map((e) => BuyOption.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<PlayResult> buy(int siteId, String game, int optionType) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/buy', {'option_type': optionType});
    return PlayResult.fromJson(r);
  }

  Future<List<GameRecord>> history(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/history');
    final list = (r['records'] as List?) ?? const [];
    return list.map((e) => GameRecord.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<List<SynthesisTarget>> syntheses(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/syntheses');
    final list = (r['targets'] as List?) ?? const [];
    return list.map((e) => SynthesisTarget.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<PlayResult> draw(int siteId, String game, int synthesisId, int count) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/draw', {'synthesis_id': synthesisId, 'count': count});
    return PlayResult.fromJson(r);
  }

  Future<PlayResult> synthesize(int siteId, String game, int synthesisId) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/synthesize', {'synthesis_id': synthesisId});
    return PlayResult.fromJson(r);
  }

  /// 勋章金字塔（六十四卦）
  Future<List<MedalNode>> medalTree(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/medal-tree');
    final list = (r['nodes'] as List?) ?? const [];
    return list.map((e) => MedalNode.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 阳爻/阴爻碎片余额
  Future<MedalShard> medalShards(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/medal-shards');
    return MedalShard.fromJson(r['shards'] as Map<String, dynamic>? ?? const {});
  }

  /// 碎片合成（太极/两仪/四象）
  Future<PlayResult> composeMedal(int siteId, String game, String medalId) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/compose', {'medal_id': medalId});
    return PlayResult.fromJson(r);
  }

  /// 大勋章合成（八卦/六十四卦，耗魔力+子勋章）
  Future<PlayResult> composeBigMedal(int siteId, String game, String medalId) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/compose-big', {'medal_id': medalId});
    return PlayResult.fromJson(r);
  }

  /// 阴阳转换（yiYang>0 阴→阳，<0 阳→阴）
  Future<PlayResult> convertShard(int siteId, String game, int yiYang) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/convert-shard', {'yi_yang': yiYang});
    return PlayResult.fromJson(r);
  }

  /// 农场全景（NovaHD 魔力农场）
  Future<FarmState> farm(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/farm');
    return FarmState.fromJson(r);
  }

  /// 种植/养殖（section=crop→plant / animal→breed）
  Future<PlayResult> plant(int siteId, String game, String section, int itemId) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/plant', {'section': section, 'item_id': itemId});
    return PlayResult.fromJson(r);
  }

  /// 收获单个
  Future<PlayResult> harvest(int siteId, String game, String section, int itemId) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/harvest', {'section': section, 'item_id': itemId});
    return PlayResult.fromJson(r);
  }

  /// 一键收获
  Future<PlayResult> harvestAll(int siteId, String game) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/harvest-all', {});
    return PlayResult.fromJson(r);
  }

  /// 出售仓库物品
  Future<PlayResult> sell(int siteId, String game, String key) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/sell', {'key': key});
    return PlayResult.fromJson(r);
  }

  /// 下注/选号（猜大小/骰宝/双色球/翻面/Betting/红包）
  Future<PlayResult> bet(int siteId, String game, BetRequest req) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/bet', req.toJson());
    return PlayResult.fromJson(r);
  }

  /// 二十一点动作（new/hit/stand/double）
  Future<BlackjackResult> blackjack(int siteId, String game, String action) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/blackjack', {'action': action});
    return BlackjackResult.fromJson(r);
  }
}
