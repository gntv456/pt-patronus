import '../core/api_client.dart';

class InviteeInfo {
  final String username;
  final String email;
  final int uploaded;
  final int downloaded;
  final String ratio;
  final String status;
  final bool enabled;
  InviteeInfo({
    required this.username,
    required this.email,
    required this.uploaded,
    required this.downloaded,
    required this.ratio,
    required this.status,
    required this.enabled,
  });
  factory InviteeInfo.fromJson(Map<String, dynamic> j) => InviteeInfo(
        username: j['username'] as String? ?? '',
        email: j['email'] as String? ?? '',
        uploaded: (j['uploaded'] as num?)?.toInt() ?? 0,
        downloaded: (j['downloaded'] as num?)?.toInt() ?? 0,
        ratio: j['ratio'] as String? ?? '',
        status: j['status'] as String? ?? 'confirmed',
        enabled: j['enabled'] as bool? ?? true,
      );
}

class PrescriptionItem {
  final int siteId;
  final String siteName;
  final int remain;
  final int canBuy;
  final bool canInvite;
  final int haremCount;
  final int deadCount;
  final List<InviteeInfo> invitees;
  final String error;
  PrescriptionItem({
    required this.siteId,
    required this.siteName,
    required this.remain,
    required this.canBuy,
    required this.canInvite,
    required this.haremCount,
    required this.deadCount,
    required this.invitees,
    required this.error,
  });
  factory PrescriptionItem.fromJson(Map<String, dynamic> j) => PrescriptionItem(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        remain: (j['remain'] as num?)?.toInt() ?? 0,
        canBuy: (j['can_buy'] as num?)?.toInt() ?? 0,
        canInvite: j['can_invite'] as bool? ?? false,
        haremCount: (j['harem_count'] as num?)?.toInt() ?? 0,
        deadCount: (j['dead_count'] as num?)?.toInt() ?? 0,
        invitees: ((j['invitees'] as List?) ?? const [])
            .map((e) => InviteeInfo.fromJson(e as Map<String, dynamic>))
            .toList(),
        error: j['error'] as String? ?? '',
      );
}

class HaremOverview {
  final int totalSites, totalInvitees, totalPermInvites, totalTempInvites, totalLowRatio, totalBanned, totalNoData;
  HaremOverview({
    required this.totalSites,
    required this.totalInvitees,
    required this.totalPermInvites,
    required this.totalTempInvites,
    required this.totalLowRatio,
    required this.totalBanned,
    required this.totalNoData,
  });
  factory HaremOverview.fromJson(Map<String, dynamic> j) => HaremOverview(
        totalSites: (j['total_sites'] as num?)?.toInt() ?? 0,
        totalInvitees: (j['total_invitees'] as num?)?.toInt() ?? 0,
        totalPermInvites: (j['total_perm_invites'] as num?)?.toInt() ?? 0,
        totalTempInvites: (j['total_temp_invites'] as num?)?.toInt() ?? 0,
        totalLowRatio: (j['total_low_ratio'] as num?)?.toInt() ?? 0,
        totalBanned: (j['total_banned'] as num?)?.toInt() ?? 0,
        totalNoData: (j['total_no_data'] as num?)?.toInt() ?? 0,
      );
}

class HaremResult {
  final HaremOverview overview;
  final List<PrescriptionItem> prescriptions;
  HaremResult({required this.overview, required this.prescriptions});
  factory HaremResult.fromJson(Map<String, dynamic> j) => HaremResult(
        overview: HaremOverview.fromJson(j['overview'] as Map<String, dynamic>? ?? {}),
        prescriptions: ((j['prescriptions'] as List?) ?? const [])
            .map((e) => PrescriptionItem.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class HaremApi {
  final _api = ApiClient();
  Future<HaremResult> all({bool refresh = false}) async {
    final r = await _api.get('/harem${refresh ? '?refresh=1' : ''}');
    return HaremResult.fromJson(r);
  }
  Future<String> exportText() async {
    final r = await _api.get('/harem/export');
    return r['text'] as String? ?? '';
  }
}
