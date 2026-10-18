import '../core/api_client.dart';

/// Trakt 配置（脱敏回显，对齐后端 /trakt/config）。
class TraktConfig {
  final bool enabled;
  final String clientId;
  final bool clientSecretConfigured;
  final String redirectUri;
  final bool syncMovies;
  final bool syncShows;
  final String resolution;
  final int downloaderId;
  final bool authorized;
  final int lastSyncAt;
  final int lastSyncCount;
  final String lastError;
  final String pendingUserCode;
  final String pendingVerificationUrl;
  final int pendingExpiresAt;

  TraktConfig({
    required this.enabled,
    required this.clientId,
    required this.clientSecretConfigured,
    required this.redirectUri,
    required this.syncMovies,
    required this.syncShows,
    required this.resolution,
    required this.downloaderId,
    required this.authorized,
    required this.lastSyncAt,
    required this.lastSyncCount,
    required this.lastError,
    required this.pendingUserCode,
    required this.pendingVerificationUrl,
    required this.pendingExpiresAt,
  });

  factory TraktConfig.fromJson(Map<String, dynamic> j) => TraktConfig(
        enabled: j['enabled'] as bool? ?? false,
        clientId: j['client_id'] as String? ?? '',
        clientSecretConfigured: j['client_secret_configured'] as bool? ?? false,
        redirectUri: j['redirect_uri'] as String? ?? 'urn:ietf:wg:oauth:2.0:oob',
        syncMovies: j['sync_movies'] as bool? ?? true,
        syncShows: j['sync_shows'] as bool? ?? true,
        resolution: j['resolution'] as String? ?? '',
        downloaderId: j['downloader_id'] as int? ?? 0,
        authorized: j['authorized'] as bool? ?? false,
        lastSyncAt: j['last_sync_at'] as int? ?? 0,
        lastSyncCount: j['last_sync_count'] as int? ?? 0,
        lastError: j['last_error'] as String? ?? '',
        pendingUserCode: j['pending_user_code'] as String? ?? '',
        pendingVerificationUrl: j['pending_verification_url'] as String? ?? '',
        pendingExpiresAt: j['pending_expires_at'] as int? ?? 0,
      );
}

/// Trakt 设备授权码（POST /trakt/device-code）。
class TraktDeviceCode {
  final String userCode;
  final String verificationUrl;
  final int expiresIn;
  final int interval;
  final String deviceUrl;

  TraktDeviceCode({
    required this.userCode,
    required this.verificationUrl,
    required this.expiresIn,
    required this.interval,
    required this.deviceUrl,
  });

  factory TraktDeviceCode.fromJson(Map<String, dynamic> j) => TraktDeviceCode(
        userCode: j['user_code'] as String? ?? '',
        verificationUrl: j['verification_url'] as String? ?? '',
        expiresIn: j['expires_in'] as int? ?? 0,
        interval: j['interval'] as int? ?? 5,
        deviceUrl: j['device_url'] as String? ?? '/api/v1/trakt/device',
      );
}

/// 轮询设备授权的结果。
///
/// [terminal]=true 表示「再等也不会亮」：授权码过期 / 凭据没填 / 还没走完授权，
/// 面板必须停下来把 [message] 说出来（后端 #280 起了 `terminal` 字段并分档写人话）。
/// 从前这里只回一个 bool，非 2xx 一律被 `catch (_) {/* keep polling */}` 吞掉，
/// 于是手机可以对着作废的码每 3 秒空转一整晚。
class TraktDeviceStatus {
  const TraktDeviceStatus({
    required this.authorized,
    required this.terminal,
    required this.message,
  });

  final bool authorized;
  final bool terminal;
  final String message;
}

/// 哪些机器码意味着「停止轮询」。判不准就继续等——误停会让用户以为授权失败，
/// 而他其实已经在浏览器里输完码了。
bool traktPollIsTerminal(String code) => const {
      'trakt_code_expired',
      'trakt_not_configured',
      'trakt_not_authorized',
    }.contains(code);

/// 连续读不到状态到这个次数才放弃轮询；措辞只能说「读不到」。
const int traktPollGiveUpAfter = 5;

String traktPollGiveUpText(int fails) =>
    '读不到 Trakt 授权状态（已连续 $fails 次，可能是网络问题；若你已在浏览器完成输码，可稍后刷新本页确认）';

/// Trakt API（对齐后端 /trakt/*）。
class TraktApi {
  final _api = ApiClient();

  Future<TraktConfig> getConfig() async {
    final r = await _api.get('/trakt/config');
    return TraktConfig.fromJson(r);
  }

  Future<void> setConfig(Map<String, dynamic> body) => _api.put('/trakt/config', body);

  Future<TraktDeviceCode> deviceCode() async {
    final r = await _api.post('/trakt/device-code', {});
    return TraktDeviceCode.fromJson(r);
  }

  /// 轮询设备授权状态。失败也返回结构化结果（终端态要能停下来）。
  Future<TraktDeviceStatus> deviceStatus() async {
    try {
      final r = await _api.get('/trakt/device-status');
      return TraktDeviceStatus(
        authorized: r['authorized'] as bool? ?? false,
        terminal: r['terminal'] as bool? ?? false,
        message: (r['message'] as String? ?? '').trim(),
      );
    } on ApiException catch (e) {
      // 后端把 4xx/5xx 都带 code 发出来：非 2xx 不等于「继续等」，也不等于「授权失败」。
      return TraktDeviceStatus(
        authorized: false,
        terminal: traktPollIsTerminal(e.code),
        message: e.userMessage,
      );
    }
  }

  /// 立即同步，返新增订阅数。
  Future<int> sync() async {
    final r = await _api.post('/trakt/sync', {});
    return r['created'] as int? ?? 0;
  }
}
