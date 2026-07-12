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

class NotifyDingTalkConfig {
  final bool enabled;
  final String url;
  const NotifyDingTalkConfig({this.enabled = false, this.url = ''});
  factory NotifyDingTalkConfig.fromJson(Map<String, dynamic> j) => NotifyDingTalkConfig(
        enabled: j['enabled'] as bool? ?? false,
        url: j['url'] as String? ?? '',
      );
  // 回发当前值（后端 mergeNotifySettings 用 incoming 覆写非秘密字段；空 url=禁用，与 GET 一致）。
  Map<String, dynamic> toJson() => {'enabled': enabled, 'url': url};
}

class NotifyFeishuConfig {
  final bool enabled;
  final String url;
  const NotifyFeishuConfig({this.enabled = false, this.url = ''});
  factory NotifyFeishuConfig.fromJson(Map<String, dynamic> j) => NotifyFeishuConfig(
        enabled: j['enabled'] as bool? ?? false,
        url: j['url'] as String? ?? '',
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'url': url};
}

class NotifyEmailConfig {
  final bool enabled;
  final String host;
  final int port;
  final String username;
  final String from;
  final String to;
  const NotifyEmailConfig({this.enabled = false, this.host = '', this.port = 0, this.username = '', this.from = '', this.to = ''});
  factory NotifyEmailConfig.fromJson(Map<String, dynamic> j) => NotifyEmailConfig(
        enabled: j['enabled'] as bool? ?? false,
        host: j['host'] as String? ?? '',
        port: (j['port'] as num?)?.toInt() ?? 0,
        username: j['username'] as String? ?? '',
        from: j['from'] as String? ?? '',
        to: j['to'] as String? ?? '',
      );
  // 不含 password：缺失=后端保留旧密码（空串同理）；其余非秘密字段原样回发避免被零值覆写。
  Map<String, dynamic> toJson() => {'enabled': enabled, 'host': host, 'port': port, 'username': username, 'from': from, 'to': to};
}

// —— 批次6 补全渠道（11 个）。Flutter 暂无独立配置 UI（同 DingTalk/Feishu/Email 经 Web 配置）；
// 此处仅承载体（fromJson 解析 GET 回显 + toJson 回发以保留，secret 字段不发回由后端保留旧值）。
class NotifyPushPlusConfig {
  final bool enabled;
  final bool tokenConfigured;
  const NotifyPushPlusConfig({this.enabled = false, this.tokenConfigured = false});
  factory NotifyPushPlusConfig.fromJson(Map<String, dynamic> j) => NotifyPushPlusConfig(
        enabled: j['enabled'] as bool? ?? false,
        tokenConfigured: j['token_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {'enabled': enabled};
}

class NotifyPushDeerConfig {
  final bool enabled;
  final String server;
  final bool pushkeyConfigured;
  const NotifyPushDeerConfig({this.enabled = false, this.server = '', this.pushkeyConfigured = false});
  factory NotifyPushDeerConfig.fromJson(Map<String, dynamic> j) => NotifyPushDeerConfig(
        enabled: j['enabled'] as bool? ?? false,
        server: j['server'] as String? ?? '',
        pushkeyConfigured: j['pushkey_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'server': server};
}

class NotifyNtfyConfig {
  final bool enabled;
  final String server;
  final String topic;
  final bool tokenConfigured;
  const NotifyNtfyConfig({this.enabled = false, this.server = '', this.topic = '', this.tokenConfigured = false});
  factory NotifyNtfyConfig.fromJson(Map<String, dynamic> j) => NotifyNtfyConfig(
        enabled: j['enabled'] as bool? ?? false,
        server: j['server'] as String? ?? '',
        topic: j['topic'] as String? ?? '',
        tokenConfigured: j['token_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'server': server, 'topic': topic};
}

class NotifyGotifyConfig {
  final bool enabled;
  final String server;
  final bool tokenConfigured;
  const NotifyGotifyConfig({this.enabled = false, this.server = '', this.tokenConfigured = false});
  factory NotifyGotifyConfig.fromJson(Map<String, dynamic> j) => NotifyGotifyConfig(
        enabled: j['enabled'] as bool? ?? false,
        server: j['server'] as String? ?? '',
        tokenConfigured: j['token_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'server': server};
}

class NotifyAppriseConfig {
  final bool enabled;
  final String server;
  final bool apikeyConfigured;
  const NotifyAppriseConfig({this.enabled = false, this.server = '', this.apikeyConfigured = false});
  factory NotifyAppriseConfig.fromJson(Map<String, dynamic> j) => NotifyAppriseConfig(
        enabled: j['enabled'] as bool? ?? false,
        server: j['server'] as String? ?? '',
        apikeyConfigured: j['apikey_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'server': server};
}

class NotifyWorkWechatConfig {
  final bool enabled;
  final String url;
  const NotifyWorkWechatConfig({this.enabled = false, this.url = ''});
  factory NotifyWorkWechatConfig.fromJson(Map<String, dynamic> j) => NotifyWorkWechatConfig(
        enabled: j['enabled'] as bool? ?? false,
        url: j['url'] as String? ?? '',
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'url': url};
}

class NotifyWxPusherConfig {
  final bool enabled;
  final String uid;
  final String topicId;
  final bool appTokenConfigured;
  const NotifyWxPusherConfig({this.enabled = false, this.uid = '', this.topicId = '', this.appTokenConfigured = false});
  factory NotifyWxPusherConfig.fromJson(Map<String, dynamic> j) => NotifyWxPusherConfig(
        enabled: j['enabled'] as bool? ?? false,
        uid: j['uid'] as String? ?? '',
        topicId: j['topic_id'] as String? ?? '',
        appTokenConfigured: j['app_token_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'uid': uid, 'topic_id': topicId};
}

class NotifyLarkConfig {
  final bool enabled;
  final String url;
  const NotifyLarkConfig({this.enabled = false, this.url = ''});
  factory NotifyLarkConfig.fromJson(Map<String, dynamic> j) => NotifyLarkConfig(
        enabled: j['enabled'] as bool? ?? false,
        url: j['url'] as String? ?? '',
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'url': url};
}

class NotifyDiscordConfig {
  final bool enabled;
  final String url;
  const NotifyDiscordConfig({this.enabled = false, this.url = ''});
  factory NotifyDiscordConfig.fromJson(Map<String, dynamic> j) => NotifyDiscordConfig(
        enabled: j['enabled'] as bool? ?? false,
        url: j['url'] as String? ?? '',
      );
  Map<String, dynamic> toJson() => {'enabled': enabled, 'url': url};
}

class NotifyMQTTConfig {
  final bool enabled;
  final String broker;
  final String topic;
  final String username;
  final bool passwordConfigured;
  final String clientId;
  const NotifyMQTTConfig({this.enabled = false, this.broker = '', this.topic = '', this.username = '', this.passwordConfigured = false, this.clientId = ''});
  factory NotifyMQTTConfig.fromJson(Map<String, dynamic> j) => NotifyMQTTConfig(
        enabled: j['enabled'] as bool? ?? false,
        broker: j['broker'] as String? ?? '',
        topic: j['topic'] as String? ?? '',
        username: j['username'] as String? ?? '',
        passwordConfigured: j['password_configured'] as bool? ?? false,
        clientId: j['client_id'] as String? ?? '',
      );
  // password 秘密不发回；其余非秘密字段原样回发避免被零值覆写。
  Map<String, dynamic> toJson() => {'enabled': enabled, 'broker': broker, 'topic': topic, 'username': username, 'client_id': clientId};
}

class NotifyServerChanConfig {
  final bool enabled;
  final bool keyConfigured;
  const NotifyServerChanConfig({this.enabled = false, this.keyConfigured = false});
  factory NotifyServerChanConfig.fromJson(Map<String, dynamic> j) => NotifyServerChanConfig(
        enabled: j['enabled'] as bool? ?? false,
        keyConfigured: j['key_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {'enabled': enabled};
}

class NotifyConfig {
  final NotifyBarkConfig bark;
  final NotifyTelegramConfig telegram;
  final NotifyQQConfig qq;
  final NotifyWeChatConfig wechat;
  final NotifyWebhookConfig webhook;
  final NotifyDingTalkConfig dingtalk;
  final NotifyFeishuConfig feishu;
  final NotifyEmailConfig email;
  final NotifyPushPlusConfig pushplus;
  final NotifyPushDeerConfig pushdeer;
  final NotifyNtfyConfig ntfy;
  final NotifyGotifyConfig gotify;
  final NotifyAppriseConfig apprise;
  final NotifyWorkWechatConfig workwechat;
  final NotifyWxPusherConfig wxpusher;
  final NotifyLarkConfig lark;
  final NotifyDiscordConfig discord;
  final NotifyMQTTConfig mqtt;
  final NotifyServerChanConfig serverchan;
  const NotifyConfig({
    this.bark = const NotifyBarkConfig(),
    this.telegram = const NotifyTelegramConfig(),
    this.qq = const NotifyQQConfig(),
    this.wechat = const NotifyWeChatConfig(),
    this.webhook = const NotifyWebhookConfig(),
    this.dingtalk = const NotifyDingTalkConfig(),
    this.feishu = const NotifyFeishuConfig(),
    this.email = const NotifyEmailConfig(),
    this.pushplus = const NotifyPushPlusConfig(),
    this.pushdeer = const NotifyPushDeerConfig(),
    this.ntfy = const NotifyNtfyConfig(),
    this.gotify = const NotifyGotifyConfig(),
    this.apprise = const NotifyAppriseConfig(),
    this.workwechat = const NotifyWorkWechatConfig(),
    this.wxpusher = const NotifyWxPusherConfig(),
    this.lark = const NotifyLarkConfig(),
    this.discord = const NotifyDiscordConfig(),
    this.mqtt = const NotifyMQTTConfig(),
    this.serverchan = const NotifyServerChanConfig(),
  });
  factory NotifyConfig.fromJson(Map<String, dynamic> j) => NotifyConfig(
        bark: NotifyBarkConfig.fromJson((j['bark'] as Map<String, dynamic>?) ?? const {}),
        telegram: NotifyTelegramConfig.fromJson((j['telegram'] as Map<String, dynamic>?) ?? const {}),
        qq: NotifyQQConfig.fromJson((j['qq'] as Map<String, dynamic>?) ?? const {}),
        wechat: NotifyWeChatConfig.fromJson((j['wechat'] as Map<String, dynamic>?) ?? const {}),
        webhook: NotifyWebhookConfig.fromJson((j['webhook'] as Map<String, dynamic>?) ?? const {}),
        dingtalk: NotifyDingTalkConfig.fromJson((j['dingtalk'] as Map<String, dynamic>?) ?? const {}),
        feishu: NotifyFeishuConfig.fromJson((j['feishu'] as Map<String, dynamic>?) ?? const {}),
        email: NotifyEmailConfig.fromJson((j['email'] as Map<String, dynamic>?) ?? const {}),
        pushplus: NotifyPushPlusConfig.fromJson((j['pushplus'] as Map<String, dynamic>?) ?? const {}),
        pushdeer: NotifyPushDeerConfig.fromJson((j['pushdeer'] as Map<String, dynamic>?) ?? const {}),
        ntfy: NotifyNtfyConfig.fromJson((j['ntfy'] as Map<String, dynamic>?) ?? const {}),
        gotify: NotifyGotifyConfig.fromJson((j['gotify'] as Map<String, dynamic>?) ?? const {}),
        apprise: NotifyAppriseConfig.fromJson((j['apprise'] as Map<String, dynamic>?) ?? const {}),
        workwechat: NotifyWorkWechatConfig.fromJson((j['workwechat'] as Map<String, dynamic>?) ?? const {}),
        wxpusher: NotifyWxPusherConfig.fromJson((j['wxpusher'] as Map<String, dynamic>?) ?? const {}),
        lark: NotifyLarkConfig.fromJson((j['lark'] as Map<String, dynamic>?) ?? const {}),
        discord: NotifyDiscordConfig.fromJson((j['discord'] as Map<String, dynamic>?) ?? const {}),
        mqtt: NotifyMQTTConfig.fromJson((j['mqtt'] as Map<String, dynamic>?) ?? const {}),
        serverchan: NotifyServerChanConfig.fromJson((j['serverchan'] as Map<String, dynamic>?) ?? const {}),
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
