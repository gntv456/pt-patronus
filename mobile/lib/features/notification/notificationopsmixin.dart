// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NotificationSettingsPage 的 NotificationOpsMixin mixin（含 setState 段；从 notification_settings_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/notification/notification_settings_page.dart';

mixin NotificationOpsMixin on State<NotificationSettingsPage> {

  final _api = NotificationApi();
  NotifyConfig? _config;
  bool _loading = true;
  bool _saving = false;
  bool _testing = false;
  String? _error;
  // 通道启用开关
  bool _barkEnabled = false,
      _tgEnabled = false,
      _qqEnabled = false,
      _wxEnabled = false,
      _whEnabled = false;
  // 控制器（secret 类保存后清空；非秘密类回填当前值）
  final _barkServer = TextEditingController(),
      _barkKey = TextEditingController();
  final _tgToken = TextEditingController(),
      _tgChat = TextEditingController(),
      _tgApi = TextEditingController();
  final _qqAppId = TextEditingController(),
      _qqSecret = TextEditingController(),
      _qqOpenid = TextEditingController(),
      _qqGroup = TextEditingController();
  final _wxToken = TextEditingController(),
      _wxAccount = TextEditingController(),
      _wxTarget = TextEditingController(),
      _wxBase = TextEditingController();
  final _whUrl = TextEditingController();

  // 宿主提供的钩子。
  void _toast(String m);
  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final c = await _api.get();
      _config = c;
      _barkEnabled = c.bark.enabled;
      _barkServer.text = c.bark.server;
      _tgEnabled = c.telegram.enabled;
      _tgChat.text = c.telegram.chatId;
      _tgApi.text = c.telegram.apiUrl;
      _qqEnabled = c.qq.enabled;
      _qqAppId.text = c.qq.appId;
      _qqOpenid.text = c.qq.openid;
      _qqGroup.text = c.qq.groupOpenid;
      _wxEnabled = c.wechat.enabled;
      _wxAccount.text = c.wechat.accountId;
      _wxTarget.text = c.wechat.target;
      _wxBase.text = c.wechat.baseUrl;
      _whEnabled = c.webhook.enabled;
      _whUrl.text = c.webhook.url;
      // secret 输入清空（占位显 configured）
      for (final c in [_barkKey, _tgToken, _qqSecret, _wxToken]) {
        c.clear();
      }
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Map<String, dynamic> _buildInput() => {
    'bark': {
      'enabled': _barkEnabled,
      'server': _barkServer.text.trim(),
      'key': _barkKey.text.trim(),
    },
    'telegram': {
      'enabled': _tgEnabled,
      'token': _tgToken.text.trim(),
      'chat_id': _tgChat.text.trim(),
      'api_url': _tgApi.text.trim(),
    },
    'qq': {
      'enabled': _qqEnabled,
      'app_id': _qqAppId.text.trim(),
      'app_secret': _qqSecret.text.trim(),
      'openid': _qqOpenid.text.trim(),
      'group_openid': _qqGroup.text.trim(),
    },
    'wechat': {
      'enabled': _wxEnabled,
      'bot_token': _wxToken.text.trim(),
      'account_id': _wxAccount.text.trim(),
      'target': _wxTarget.text.trim(),
      'base_url': _wxBase.text.trim(),
    },
    'webhook': {'enabled': _whEnabled, 'url': _whUrl.text.trim()},
    // 回发 Flutter 暂无独立 UI 的通道当前值，避免后端 mergeNotifySettings 用 incoming 零值覆写清空（P0-1）。
    if (_config != null) 'dingtalk': _config!.dingtalk.toJson(),
    if (_config != null) 'feishu': _config!.feishu.toJson(),
    if (_config != null) 'email': _config!.email.toJson(),
    // 批次6 补全渠道（11 个，同上：Flutter 经 Web 配置，保存时原样回发保留）。
    if (_config != null) 'pushplus': _config!.pushplus.toJson(),
    if (_config != null) 'pushdeer': _config!.pushdeer.toJson(),
    if (_config != null) 'ntfy': _config!.ntfy.toJson(),
    if (_config != null) 'gotify': _config!.gotify.toJson(),
    if (_config != null) 'apprise': _config!.apprise.toJson(),
    if (_config != null) 'workwechat': _config!.workwechat.toJson(),
    if (_config != null) 'wxpusher': _config!.wxpusher.toJson(),
    if (_config != null) 'lark': _config!.lark.toJson(),
    if (_config != null) 'discord': _config!.discord.toJson(),
    if (_config != null) 'mqtt': _config!.mqtt.toJson(),
    if (_config != null) 'serverchan': _config!.serverchan.toJson(),
  };

  Future<void> _save() async {
    setState(() => _saving = true);
    try {
      await _api.set(_buildInput());
      _toast('已保存，即时生效');
      await _load();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _test() async {
    setState(() => _testing = true);
    try {
      final results = await _api.test('PTPatronus 测试通知', '看到这条消息说明通知通道工作正常');
      if (results.isEmpty) {
        _toast('没有已启用的通道，请先启用并保存');
        return;
      }
      final ok = results.where((r) => r.ok).length;
      _toast('已发送：$ok/${results.length} 个通道成功');
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _testing = false);
    }
  }
}
