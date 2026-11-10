// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 entertainment.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/api/entertainment.dart';

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
