// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 entertainment.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/api/entertainment.dart';

class EntertainmentApi {
  final _api = ApiClient();

  /// 娱乐页站点列表（含各站已适配娱乐项目数 game_count，独立于通用 /sites）。
  Future<List<Site>> sites() async {
    final r = await _api.get('/entertainment');
    final list = (r['sites'] as List?) ?? const [];
    return list.map((e) => Site.fromJson(e as Map<String, dynamic>)).toList();
  }

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

  /// 赛事押注：列竞猜主题
  Future<List<BetEvent>> events(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/events');
    final list = (r['events'] as List?) ?? const [];
    return list.map((e) => BetEvent.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 赛事押注：某主题盘口（选项+赔率）
  Future<BetEventDetail> eventDetail(int siteId, String game, int eventId) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/events/$eventId');
    return BetEventDetail.fromJson(r);
  }

  /// 翻面夺宝全景（PandaPT：当前轮次/余额/100 格状态/奖项/记录）
  Future<FlipState> flipState(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/flip-state');
    return FlipState.fromJson(r);
  }

  /// 猜大小全景快照（走势/排行/记录/当前局/任务/道具/聊天/红包）
  Future<BigSmallState> bigsmallState(int siteId, String game) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/bigsmall-state');
    return BigSmallState.fromJson(r);
  }

  /// 猜大小拉新聊天（since：好学 last_id；熊猫忽略）
  Future<List<BigSmallChat>> bigsmallChat(int siteId, String game, int since) async {
    final r = await _api.get('/entertainment/$siteId/games/$game/bigsmall-chat?since=$since');
    final list = (r['chat'] as List?) ?? const [];
    return list.map((e) => BigSmallChat.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 猜大小发聊天
  Future<PlayResult> bigsmallSendChat(int siteId, String game, String message) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/bigsmall-chat', {'message': message});
    return PlayResult.fromJson(r);
  }

  /// 猜大小任务（claim/reward/refund）
  Future<PlayResult> bigsmallTask(int siteId, String game, String action, int taskId) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/bigsmall-task', {'action': action, 'task_id': taskId});
    return PlayResult.fromJson(r);
  }

  /// 猜大小使用道具
  Future<PlayResult> bigsmallItem(int siteId, String game, String tag) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/bigsmall-item', {'tag': tag});
    return PlayResult.fromJson(r);
  }

  /// 猜大小红包（send/claim/info）
  Future<PlayResult> bigsmallRedpacket(int siteId, String game, String action, Map<String, String> extra) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/bigsmall-redpacket', {'action': action, 'extra': extra});
    return PlayResult.fromJson(r);
  }

  /// 二十一点动作（new/hit/stand/double）
  Future<BlackjackResult> blackjack(int siteId, String game, String action) async {
    final r = await _api.post('/entertainment/$siteId/games/$game/blackjack', {'action': action});
    return BlackjackResult.fromJson(r);
  }
}
