import '../core/api_client.dart';

// 对齐后端 chain.SiteStatItem：每站点最新统计。新字段（level/leeching/seeding_size/
// uploads/bonus_per_hour/join_date/updated_at）此前 fromJson 容错丢弃，现补齐以便展示。
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
  final int seeding;
  final int leeching;
  final int seedingSize;
  final int uploads;
  final double bonus;
  final double bonusPerHour;
  final String joinDate;
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
    required this.seeding,
    required this.leeching,
    required this.seedingSize,
    required this.uploads,
    required this.bonus,
    required this.bonusPerHour,
    required this.joinDate,
    required this.updatedAt,
  });

  factory SiteStatItem.fromJson(Map<String, dynamic> j) => SiteStatItem(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        username: j['username'] as String? ?? '',
        userId: j['user_id'] as String? ?? '',
        email: j['email'] as String? ?? '',
        level: j['level'] as String? ?? '',
        uploaded: (j['uploaded'] as num?)?.toInt() ?? 0,
        downloaded: (j['downloaded'] as num?)?.toInt() ?? 0,
        ratio: (j['ratio'] as num?)?.toDouble() ?? 0,
        seeding: (j['seeding'] as num?)?.toInt() ?? 0,
        leeching: (j['leeching'] as num?)?.toInt() ?? 0,
        seedingSize: (j['seeding_size'] as num?)?.toInt() ?? 0,
        uploads: (j['uploads'] as num?)?.toInt() ?? 0,
        bonus: (j['bonus'] as num?)?.toDouble() ?? 0,
        bonusPerHour: (j['bonus_per_hour'] as num?)?.toDouble() ?? 0,
        joinDate: j['join_date'] as String? ?? '',
        updatedAt: j['updated_at'] as String? ?? '',
      );
}

// DashboardOverview 是仪表盘顶部 4 张卡片的数据源（跨站汇总 + 计数）。
class DashboardOverview {
  final int uploaded;
  final int downloaded;
  final int seeding;
  final int leeching;
  final int seedingSize;
  final double bonus;
  final double bonusPerHour;
  final List<SiteStatItem> sites;
  final int siteCount;
  final int downloaderCount;

  DashboardOverview({
    required this.uploaded,
    required this.downloaded,
    required this.seeding,
    required this.leeching,
    required this.seedingSize,
    required this.bonus,
    required this.bonusPerHour,
    required this.sites,
    required this.siteCount,
    required this.downloaderCount,
  });

  factory DashboardOverview.fromJson(Map<String, dynamic> j) {
    final totals = (j['totals'] as Map<String, dynamic>?) ?? const {};
    return DashboardOverview(
      uploaded: (totals['uploaded'] as num?)?.toInt() ?? 0,
      downloaded: (totals['downloaded'] as num?)?.toInt() ?? 0,
      seeding: (totals['seeding'] as num?)?.toInt() ?? 0,
      leeching: (totals['leeching'] as num?)?.toInt() ?? 0,
      seedingSize: (totals['seeding_size'] as num?)?.toInt() ?? 0,
      bonus: (totals['bonus'] as num?)?.toDouble() ?? 0,
      bonusPerHour: (totals['bonus_per_hour'] as num?)?.toDouble() ?? 0,
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

  factory DashboardTrendPoint.fromJson(Map<String, dynamic> j) => DashboardTrendPoint(
        day: j['day'] as String? ?? '',
        uploaded: (j['uploaded'] as num?)?.toInt() ?? 0,
        downloaded: (j['downloaded'] as num?)?.toInt() ?? 0,
        seeding: (j['seeding'] as num?)?.toInt() ?? 0,
        seedingSize: (j['seeding_size'] as num?)?.toInt() ?? 0,
        bonus: (j['bonus'] as num?)?.toDouble() ?? 0,
        ratio: (j['ratio'] as num?)?.toDouble() ?? 0,
      );
}

class DashboardApi {
  final _api = ApiClient();

  // overview 拉所有站点最新统计汇总：GET /dashboard。
  Future<DashboardOverview> overview() async {
    final r = await _api.get('/dashboard');
    return DashboardOverview.fromJson(r);
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
