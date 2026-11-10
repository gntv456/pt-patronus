// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 总览仪表盘设置面板（从 dashboard_page.dart 拆出，行为不变）。
// 底部弹层：展示项开关 + 站点选择 + 紧凑模式等，保存经 onSave 回调。
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/dashboard.dart';
import 'package:ptpatronus/features/dashboard/total_dashboard_config.dart';
import 'package:ptpatronus/theme/glass.dart';

class DashboardSettingsSheet extends StatefulWidget {
  final TotalDashboardConfig initial;
  final List<SiteStatItem> sites;
  final Future<void> Function(TotalDashboardConfig) onSave;

  const DashboardSettingsSheet({
    super.key,
    required this.initial,
    required this.sites,
    required this.onSave,
  });

  @override
  State<DashboardSettingsSheet> createState() =>
      _DashboardSettingsSheetState();
}

class _DashboardSettingsSheetState
    extends State<DashboardSettingsSheet> {
  late TotalDashboardConfig _config = widget.initial;

  @override
  Widget build(BuildContext context) {
    final maxSites = widget.sites.isEmpty ? 1 : widget.sites.length;
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.fromLTRB(
          16,
          10,
          16,
          16 + MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.sizeOf(context).height * 0.86,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  const Icon(Icons.tune, color: PTColors.primary, size: 20),
                  const SizedBox(width: 8),
                  const Expanded(
                    child: Text(
                      '总仪表盘设置',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 16,
                      ),
                    ),
                  ),
                  IconButton(
                    tooltip: '关闭',
                    onPressed: () => Navigator.pop(context),
                    icon: const Icon(Icons.close),
                  ),
                ],
              ),
              const SizedBox(height: 4),
              Expanded(
                child: ListView(
                  children: [
                    _section('显示内容'),
                    _switch(
                      '总站点数',
                      _config.showTotalSites,
                      (v) => _set(_config.copyWith(showTotalSites: v)),
                    ),
                    _switch(
                      '发布数',
                      _config.showUploadCount,
                      (v) => _set(_config.copyWith(showUploadCount: v)),
                    ),
                    _switch(
                      '上传量',
                      _config.showUpload,
                      (v) => _set(_config.copyWith(showUpload: v)),
                    ),
                    _switch(
                      '下载量',
                      _config.showDownload,
                      (v) => _set(_config.copyWith(showDownload: v)),
                    ),
                    _switch(
                      '做种数',
                      _config.showSeedingCount,
                      (v) => _set(_config.copyWith(showSeedingCount: v)),
                    ),
                    _switch(
                      '做种体积',
                      _config.showSeedingVolume,
                      (v) => _set(_config.copyWith(showSeedingVolume: v)),
                    ),
                    _switch(
                      '积分值/魔力值',
                      _config.showBonus,
                      (v) => _set(_config.copyWith(showBonus: v)),
                    ),
                    _switch(
                      '每小时积分',
                      _config.showBonusPerHour,
                      (v) => _set(_config.copyWith(showBonusPerHour: v)),
                    ),
                    _switch(
                      '分享率',
                      _config.showRatio,
                      (v) => _set(_config.copyWith(showRatio: v)),
                    ),
                    _section('排行榜'),
                    _switch(
                      '显示冠军排行',
                      _config.showRankPanel,
                      (v) => _set(_config.copyWith(showRankPanel: v)),
                    ),
                    _section('站点列表'),
                    _switch(
                      '显示站点详情列表',
                      _config.showSiteList,
                      (v) => _set(_config.copyWith(showSiteList: v)),
                    ),
                    _countSlider(maxSites),
                    _switch(
                      '显示站点名称',
                      _config.showSiteName,
                      (v) => _set(_config.copyWith(showSiteName: v)),
                    ),
                    _switch(
                      '显示用户名',
                      _config.showUsername,
                      (v) => _set(_config.copyWith(showUsername: v)),
                    ),
                    _switch(
                      '模糊站点信息（站名/Logo）',
                      _config.blurSensitiveInfo,
                      (v) => _set(_config.copyWith(blurSensitiveInfo: v)),
                    ),
                    _switch(
                      '模糊用户信息（用户名/UID/等级）',
                      _config.blurUserInfo,
                      (v) => _set(_config.copyWith(blurUserInfo: v)),
                    ),
                    _section('样式'),
                    _switch(
                      '紧凑模式',
                      _config.compactMode,
                      (v) => _set(_config.copyWith(compactMode: v)),
                    ),
                    if (widget.sites.isNotEmpty) ...[
                      _section('显示站点'),
                      _sitePickerSummary(),
                    ],
                  ],
                ),
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  TextButton(
                    onPressed: () => _set(TotalDashboardConfig.defaults),
                    child: const Text('恢复默认'),
                  ),
                  const Spacer(),
                  TextButton(
                    onPressed: () => Navigator.pop(context),
                    child: const Text('取消'),
                  ),
                  const SizedBox(width: 8),
                  FilledButton(
                    onPressed: () async {
                      final navigator = Navigator.of(context);
                      await widget.onSave(_config);
                      if (mounted) navigator.pop();
                    },
                    child: const Text('确定'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _set(TotalDashboardConfig config) {
    setState(() => _config = config);
  }

  Widget _section(String title) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 10, 4, 2),
      child: Text(
        title,
        style: const TextStyle(
          color: PTColors.primary,
          fontWeight: FontWeight.w700,
          fontSize: 13,
        ),
      ),
    );
  }

  Widget _switch(String label, bool value, ValueChanged<bool> onChanged) {
    return SwitchListTile(
      value: value,
      onChanged: onChanged,
      title: Text(label, style: const TextStyle(fontSize: 14)),
      dense: true,
      visualDensity: VisualDensity.compact,
      contentPadding: const EdgeInsets.symmetric(horizontal: 4),
    );
  }

  Widget _countSlider(int maxSites) {
    final value = _config.siteDataLimit.clamp(0, maxSites).toInt();
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 4),
      child: Column(
        children: [
          Row(
            children: [
              const Expanded(
                child: Text('各站点数据数量', style: TextStyle(fontSize: 14)),
              ),
              Text(
                value == 0 ? '全部' : '$value',
                style: const TextStyle(
                  color: PTColors.primary,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
          Slider(
            value: value.toDouble(),
            min: 0,
            max: maxSites.toDouble(),
            divisions: maxSites,
            onChanged: _config.showSiteList
                ? (v) => _set(_config.copyWith(siteDataLimit: v.round()))
                : null,
          ),
        ],
      ),
    );
  }

  Widget _sitePickerSummary() {
    final selected = _config.selectedSiteIds;
    final label = selected.contains(-1)
        ? '全不选'
        : selected.isEmpty
        ? '全部站点'
        : '已选择 ${selected.length} 个站点';
    return ListTile(
      dense: true,
      contentPadding: const EdgeInsets.symmetric(horizontal: 4),
      title: const Text('显示站点', style: TextStyle(fontSize: 14)),
      subtitle: Text(label, maxLines: 1, overflow: TextOverflow.ellipsis),
      trailing: const Icon(Icons.chevron_right),
      onTap: _openSitePicker,
    );
  }

  void _openSitePicker() {
    final selected = _config.selectedSiteIds.toSet();
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => StatefulBuilder(
        builder: (context, setSheetState) {
          return SafeArea(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 10, 16, 16),
              child: SizedBox(
                height: MediaQuery.sizeOf(context).height * 0.72,
                child: Column(
                  children: [
                    Row(
                      children: [
                        const Expanded(
                          child: Text(
                            '选择显示站点',
                            style: TextStyle(
                              fontWeight: FontWeight.w700,
                              fontSize: 16,
                            ),
                          ),
                        ),
                        TextButton(
                          onPressed: () {
                            selected.clear();
                            setSheetState(() {});
                          },
                          child: const Text('全选'),
                        ),
                        TextButton(
                          onPressed: () {
                            selected
                              ..clear()
                              ..add(-1);
                            setSheetState(() {});
                          },
                          child: const Text('全不选'),
                        ),
                      ],
                    ),
                    const SizedBox(height: 6),
                    Expanded(
                      child: ListView.builder(
                        itemCount: widget.sites.length,
                        itemBuilder: (_, i) {
                          final site = widget.sites[i];
                          final none = selected.contains(-1);
                          final checked =
                              !none &&
                              (selected.isEmpty ||
                                  selected.contains(site.siteId));
                          return CheckboxListTile(
                            dense: true,
                            visualDensity: VisualDensity.compact,
                            contentPadding: EdgeInsets.zero,
                            value: checked,
                            title: Text(
                              site.siteName,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                            ),
                            onChanged: (v) {
                              if (selected.contains(-1)) selected.clear();
                              if (selected.isEmpty) {
                                selected.addAll(
                                  widget.sites.map((s) => s.siteId),
                                );
                              }
                              if (v == true) {
                                selected.add(site.siteId);
                              } else {
                                selected.remove(site.siteId);
                              }
                              setSheetState(() {});
                            },
                          );
                        },
                      ),
                    ),
                    FilledButton(
                      onPressed: () {
                        final ids = selected.contains(-1)
                            ? <int>[-1]
                            : selected.length == widget.sites.length
                            ? <int>[]
                            : (selected.toList()..sort());
                        _set(_config.copyWith(selectedSiteIds: ids));
                        Navigator.pop(context);
                      },
                      child: const Text('确定'),
                    ),
                  ],
                ),
              ),
            ),
          );
        },
      ),
    );
  }
}

