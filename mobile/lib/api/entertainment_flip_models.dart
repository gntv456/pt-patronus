// 从 entertainment.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/api/entertainment.dart';

// FlipTier 翻面夺宝奖项档位（PandaPT）。
class FlipTier {
  final String id;
  final String name;
  final double reward;
  final String rewardType;
  final int count;
  final String color;
  FlipTier({required this.id, required this.name, required this.reward, required this.rewardType, required this.count, required this.color});
  factory FlipTier.fromJson(Map<String, dynamic> j) => FlipTier(
        id: j['id']?.toString() ?? '',
        name: j['name'] as String? ?? '',
        reward: (j['reward'] as num?)?.toDouble() ?? 0,
        rewardType: j['reward_type'] as String? ?? '',
        count: (j['count'] as num?)?.toInt() ?? 0,
        color: j['color'] as String? ?? '',
      );
}

// FlipTile 翻面夺宝单格（100 格之一）。
class FlipTile {
  final int index;
  final bool flipped;
  final String tierId;
  final String byUser;
  FlipTile({required this.index, required this.flipped, required this.tierId, required this.byUser});
  factory FlipTile.fromJson(Map<String, dynamic> j) => FlipTile(
        index: (j['index'] as num?)?.toInt() ?? 0,
        flipped: j['flipped'] as bool? ?? false,
        tierId: j['tier_id']?.toString() ?? '',
        byUser: j['by_user'] as String? ?? '',
      );
}

// FlipState 翻面夺宝全景（当前轮次/余额/100 格状态/奖项/记录）。
class FlipState {
  final int roundNumber;
  final double balance;
  final double costPerFlip;
  final int totalTiles;
  final int flippedCount;
  final List<FlipTier> tiers;
  final List<FlipTile> tiles;
  final List<GameRecord> history;
  FlipState({required this.roundNumber, required this.balance, required this.costPerFlip, required this.totalTiles, required this.flippedCount, required this.tiers, required this.tiles, required this.history});
  factory FlipState.fromJson(Map<String, dynamic> j) => FlipState(
        roundNumber: (j['round_number'] as num?)?.toInt() ?? 0,
        balance: (j['balance'] as num?)?.toDouble() ?? 0,
        costPerFlip: (j['cost_per_flip'] as num?)?.toDouble() ?? 0,
        totalTiles: (j['total_tiles'] as num?)?.toInt() ?? 100,
        flippedCount: (j['flipped_count'] as num?)?.toInt() ?? 0,
        tiers: ((j['tiers'] as List?) ?? []).map((e) => FlipTier.fromJson(e as Map<String, dynamic>)).toList(),
        tiles: ((j['tiles'] as List?) ?? []).map((e) => FlipTile.fromJson(e as Map<String, dynamic>)).toList(),
        history: ((j['history'] as List?) ?? []).map((e) => GameRecord.fromJson(e as Map<String, dynamic>)).toList(),
      );
}
