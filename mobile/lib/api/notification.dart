import 'package:ptpatronus/core/api_client.dart';

// 通知通道配置（GET /notification/config 回显）。secret 脱敏 + *_configured 标记。
// 每通道一个子对象：enabled + 非敏感字段明文 + 秘密字段脱敏/标记。
class NotifyBarkConfig {
  final bool enabled;
  final String server;
  final String key; // 脱敏
  final bool keyConfigured;
  const NotifyBarkConfig({this.enabled = false, this.server = '', this.key = '', this.keyConfigured = false});
  factory NotifyBarkConfig.fromJson(Map<String, dynamic> j) => NotifyBarkConfig(
        enabled: j['enabled'] as bool? ?? false,
        server: j['server'] as String? ?? '',
        key: j['key'] as String? ?? '',
        keyConfigured: j['key_configured'] as bool? ?? false,
      );
}

class NotifyTelegramConfig {
  final bool enabled;
  final String chatId;
  final String apiUrl;
  final String token; // 脱敏
  final bool tokenConfigured;
  const NotifyTelegramConfig({this.enabled = false, this.chatId = '', this.apiUrl = '', this.token = '', this.tokenConfigured = false});
  factory NotifyTelegramConfig.fromJson(Map<String, dynamic> j) => NotifyTelegramConfig(
        enabled: j['enabled'] as bool? ?? false,
        chatId: j['chat_id'] as String? ?? '',
        apiUrl: j['api_url'] as String? ?? '',
        token: j['token'] as String? ?? '',
        tokenConfigured: j['token_configured'] as bool? ?? false,
      );
}

class NotifyQQConfig {
  final bool enabled;
  final String appId;
  final String openid;
  final String groupOpenid;
  final String appSecret; // 脱敏
  final bool appSecretConfigured;
  const NotifyQQConfig({this.enabled = false, this.appId = '', this.openid = '', this.groupOpenid = '', this.appSecret = '', this.appSecretConfigured = false});
  factory NotifyQQConfig.fromJson(Map<String, dynamic> j) => NotifyQQConfig(
        enabled: j['enabled'] as bool? ?? false,
        appId: j['app_id'] as String? ?? '',
        openid: j['openid'] as String? ?? '',
        groupOpenid: j['group_openid'] as String? ?? '',
        appSecret: j['app_secret'] as String? ?? '',
        appSecretConfigured: j['app_secret_configured'] as bool? ?? false,
      );
}

class NotifyWeChatConfig {
  final bool enabled;
  final String accountId;
  final String target;
  final String baseUrl;
  final String botToken; // 脱敏
  final bool botTokenConfigured;
  const NotifyWeChatConfig({this.enabled = false, this.accountId = '', this.target = '', this.baseUrl = '', this.botToken = '', this.botTokenConfigured = false});
  factory NotifyWeChatConfig.fromJson(Map<String, dynamic> j) => NotifyWeChatConfig(
        enabled: j['enabled'] as bool? ?? false,
        accountId: j['account_id'] as String? ?? '',
        target: j['target'] as String? ?? '',
        baseUrl: j['base_url'] as String? ?? '',
        botToken: j['bot_token'] as String? ?? '',
        botTokenConfigured: j['bot_token_configured'] as bool? ?? false,
      );
}

class NotifyWebhookConfig {
  final bool enabled;
  final String url;
  const NotifyWebhookConfig({this.enabled = false, this.url = ''});
  factory NotifyWebhookConfig.fromJson(Map<String, dynamic> j) => NotifyWebhookConfig(
        enabled: j['enabled'] as bool? ?? false,
        url: j['url'] as String? ?? '',
      );
}

class NotifyConfig {
  final NotifyBarkConfig bark;
  final NotifyTelegramConfig telegram;
  final NotifyQQConfig qq;
  final NotifyWeChatConfig wechat;
  final NotifyWebhookConfig webhook;
  const NotifyConfig({
    this.bark = const NotifyBarkConfig(),
    this.telegram = const NotifyTelegramConfig(),
    this.qq = const NotifyQQConfig(),
    this.wechat = const NotifyWeChatConfig(),
    this.webhook = const NotifyWebhookConfig(),
  });
  factory NotifyConfig.fromJson(Map<String, dynamic> j) => NotifyConfig(
        bark: NotifyBarkConfig.fromJson((j['bark'] as Map<String, dynamic>?) ?? const {}),
        telegram: NotifyTelegramConfig.fromJson((j['telegram'] as Map<String, dynamic>?) ?? const {}),
        qq: NotifyQQConfig.fromJson((j['qq'] as Map<String, dynamic>?) ?? const {}),
        wechat: NotifyWeChatConfig.fromJson((j['wechat'] as Map<String, dynamic>?) ?? const {}),
        webhook: NotifyWebhookConfig.fromJson((j['webhook'] as Map<String, dynamic>?) ?? const {}),
      );
}

// 单通道测试结果。
class NotifySendResult {
  final String channel;
  final bool ok;
  final String? error;
  const NotifySendResult({required this.channel, required this.ok, this.error});
  factory NotifySendResult.fromJson(Map<String, dynamic> j) => NotifySendResult(
        channel: j['channel'] as String? ?? '',
        ok: j['ok'] as bool? ?? false,
        error: j['error'] as String?,
      );
}

class WechatQRStatus {
  final String status; // waiting | scanned | loggedin
  final bool loggedIn;
  final String? accountId;
  const WechatQRStatus({this.status = 'waiting', this.loggedIn = false, this.accountId});
  factory WechatQRStatus.fromJson(Map<String, dynamic> j) => WechatQRStatus(
        status: j['status'] as String? ?? 'waiting',
        loggedIn: j['logged_in'] as bool? ?? false,
        accountId: j['account_id'] as String?,
      );
}

class NotificationApi {
  final _api = ApiClient();

  // 当前配置（secret 脱敏）：GET /notification/config
  Future<NotifyConfig> get() async =>
      NotifyConfig.fromJson(await _api.get('/notification/config'));

  // 保存配置：PUT /notification/config。input 由调用方按通道拼好；秘密字段空串=不改。
  Future<void> set(Map<String, dynamic> input) async =>
      await _api.put('/notification/config', input);

  // 向全部通道发测试通知：POST /notification/test
  Future<List<NotifySendResult>> test(String title, [String body = '']) async {
    final r = await _api.post('/notification/test', {'title': title, 'body': body});
    final list = (r['results'] as List?) ?? const [];
    return list.map((e) => NotifySendResult.fromJson(e as Map<String, dynamic>)).toList();
  }

  // 已注册通道名：GET /notification/channels
  Future<List<String>> channels() async {
    final r = await _api.get('/notification/channels');
    final list = (r['channels'] as List?) ?? const [];
    return list.cast<String>();
  }

  // 微信扫码登录（admin）：取二维码
  Future<Map<String, String>> wechatQrcode() async {
    final r = await _api.post('/notification/wechat/qrcode', <String, dynamic>{});
    return {'qrcode': r['qrcode'] as String? ?? '', 'qrcode_url': r['qrcode_url'] as String? ?? ''};
  }

  // 微信扫码状态轮询：GET /notification/wechat/qrcode/status?qrcode=
  Future<WechatQRStatus> wechatQrcodeStatus(String qrcode) async {
    final r = await _api.get('/notification/wechat/qrcode/status?qrcode=${Uri.encodeQueryComponent(qrcode)}');
    return WechatQRStatus.fromJson(r);
  }
}
