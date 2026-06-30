import '../core/api_client.dart';

/// 站免状态（对齐后端 site.FreeleechStatus）。
enum FreeleechStatus { active, upcoming, none, failed }

class SiteFreeleech {
  final int siteId;
  final String siteName;
  final FreeleechStatus status;
  final String title;
  final String content;
  final String link;
  final String error;
  final DateTime fetchedAt;

  SiteFreeleech({
    required this.siteId,
    required this.siteName,
    required this.status,
    required this.title,
    required this.content,
    required this.link,
    required this.error,
    required this.fetchedAt,
  });

  factory SiteFreeleech.fromJson(Map<String, dynamic> j) {
    final s = j['status'] as String? ?? 'none';
    FreeleechStatus st;
    switch (s) {
      case 'active':
        st = FreeleechStatus.active;
        break;
      case 'upcoming':
        st = FreeleechStatus.upcoming;
        break;
      case 'failed':
        st = FreeleechStatus.failed;
        break;
      default:
        st = FreeleechStatus.none;
    }
    return SiteFreeleech(
      siteId: (j['site_id'] as num?)?.toInt() ?? 0,
      siteName: j['site_name'] as String? ?? '',
      status: st,
      title: j['title'] as String? ?? '',
      content: j['content'] as String? ?? '',
      link: j['link'] as String? ?? '',
      error: j['error'] as String? ?? '',
      fetchedAt: DateTime.tryParse(j['fetched_at'] as String? ?? '') ?? DateTime.now(),
    );
  }
}

/// 单站站免聚合结果。
class FreeleechResult {
  final int siteId;
  final String siteName;
  final List<SiteFreeleech> notices;
  final String error;
  FreeleechResult({
    required this.siteId,
    required this.siteName,
    required this.notices,
    required this.error,
  });

  factory FreeleechResult.fromJson(Map<String, dynamic> j) => FreeleechResult(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        notices: ((j['notices'] as List?) ?? const [])
            .map((e) => SiteFreeleech.fromJson(e as Map<String, dynamic>))
            .toList(),
        error: j['error'] as String? ?? '',
      );
}

class FreeleechApi {
  final _api = ApiClient();

  /// 全站站免聚合（refresh=true 强制刷新）。
  Future<List<FreeleechResult>> all({bool refresh = false}) async {
    final r = await _api.get('/freeleech${refresh ? '?refresh=1' : ''}');
    final list = (r['sites'] as List?) ?? const [];
    return list.map((e) => FreeleechResult.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<FreeleechResult> syncOne(int id) async {
    final r = await _api.post('/freeleech/sync/$id', {});
    return FreeleechResult.fromJson(r);
  }
}
