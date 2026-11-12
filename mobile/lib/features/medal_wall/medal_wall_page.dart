// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/medal_wall.dart';
import 'package:ptpatronus/features/common/stream_account.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:ptpatronus/core/err_text.dart';

part 'medalwallopsmixin.dart';
part 'medal_wall_widgets.dart';

class MedalWallPage extends StatefulWidget {
  const MedalWallPage({super.key, this.frames});

  /// 帧源注入缝（对齐 #195/#199/#201 的做法）：done 帧的账面要被页面吸收成
  /// `_error` / `_streamBanner`，不从这儿喂帧就永远只能在 widget 层外挂断言。
  final StreamFrames? frames;
  @override
  State<MedalWallPage> createState() => _MedalWallPageState();
}

class _MedalWallPageState extends State<MedalWallPage> with MedalWallOpsMixin {
  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _sub?.cancel();
    super.dispose();
  }

  List<MapEntry<int, String>> get _sites {
    final seen = <int, String>{};
    for (final m in _medals) {
      seen.putIfAbsent(m.siteId, () => m.siteName);
    }
    return seen.entries.toList();
  }

  List<Medal> get _siteFiltered {
    if (_selectedSiteIds.isEmpty) return _medals;
    return _medals.where((m) => _selectedSiteIds.contains(m.siteId)).toList();
  }

  List<Medal> get _filtered {
    final list = _siteFiltered;
    if (_filter == 'all') return list;
    if (_filter == 'wearing') return list.where((m) => m.wearStatus).toList();
    return list
        .where((m) => m.purchaseStatus.toLowerCase() == _filter)
        .toList();
  }

  int get _ownedCount => _siteFiltered.where((m) => m.isOwned).length;
  int get _availableCount => _siteFiltered.where((m) => m.isAvailable).length;
  int get _oosCount => _siteFiltered.where((m) => m.isOutOfStock).length;
  int get _wearingCount => _siteFiltered.where((m) => m.wearStatus).length;

  Color _statusColor(String s) => switch (s) {
    'OWNED' => const Color(0xFF2ECC71),
    'AVAILABLE' => const Color(0xFF3498DB),
    'OUT_OF_STOCK' => const Color(0xFFF56C6C),
    'SYNTHESIS' => const Color(0xFF9B59B6),
    _ => const Color(0xFF95A5A6), // NOT_ON_SALE/GRANT_ONLY/DISCONTINUED/UNKNOWN
  };

  String _statusLabel(String s) => switch (s) {
    'OWNED' => '已拥有',
    'AVAILABLE' => '可购买',
    'OUT_OF_STOCK' => '缺货',
    'NOT_ON_SALE' => '未开售',
    'GRANT_ONLY' => '仅授予',
    'SYNTHESIS' => '仅合成',
    'DISCONTINUED' => '已下架',
    _ => '未知',
  };

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '勋章墙',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : () => _load(refresh: true),
        ),
      ],
      filterDimensions: _filterDimensions(),
      siteFilter: PTSiteFilterConfig(
        selected: _selectedSiteIds,
        onChanged: (v) => setState(() => _selectedSiteIds = v),
        options: _sites
            .map((s) => PTFilterOption<Object?>(value: s.key, label: s.value))
            .toList(),
      ),
      filterOnClearAll: () => setState(() => _filter = 'all'),
      body: RefreshIndicator(
        onRefresh: () => _load(refresh: true),
        child: _loading && _medals.isEmpty
            ? const PTLoadingView()
            : _error != null
            ? PTEmptyState(text: _error!, onRetry: _load)
            : ListView(
                padding: const EdgeInsets.all(12),
                children: [
                  StreamNoticeBanner(text: _streamBanner),
                  if (_filtered.isEmpty)
                    PTEmptyState(
                      icon: Icons.military_tech_outlined,
                      text: '暂无勋章数据',
                      description: 'NexusPHP 站自动抓取；馒头无勋章页。若有勋章却为空，可能未登录或布局待校准',
                    )
                  else
                    LayoutBuilder(
                      builder: (context, c) {
                        final cols = ptAutoColumns(
                          c.maxWidth,
                          150,
                          minColumns: 3,
                        );
                        return GridView.count(
                          crossAxisCount: cols,
                          shrinkWrap: true,
                          physics: const NeverScrollableScrollPhysics(),
                          mainAxisSpacing: 10,
                          crossAxisSpacing: 10,
                          childAspectRatio: 0.82,
                          children: _filtered.map(_gridCard).toList(),
                        );
                      },
                    ),
                ],
              ),
      ),
    );
  }

  // 状态维度（收进通用筛选按钮）；站点筛选单独成按钮（siteFilter）。
  List<PTFilterDimension> _filterDimensions() {
    return [
      PTFilterDimension.choice(
        label: '状态',
        selected: _filter,
        isDefault: _filter == 'all',
        onChanged: (v) => setState(() => _filter = (v as String?) ?? 'all'),
        options: [
          PTFilterOption<Object?>(
            value: 'all',
            label: '全部 (${_siteFiltered.length})',
          ),
          PTFilterOption<Object?>(value: 'owned', label: '已拥有 ($_ownedCount)'),
          PTFilterOption<Object?>(
            value: 'available',
            label: '可购买 ($_availableCount)',
          ),
          PTFilterOption<Object?>(
            value: 'out_of_stock',
            label: '缺货 ($_oosCount)',
          ),
          PTFilterOption<Object?>(
            value: 'wearing',
            label: '佩戴中 ($_wearingCount)',
          ),
        ],
      ),
    ];
  }

}
