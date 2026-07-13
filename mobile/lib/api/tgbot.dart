import '../core/api_client.dart';

/// Telegram 入站 bot 配置（对齐后端 /tgbot/config）。
///
/// getUpdates 长轮询收消息 → 斜杠命令或自然语言 → 复用 AI Agent → 回帖。
/// 与出站推送（notification/telegram.go）分离：bot 不是 Channel。
///
/// GET 返回脱敏视图：[botTokenConfigured] 标记 BotFather token 是否已设置；
/// PUT 接受同形 body，`bot_token` 留空 = 保留原 token（见后端 tgbot.go
/// `tgBotConfigReq` + `SetConfig`：bot_token 非空才加密覆盖）。
class TgBotConfig {
  final bool enabled;
  final bool botTokenConfigured;
  final List<int> allowedUserIds; // 允许使用的 Telegram user id 白名单
  final bool allowGroupChat; // 是否允许在群聊中响应
  final String apiUrl; // 自定义 Bot API 域名（国内反代）；空 → 官方

  TgBotConfig({
    required this.enabled,
    required this.botTokenConfigured,
    required this.allowedUserIds,
    required this.allowGroupChat,
    required this.apiUrl,
  });

  factory TgBotConfig.fromJson(Map<String, dynamic> j) => TgBotConfig(
        enabled: j['enabled'] as bool? ?? false,
        botTokenConfigured: j['bot_token_configured'] as bool? ?? false,
        allowedUserIds: (j['allowed_user_ids'] as List?)
                ?.map((e) => (e as num).toInt())
                .toList() ??
            const [],
        allowGroupChat: j['allow_group_chat'] as bool? ?? false,
        apiUrl: j['api_url'] as String? ?? '',
      );

  TgBotConfig copyWith({
    bool? enabled,
    bool? botTokenConfigured,
    List<int>? allowedUserIds,
    bool? allowGroupChat,
    String? apiUrl,
  }) =>
      TgBotConfig(
        enabled: enabled ?? this.enabled,
        botTokenConfigured: botTokenConfigured ?? this.botTokenConfigured,
        allowedUserIds: allowedUserIds ?? this.allowedUserIds,
        allowGroupChat: allowGroupChat ?? this.allowGroupChat,
        apiUrl: apiUrl ?? this.apiUrl,
      );
}

/// Telegram Bot API（GET / PUT /tgbot/config，管理员）。改动需重启进程生效。
class TgBotApi {
  final _api = ApiClient();

  /// 读配置（脱敏）。
  Future<TgBotConfig> get() async {
    final r = await _api.get('/tgbot/config');
    return TgBotConfig.fromJson(r);
  }

  /// 写配置。`body` 中 `bot_token` 仅在用户输入新 token 时携带（留空=保留原 token）。
  /// 返回更新后的脱敏配置。
  Future<TgBotConfig> update(Map<String, dynamic> body) async {
    final r = await _api.put('/tgbot/config', body);
    return TgBotConfig.fromJson(r);
  }
}
