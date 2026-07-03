import '../core/api_client.dart';

enum SiteHealthStatus { healthy, critical, closed, unknown }

enum MatchType { none, urlExact, domainExact, nameExact, nameFuzzy }

enum AlertType { danger, obituary, warning, info, celebration }

class SaveptSiteCard {
  final String name;
  final String url;
  final String domain;
  final String? icon;
  final String year;
  final SiteHealthStatus status;
  final String statusText;
  final String siteType; // internal / external / unknown
  final String anniversaryText;
  final String? anniversaryFlag; // "yes" / "no"
  final String durationText;
  final String faultDuration;
  final String openTime;
  final String closeTime;
  final MatchType matchType;
  final String localSiteName;
  final String search; // 后端提供的搜索关键词串（Web buildSiteKeywords 并入），Flutter 此前漏解析

  SaveptSiteCard({
    required this.name,
    required this.url,
    required this.domain,
    this.icon,
    required this.year,
    required this.status,
    required this.statusText,
    required this.siteType,
    required this.anniversaryText,
    this.anniversaryFlag,
    required this.durationText,
    required this.faultDuration,
    required this.openTime,
    required this.closeTime,
    required this.matchType,
    required this.localSiteName,
    required this.search,
  });

  factory SaveptSiteCard.fromJson(Map<String, dynamic> j) {
    SiteHealthStatus st;
    switch (j['status'] as String? ?? 'unknown') {
      case 'healthy':
        st = SiteHealthStatus.healthy;
        break;
      case 'critical':
        st = SiteHealthStatus.critical;
        break;
      case 'closed':
        st = SiteHealthStatus.closed;
        break;
      default:
        st = SiteHealthStatus.unknown;
    }
    MatchType mt;
    switch (j['match_type'] as String? ?? 'none') {
      case 'url_exact':
        mt = MatchType.urlExact;
        break;
      case 'domain_exact':
        mt = MatchType.domainExact;
        break;
      case 'name_exact':
        mt = MatchType.nameExact;
        break;
      case 'name_fuzzy':
        mt = MatchType.nameFuzzy;
        break;
      default:
        mt = MatchType.none;
    }
    return SaveptSiteCard(
      name: j['name'] as String? ?? '',
      url: j['url'] as String? ?? '',
      domain: j['domain'] as String? ?? '',
      icon: j['icon'] as String?,
      year: j['year'] as String? ?? '未知',
      status: st,
      statusText: j['status_text'] as String? ?? '',
      siteType: j['site_type'] as String? ?? 'unknown',
      anniversaryText: j['anniversary_text'] as String? ?? '',
      anniversaryFlag: j['anniversary_flag'] as String?,
      durationText: j['duration_text'] as String? ?? '',
      faultDuration: j['fault_duration'] as String? ?? '',
      openTime: j['open_time'] as String? ?? '',
      closeTime: j['close_time'] as String? ?? '',
      matchType: mt,
      localSiteName: j['local_site_name'] as String? ?? '',
      search: j['search'] as String? ?? '',
    );
  }
}

class SaveptAlert {
  final AlertType type;
  final String title;
  final String content;
  SaveptAlert({required this.type, required this.title, required this.content});

  factory SaveptAlert.fromJson(Map<String, dynamic> j) {
    AlertType t;
    switch (j['type'] as String? ?? 'info') {
      case 'danger':
        t = AlertType.danger;
        break;
      case 'obituary':
        t = AlertType.obituary;
        break;
      case 'warning':
        t = AlertType.warning;
        break;
      case 'celebration':
        t = AlertType.celebration;
        break;
      default:
        t = AlertType.info;
    }
    return SaveptAlert(
      type: t,
      title: j['title'] as String? ?? '',
      content: j['content'] as String? ?? '',
    );
  }
}

class SaveptSummary {
  final int total, healthy, critical, closed, bound, available;
  final int internal, external, todayAnniv, years;
  final int mpOwned, mpAvailable, mpUnsupported;

  SaveptSummary({
    required this.total,
    required this.healthy,
    required this.critical,
    required this.closed,
    required this.bound,
    required this.available,
    required this.internal,
    required this.external,
    required this.todayAnniv,
    required this.years,
    required this.mpOwned,
    required this.mpAvailable,
    required this.mpUnsupported,
  });

  factory SaveptSummary.fromJson(Map<String, dynamic> j) => SaveptSummary(
        total: (j['total'] as num?)?.toInt() ?? 0,
        healthy: (j['healthy'] as num?)?.toInt() ?? 0,
        critical: (j['critical'] as num?)?.toInt() ?? 0,
        closed: (j['closed'] as num?)?.toInt() ?? 0,
        bound: (j['bound'] as num?)?.toInt() ?? 0,
        available: (j['available'] as num?)?.toInt() ?? 0,
        internal: (j['internal'] as num?)?.toInt() ?? 0,
        external: (j['external'] as num?)?.toInt() ?? 0,
        todayAnniv: (j['today_anniv'] as num?)?.toInt() ?? 0,
        years: (j['years'] as num?)?.toInt() ?? 0,
        mpOwned: (j['mp_owned'] as num?)?.toInt() ?? 0,
        mpAvailable: (j['mp_available'] as num?)?.toInt() ?? 0,
        mpUnsupported: (j['mp_unsupported'] as num?)?.toInt() ?? 0,
      );
}

class SaveptResult {
  final SaveptSummary summary;
  final List<SaveptAlert> alerts;
  final List<SaveptSiteCard> cards;
  final String fetchedAt;
  final bool cached;
  final bool stale;
  final String notice;

  SaveptResult({
    required this.summary,
    required this.alerts,
    required this.cards,
    required this.fetchedAt,
    required this.cached,
    required this.stale,
    required this.notice,
  });

  factory SaveptResult.fromJson(Map<String, dynamic> j) => SaveptResult(
        summary: SaveptSummary.fromJson(j['summary'] as Map<String, dynamic>? ?? {}),
        alerts: ((j['alerts'] as List?) ?? const [])
            .map((e) => SaveptAlert.fromJson(e as Map<String, dynamic>))
            .toList(),
        cards: ((j['cards'] as List?) ?? const [])
            .map((e) => SaveptSiteCard.fromJson(e as Map<String, dynamic>))
            .toList(),
        fetchedAt: j['fetched_at'] as String? ?? '',
        cached: j['cached'] as bool? ?? false,
        stale: j['stale'] as bool? ?? false,
        notice: j['notice'] as String? ?? '',
      );
}

class SaveptApi {
  final _api = ApiClient();

  Future<SaveptResult> get({bool refresh = false}) async {
    final r = await _api.get('/savept${refresh ? '?refresh=1' : ''}');
    return SaveptResult.fromJson(r);
  }
}
