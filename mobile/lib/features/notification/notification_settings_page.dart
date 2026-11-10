// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/notification.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'notificationopsmixin.dart';
part 'notification_settings_widgets.dart';

// 通知通道设置（对标 Web Settings→通知）：Telegram / 微信 ClawBot / QQ / Bark / Webhook。
// DB+UI 热更新，保存即时 rebuild；微信 ClawBot 走扫码登录。secret 留空=不修改。
class NotificationSettingsPage extends StatefulWidget {
  const NotificationSettingsPage({super.key});
  @override
  State<NotificationSettingsPage> createState() =>
      _NotificationSettingsPageState();
}

class _NotificationSettingsPageState extends State<NotificationSettingsPage> with NotificationOpsMixin {
  @override
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _wxTimer?.cancel(); // 扫码轮询显式停（回调虽有 !mounted 兜底，dispose cancel 更干净——审计 P2）
    for (final c in [
      _barkServer,
      _barkKey,
      _tgToken,
      _tgChat,
      _tgApi,
      _qqAppId,
      _qqSecret,
      _qqOpenid,
      _qqGroup,
      _wxToken,
      _wxAccount,
      _wxTarget,
      _wxBase,
      _whUrl,
    ]) {
      c.dispose();
    }
    super.dispose();
  }

  // 微信扫码登录
  Timer? _wxTimer;
  Future<void> _wxLogin() async {
    try {
      final r = await _api.wechatQrcode();
      if (!mounted) return;
      final qrUrl = r['qrcode_url'] ?? '';
      final qrcode = r['qrcode'] ?? '';
      if (qrUrl.isEmpty) {
        _toast('获取二维码失败');
        return;
      }
      _wxTimer?.cancel();
      final closed = ValueNotifier<bool>(false);
      showDialog(
        context: context,
        barrierDismissible: false,
        builder: (ctx) => AlertDialog(
          title: const Text('微信扫码登录'),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Image.network(
                qrUrl,
                width: 220,
                height: 220,
                fit: BoxFit.contain,
                errorBuilder: (_, _, _) => const Icon(Icons.qr_code, size: 120),
              ),
              const SizedBox(height: 10),
              Text(
                '用微信扫码完成登录',
                style: TextStyle(fontSize: 12, color: context.dimText),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () {
                closed.value = true;
                Navigator.pop(ctx);
              },
              child: const Text('取消'),
            ),
          ],
        ),
      );
      _wxTimer = Timer.periodic(const Duration(seconds: 2), (t) async {
        if (closed.value || !mounted) {
          t.cancel();
          return;
        }
        try {
          final s = await _api.wechatQrcodeStatus(qrcode);
          if (!mounted) {
            t.cancel();
            return;
          }
          if (s.loggedIn) {
            t.cancel();
            if (!closed.value) Navigator.of(context).pop();
            _toast('微信登录成功，已自动绑定');
            await _load();
          }
        } catch (_) {}
      });
    } catch (e) {
      _toast('获取二维码失败：$e');
    }
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '通知设置',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _load,
        ),
      ],
      body: SafeArea(
        child: _loading
            ? const PTLoadingView()
            : _error != null
            ? _errorView
            : _body,
      ),
    );
  }

  Widget get _errorView => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          '⚠ $_error',
          textAlign: TextAlign.center,
          style: const TextStyle(color: Colors.redAccent),
        ),
        const SizedBox(height: 12),
        FilledButton(onPressed: _load, child: const Text('重试')),
      ],
    ),
  );

  Widget get _body {
    final c = _config!;
    return ListView(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 32),
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(4, 0, 4, 8),
          child: GlassCard(
            padding: const EdgeInsets.all(14),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    '向已启用通道各发一条测试通知',
                    style: TextStyle(fontSize: 13, color: context.dimText),
                  ),
                ),
                FilledButton.tonalIcon(
                  onPressed: _testing ? null : _test,
                  icon: _testing
                      ? const SizedBox(
                          width: 16,
                          height: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.send_outlined),
                  label: const Text('测试'),
                ),
              ],
            ),
          ),
        ),
        _group(
          'Telegram',
          'Bot API 推送（可选国内反代域名）',
          c.telegram.enabled,
          (v) => setState(() => _tgEnabled = v),
          [
            _secretField(
              'Bot Token',
              _tgToken,
              c.telegram.tokenConfigured,
              c.telegram.token,
            ),
            _plainField('Chat ID', _tgChat, hint: '123456789 或 -100xxx 群'),
            _plainField('API 域名', _tgApi, hint: '留空走官方', help: '国内反代可选'),
          ],
        ),
        _group(
          '微信 ClawBot',
          'iLink Bot 推送（需扫码登录）',
          c.wechat.enabled,
          (v) => setState(() => _wxEnabled = v),
          [
            Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: Row(
                children: [
                  FilledButton.tonalIcon(
                    onPressed: _wxLogin,
                    icon: const Icon(Icons.qr_code_scanner),
                    label: Text(c.wechat.botTokenConfigured ? '重新扫码' : '扫码登录'),
                  ),
                  const SizedBox(width: 8),
                  _statusChip(
                    c.wechat.botTokenConfigured,
                    c.wechat.botTokenConfigured ? '已登录' : '未登录',
                  ),
                ],
              ),
            ),
            _secretField(
              'Bot Token',
              _wxToken,
              c.wechat.botTokenConfigured,
              c.wechat.botToken,
            ),
            _plainField('账号 ID', _wxAccount, hint: '登录后自动填'),
            _plainField('接收者 wxid', _wxTarget, help: '给自己发填自己的 wxid'),
            _plainField('Base URL', _wxBase, hint: '留空走官方'),
          ],
        ),
        _group(
          'QQ 机器人',
          '开放平台机器人（Markdown 优先，失败回退纯文本）',
          c.qq.enabled,
          (v) => setState(() => _qqEnabled = v),
          [
            _plainField('AppID', _qqAppId),
            _secretField(
              'AppSecret',
              _qqSecret,
              c.qq.appSecretConfigured,
              c.qq.appSecret,
            ),
            _plainField('OpenID', _qqOpenid, help: '单聊接收者'),
            _plainField('群 OpenID', _qqGroup, help: '群聊接收者（优先于单聊）'),
          ],
        ),
        _group(
          'Bark',
          'iOS 推送',
          c.bark.enabled,
          (v) => setState(() => _barkEnabled = v),
          [
            _plainField('服务器', _barkServer, hint: '留空用 https://api.day.app'),
            _secretField(
              'Device Key',
              _barkKey,
              c.bark.keyConfigured,
              c.bark.key,
            ),
          ],
        ),
        _group(
          'Webhook',
          '转发到自定义 URL',
          c.webhook.enabled,
          (v) => setState(() => _whEnabled = v),
          [_plainField('URL', _whUrl)],
        ),
        const SizedBox(height: 8),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 4),
          child: FilledButton.icon(
            onPressed: _saving ? null : _save,
            icon: _saving
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.save_outlined),
            label: const Text('保存（即时生效）'),
          ),
        ),
      ],
    );
  }

}
