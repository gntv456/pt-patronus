// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 总览仪表盘展示配置（模型独立文件，dashboard_page 与设置面板/分享卡共用）。
// 持久化在 SharedPreferences（key 见 [_storageKey]）；JSON 字段与 Web 端偏好对齐。
class TotalDashboardConfig {
  final bool showTotalSites;
  final bool showUploadCount;
  final bool showUpload;
  final bool showDownload;
  final bool showSeedingCount;
  final bool showSeedingVolume;
  final bool showBonus;
  final bool showBonusPerHour;
  final bool showRatio;
  final bool showRankPanel;
  final bool showSiteList;
  final bool showSiteName;
  final bool showUsername;
  final bool blurSensitiveInfo;
  final bool blurUserInfo;
  final bool compactMode;
  final int siteDataLimit;
  final List<int> selectedSiteIds;

  const TotalDashboardConfig({
    this.showTotalSites = true,
    this.showUploadCount = true,
    this.showUpload = true,
    this.showDownload = true,
    this.showSeedingCount = true,
    this.showSeedingVolume = true,
    this.showBonus = true,
    this.showBonusPerHour = true,
    this.showRatio = true,
    this.showRankPanel = true,
    this.showSiteList = true,
    this.showSiteName = true,
    this.showUsername = true,
    this.blurSensitiveInfo = false,
    this.blurUserInfo = false,
    this.compactMode = true,
    this.siteDataLimit = 0,
    this.selectedSiteIds = const [],
  });

  static const defaults = TotalDashboardConfig();

  bool isSiteSelected(int siteId) =>
      selectedSiteIds.isEmpty || selectedSiteIds.contains(siteId);

  TotalDashboardConfig copyWith({
    bool? showTotalSites,
    bool? showUploadCount,
    bool? showUpload,
    bool? showDownload,
    bool? showSeedingCount,
    bool? showSeedingVolume,
    bool? showBonus,
    bool? showBonusPerHour,
    bool? showRatio,
    bool? showRankPanel,
    bool? showSiteList,
    bool? showSiteName,
    bool? showUsername,
    bool? blurSensitiveInfo,
    bool? blurUserInfo,
    bool? compactMode,
    int? siteDataLimit,
    List<int>? selectedSiteIds,
  }) {
    return TotalDashboardConfig(
      showTotalSites: showTotalSites ?? this.showTotalSites,
      showUploadCount: showUploadCount ?? this.showUploadCount,
      showUpload: showUpload ?? this.showUpload,
      showDownload: showDownload ?? this.showDownload,
      showSeedingCount: showSeedingCount ?? this.showSeedingCount,
      showSeedingVolume: showSeedingVolume ?? this.showSeedingVolume,
      showBonus: showBonus ?? this.showBonus,
      showBonusPerHour: showBonusPerHour ?? this.showBonusPerHour,
      showRatio: showRatio ?? this.showRatio,
      showRankPanel: showRankPanel ?? this.showRankPanel,
      showSiteList: showSiteList ?? this.showSiteList,
      showSiteName: showSiteName ?? this.showSiteName,
      showUsername: showUsername ?? this.showUsername,
      blurSensitiveInfo: blurSensitiveInfo ?? this.blurSensitiveInfo,
      blurUserInfo: blurUserInfo ?? this.blurUserInfo,
      compactMode: compactMode ?? this.compactMode,
      siteDataLimit: siteDataLimit ?? this.siteDataLimit,
      selectedSiteIds: selectedSiteIds ?? this.selectedSiteIds,
    );
  }

  Map<String, dynamic> toJson() => {
    'show_total_sites': showTotalSites,
    'show_upload_count': showUploadCount,
    'show_upload': showUpload,
    'show_download': showDownload,
    'show_seeding_count': showSeedingCount,
    'show_seeding_volume': showSeedingVolume,
    'show_bonus': showBonus,
    'show_bonus_per_hour': showBonusPerHour,
    'show_ratio': showRatio,
    'show_rank_panel': showRankPanel,
    'show_site_list': showSiteList,
    'show_site_name': showSiteName,
    'show_username': showUsername,
    'blur_sensitive_info': blurSensitiveInfo,
    'blur_user_info': blurUserInfo,
    'compact_mode': compactMode,
    'site_data_limit': siteDataLimit,
    'selected_site_ids': selectedSiteIds,
  };

  factory TotalDashboardConfig.fromJson(Map<String, dynamic> json) {
    bool b(String key, bool fallback) =>
        json[key] is bool ? json[key] as bool : fallback;
    int i(String key, int fallback) =>
        json[key] is num ? (json[key] as num).toInt() : fallback;
    final ids = ((json['selected_site_ids'] as List?) ?? const [])
        .map((e) => e is num ? e.toInt() : int.tryParse('$e'))
        .whereType<int>()
        .toList();
    return TotalDashboardConfig(
      showTotalSites: b('show_total_sites', true),
      showUploadCount: b('show_upload_count', true),
      showUpload: b('show_upload', true),
      showDownload: b('show_download', true),
      showSeedingCount: b('show_seeding_count', true),
      showSeedingVolume: b('show_seeding_volume', true),
      showBonus: b('show_bonus', true),
      showBonusPerHour: b('show_bonus_per_hour', true),
      showRatio: b('show_ratio', true),
      showRankPanel: b('show_rank_panel', true),
      showSiteList: b('show_site_list', true),
      showSiteName: b('show_site_name', true),
      showUsername: b('show_username', true),
      blurSensitiveInfo: b('blur_sensitive_info', false),
      blurUserInfo: b('blur_user_info', false),
      compactMode: b('compact_mode', true),
      siteDataLimit: i('site_data_limit', 0),
      selectedSiteIds: ids,
    );
  }
}
