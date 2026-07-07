import '../core/api_client.dart';

// 容错数值转换：后端字段可能缺省/偶发为字符串，统一走 helper，避免单字段类型异常致整页 fromJson 抛错（审计 P1）。
int _i(dynamic v) =>
    v == null ? 0 : (v is num ? v.toInt() : (int.tryParse('$v') ?? 0));
double _n(dynamic v) =>
    v == null ? 0 : (v is num ? v.toDouble() : (double.tryParse('$v') ?? 0));

// 对齐后端 chain.SiteStatItem：每站点最新统计。
class SiteStatItem {
  final int siteId;
  final String siteName;
  final String username; // 站点用户名（导出海报身份行，best-effort）
  final String userId; // 站点用户 ID
  final String email; // 账号邮箱（best-effort，可能为空/部分隐藏）
  final String level;
  final int uploaded;
  final int downloaded;
  final double ratio;
  final int trueUploaded;
  final int trueDownloaded;
  final double trueRatio;
  final int seeding;
  final int leeching;
  final int seedingSize;
  final int uploads;
  final int uniqueGroups;
  final int perfectFlac;
  final int invited;
  final double bonus;
  final double bonusPerHour;
  final double seedingBonus;
  final double seedingBonusPerHour;
  final double rescueSeedingBonusPerHour;
  final int claimCount;
  final int maxClaim;
  final String joinDate;
  final String lastAccessAt;
  final String updatedAt;

  SiteStatItem({
    required this.siteId,
    required this.siteName,
    required this.username,
    required this.userId,
    required this.email,
    required this.level,
    required this.uploaded,
    required this.downloaded,
    required this.ratio,
    required this.trueUploaded,
    required this.trueDownloaded,
    required this.trueRatio,
    required this.seeding,
    required this.leeching,
    required this.seedingSize,
    required this.uploads,
    required this.uniqueGroups,
    required this.perfectFlac,
    required this.invited,
    required this.bonus,
    required this.bonusPerHour,
    required this.seedingBonus,
    required this.seedingBonusPerHour,
    required this.rescueSeedingBonusPerHour,
    required this.claimCount,
    required this.maxClaim,
    required this.joinDate,
    required this.lastAccessAt,
    required this.updatedAt,
  });

  factory SiteStatItem.fromJson(Map<String, dynamic> j) => SiteStatItem(
    siteId: _i(j['site_id']),
    siteName: j['site_name'] as String? ?? '',
    username: j['username'] as String? ?? '',
    userId: j['user_id'] as String? ?? '',
    email: j['email'] as String? ?? '',
    level: j['level'] as String? ?? '',
    uploaded: _i(j['uploaded']),
    downloaded: _i(j['downloaded']),
    ratio: _n(j['ratio']),
    trueUploaded: _i(j['true_uploaded']),
    trueDownloaded: _i(j['true_downloaded']),
    trueRatio: _n(j['true_ratio']),
    seeding: _i(j['seeding']),
    leeching: _i(j['leeching']),
    seedingSize: _i(j['seeding_size']),
    uploads: _i(j['uploads']),
    uniqueGroups: _i(j['unique_groups']),
    perfectFlac: _i(j['perfect_flac']),
    invited: _i(j['invited']),
    bonus: _n(j['bonus']),
    bonusPerHour: _n(j['bonus_per_hour']),
    seedingBonus: _n(j['seeding_bonus']),
    seedingBonusPerHour: _n(j['seeding_bonus_per_hour']),
    rescueSeedingBonusPerHour: _n(j['rescue_seeding_bonus_per_hour']),
    claimCount: _i(j['claim_count']),
    maxClaim: _i(j['max_claim']),
    joinDate: j['join_date'] as String? ?? '',
    lastAccessAt: j['last_access_at'] as String? ?? '',
    updatedAt: j['updated_at'] as String? ?? '',
  );
}

// DashboardOverview 是仪表盘顶部 4 张卡片的数据源（跨站汇总 + 计数）。
class DashboardOverview {
  final int uploaded;
  final int downloaded;
  final int trueUploaded;
  final int trueDownloaded;
  final double trueRatio;
  final int seeding;
  final int leeching;
  final int seedingSize;
  final int uniqueGroups;
  final int perfectFlac;
  final int invited;
  final double bonus;
  final double bonusPerHour;
  final double seedingBonus;
  final double seedingBonusPerHour;
  final double rescueSeedingBonusPerHour;
  final int claimCount;
  final int maxClaim;
  final List<SiteStatItem> sites;
  final int siteCount;
  final int downloaderCount;

  DashboardOverview({
    required this.uploaded,
    required this.downloaded,
    required this.trueUploaded,
    required this.trueDownloaded,
    required this.trueRatio,
    required this.seeding,
    required this.leeching,
    required this.seedingSize,
    required this.uniqueGroups,
    required this.perfectFlac,
    required this.invited,
    required this.bonus,
    required this.bonusPerHour,
    required this.seedingBonus,
    required this.seedingBonusPerHour,
    required this.rescueSeedingBonusPerHour,
    required this.claimCount,
    required this.maxClaim,
    required this.sites,
    required this.siteCount,
    required this.downloaderCount,
  });

  factory DashboardOverview.fromJson(Map<String, dynamic> j) {
    final totals = (j['totals'] as Map<String, dynamic>?) ?? const {};
    return DashboardOverview(
      uploaded: _i(totals['uploaded']),
      downloaded: _i(totals['downloaded']),
      trueUploaded: _i(totals['true_uploaded']),
      trueDownloaded: _i(totals['true_downloaded']),
      trueRatio: _n(totals['true_ratio']),
      seeding: _i(totals['seeding']),
      leeching: _i(totals['leeching']),
      seedingSize: _i(totals['seeding_size']),
      uniqueGroups: _i(totals['unique_groups']),
      perfectFlac: _i(totals['perfect_flac']),
      invited: _i(totals['invited']),
      bonus: _n(totals['bonus']),
      bonusPerHour: _n(totals['bonus_per_hour']),
      seedingBonus: _n(totals['seeding_bonus']),
      seedingBonusPerHour: _n(totals['seeding_bonus_per_hour']),
      rescueSeedingBonusPerHour: _n(totals['rescue_seeding_bonus_per_hour']),
      claimCount: _i(totals['claim_count']),
      maxClaim: _i(totals['max_claim']),
      sites: ((j['sites'] as List?) ?? const [])
          .map((e) => SiteStatItem.fromJson(e as Map<String, dynamic>))
          .toList(),
      siteCount: (j['site_count'] as num?)?.toInt() ?? 0,
      downloaderCount: (j['downloader_count'] as num?)?.toInt() ?? 0,
    );
  }
}

// DashboardTrendPoint 是某一天的跨站聚合趋势点（对齐后端 TrendPoint）。
class DashboardTrendPoint {
  final String day; // "2026-06-19"
  final int uploaded;
  final int downloaded;
  final int seeding;
  final int seedingSize;
  final double bonus;
  final double ratio;

  DashboardTrendPoint({
    required this.day,
    required this.uploaded,
    required this.downloaded,
    required this.seeding,
    required this.seedingSize,
    required this.bonus,
    required this.ratio,
  });

  factory DashboardTrendPoint.fromJson(Map<String, dynamic> j) =>
      DashboardTrendPoint(
        day: j['day'] as String? ?? '',
        uploaded: _i(j['uploaded']),
        downloaded: _i(j['downloaded']),
        seeding: _i(j['seeding']),
        seedingSize: _i(j['seeding_size']),
        bonus: _n(j['bonus']),
        ratio: _n(j['ratio']),
      );
}

// 某站点历史统计快照（对齐后端 SiteHistory / Web SiteHistoryPoint）。
// 站点明细「用户历史数据」按钮：GET /dashboard/history/:id?days=N（默认 90，上限 365）。
class SiteHistoryPoint {
  final String snapAt;
  final String level;
  final int uploaded;
  final int downloaded;
  final double ratio;
  final int trueUploaded;
  final int trueDownloaded;
  final double trueRatio;
  final int seeding;
  final int leeching;
  final int seedingSize;
  final int uploads;
  final int uniqueGroups;
  final int perfectFlac;
  final int invited;
  final double bonus;
  final double bonusPerHour;
  final double seedingBonus;
  final double seedingBonusPerHour;
  final double rescueSeedingBonusPerHour;
  final int claimCount;
  final int maxClaim;
  final String lastAccessAt;

  SiteHistoryPoint({
    required this.snapAt,
    required this.level,
    required this.uploaded,
    required this.downloaded,
    required this.ratio,
    required this.trueUploaded,
    required this.trueDownloaded,
    required this.trueRatio,
    required this.seeding,
    required this.leeching,
    required this.seedingSize,
    required this.uploads,
    required this.uniqueGroups,
    required this.perfectFlac,
    required this.invited,
    required this.bonus,
    required this.bonusPerHour,
    required this.seedingBonus,
    required this.seedingBonusPerHour,
    required this.rescueSeedingBonusPerHour,
    required this.claimCount,
    required this.maxClaim,
    required this.lastAccessAt,
  });

  factory SiteHistoryPoint.fromJson(Map<String, dynamic> j) => SiteHistoryPoint(
    snapAt: j['snap_at'] as String? ?? '',
    level: j['level'] as String? ?? '',
    uploaded: _i(j['uploaded']),
    downloaded: _i(j['downloaded']),
    ratio: _n(j['ratio']),
    trueUploaded: _i(j['true_uploaded']),
    trueDownloaded: _i(j['true_downloaded']),
    trueRatio: _n(j['true_ratio']),
    seeding: _i(j['seeding']),
    leeching: _i(j['leeching']),
    seedingSize: _i(j['seeding_size']),
    uploads: _i(j['uploads']),
    uniqueGroups: _i(j['unique_groups']),
    perfectFlac: _i(j['perfect_flac']),
    invited: _i(j['invited']),
    bonus: _n(j['bonus']),
    bonusPerHour: _n(j['bonus_per_hour']),
    seedingBonus: _n(j['seeding_bonus']),
    seedingBonusPerHour: _n(j['seeding_bonus_per_hour']),
    rescueSeedingBonusPerHour: _n(j['rescue_seeding_bonus_per_hour']),
    claimCount: _i(j['claim_count']),
    maxClaim: _i(j['max_claim']),
    lastAccessAt: j['last_access_at'] as String? ?? '',
  );
}

class DashboardApi {
  final _api = ApiClient();

  // overview 拉所有站点最新统计汇总：GET /dashboard。
  Future<DashboardOverview> overview() async {
    final r = await _api.get('/dashboard');
    return DashboardOverview.fromJson(r);
  }

  // 立即抓取所有启用站点统计并返回最新总览（回填新加入站点的 0 数据）：POST /dashboard/sync。
  Future<DashboardOverview> sync() async {
    final r = await _api.post('/dashboard/sync', <String, dynamic>{});
    return DashboardOverview.fromJson(r);
  }

  // 仅刷新单个站点统计（站点明细「刷新数据」按钮）：POST /dashboard/sync/:id。
  Future<DashboardOverview> syncOne(int siteId) async {
    final r = await _api.post('/dashboard/sync/$siteId', <String, dynamic>{});
    return DashboardOverview.fromJson(r);
  }

  // 单站点历史统计快照：GET /dashboard/history/:id?days=N（后端回裸数组，走 getList）。
  Future<List<SiteHistoryPoint>> history(int siteId, {int days = 90}) async {
    final list = await _api.getList('/dashboard/history/$siteId?days=$days');
    return list
        .map((e) => SiteHistoryPoint.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  // trend 拉按天聚合趋势：GET /dashboard/trend?days=N。
  Future<List<DashboardTrendPoint>> trend(int days) async {
    final r = await _api.get('/dashboard/trend?days=$days');
    final list = (r['points'] as List?) ?? const [];
    return list
        .map((e) => DashboardTrendPoint.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}
