import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/notification.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

// 通知通道设置（对标 Web Settings→通知）：Telegram / 微信 ClawBot / QQ / Bark / Webhook。
// DB+UI 热更新，保存即时 rebuild；微信 ClawBot 走扫码登录。secret 留空=不修改。
class NotificationSettingsPage extends StatefulWidget {
  const NotificationSettingsPage({super.key});
  @override
  State<NotificationSettingsPage> createState() =>
      _NotificationSettingsPageState();
}

class _NotificationSettingsPageState extends State<NotificationSettingsPage> {
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

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

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
      final results = await _api.test('PTPatronus 测试通知', '看到这条消息说明通知通道工作正常 ✅');
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
              const Text(
                '用微信扫码完成登录',
                style: TextStyle(fontSize: 12, color: Colors.grey),
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
            ? const Center(child: CircularProgressIndicator())
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
                const Expanded(
                  child: Text(
                    '向已启用通道各发一条测试通知',
                    style: TextStyle(fontSize: 13, color: Colors.grey),
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

  Widget _group(
    String title,
    String subtitle,
    bool enabled,
    ValueChanged<bool> onToggle,
    List<Widget> children,
  ) {
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        title,
                        style: const TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        subtitle,
                        style: const TextStyle(
                          fontSize: 12,
                          color: Colors.grey,
                        ),
                      ),
                    ],
                  ),
                ),
                Switch(value: enabled, onChanged: onToggle),
              ],
            ),
            const SizedBox(height: 12),
            ...children,
          ],
        ),
      ),
    );
  }

  Widget _secretField(
    String label,
    TextEditingController ctrl,
    bool configured,
    String masked,
  ) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  label,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
              _statusChip(configured, configured ? '已配置 $masked' : '未配置'),
            ],
          ),
          const SizedBox(height: 6),
          TextField(
            controller: ctrl,
            obscureText: true,
            decoration: InputDecoration(
              hintText: configured ? '留空不修改' : '请输入',
              isDense: true,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _plainField(
    String label,
    TextEditingController ctrl, {
    String hint = '',
    String help = '',
  }) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 6),
          TextField(
            controller: ctrl,
            keyboardType: TextInputType.url,
            decoration: InputDecoration(
              hintText: hint,
              isDense: true,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
          if (help.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Text(
                help,
                style: const TextStyle(fontSize: 11, color: Colors.grey),
              ),
            ),
        ],
      ),
    );
  }

  Widget _statusChip(bool configured, String text) {
    final color = configured ? PTColors.success : PTColors.warn;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        text,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}
