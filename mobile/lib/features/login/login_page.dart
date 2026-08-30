import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_logo.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:url_launcher/url_launcher.dart';
import '../home/home_shell.dart';
import 'server_config_page.dart';
import 'hxpt_web_login_page.dart';

part 'loginopsmixin.dart';
part 'login_widgets.dart';

/// 登录页：双模 —— 好学账号（验证码两步 + 2FA）/ Cookie 登录。
class LoginPage extends StatefulWidget {
  const LoginPage({super.key});
  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> with LoginOpsMixin {

  @override
  Future<void> _onLoginOk(Map<String, dynamic> res) async {
    await ApiClient().setToken(res['token'] as String?);
    final u = res['user'];
    if (u is Map<String, dynamic>) {
      await ApiClient().setUser(u); // 持久化身份（用户名/admin 徽章），重启后恢复
    }
    if (!mounted) return;
    Navigator.of(
      context,
    ).pushReplacement(MaterialPageRoute(builder: (_) => const HomeShell()));
  }
  @override
  void initState() {
    super.initState();
    // 首屏先保持安静：验证码依赖后端/上游站点，失败不应在用户选择登录方式前制造错误感。
    Future.microtask(() => _fetchCaptcha(silent: true));
    Future.microtask(_fetchAuthStatus);
  }

  Uint8List? get _captchaBytes {
    final s = _captcha?['image'] as String?;
    if (s == null || s.isEmpty) return null;
    final b64 = s.contains(',') ? s.split(',').last : s;
    try {
      return base64Decode(b64);
    } catch (_) {
      return null;
    }
  }

  @override
  void dispose() {
    _user.dispose();
    _pass.dispose();
    _twoStep.dispose();
    _imgStr.dispose();
    _cookie.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: GlassBackground(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: PTEntrance(
              child: GlassCard(
              padding: const EdgeInsets.all(28),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 360),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Row(
                      children: [
                        PTLogo(size: 30),
                        SizedBox(width: 10),
                        Text(
                          'PTPatronus',
                          style: TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      'PT 守护神 · 好学 APP 专属',
                      style: TextStyle(color: context.dimText),
                    ),
                    if (_oidcEnabled == false && _authSiteOn)
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Text(
                          '本服务面向好学 (hxpt.org) APP 专属 VIP 用户；请先在好学获得 VIP 身份后再登录。首个通过验证的用户将成为管理员。',
                          textAlign: TextAlign.center,
                          style: TextStyle(color: context.dimText, fontSize: 11, height: 1.4),
                        ),
                      ),
                    const SizedBox(height: 20),
                    SegmentedButton<int>(
                      segments: const [
                        ButtonSegment(value: 0, label: Text('好学账号')),
                        ButtonSegment(value: 1, label: Text('Cookie 登录')),
                      ],
                      selected: {_tab},
                      onSelectionChanged: (s) => setState(() {
                        _tab = s.first;
                        _err = null;
                      }),
                    ),
                    const SizedBox(height: 16),
                    if (_tab == 0) ..._accountFields() else ..._cookieFields(),
                    if (_demoMode) ...[
                      const SizedBox(height: 10),
                      OutlinedButton.icon(
                        onPressed: _loading ? null : _demoLogin,
                        icon: const Icon(Icons.visibility_outlined, size: 18),
                        label: const Text('浏览演示环境'),
                      ),
                    ],
                    if (_oidcEnabled) ...[
                      const SizedBox(height: 10),
                      OutlinedButton.icon(
                        onPressed: _loading ? null : _oidcLogin,
                        icon: const Icon(Icons.login_outlined, size: 18),
                        label: Text('$_oidcName 登录'),
                      ),
                    ],
                    if (_err != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 10),
                        child: Text(
                          _err!,
                          style: const TextStyle(
                            color: Colors.redAccent,
                            fontSize: 13,
                          ),
                        ),
                      ),
                    const SizedBox(height: 10),
                    // 切换后端地址（分发场景：每个用户后端不同）
                    Center(
                      child: TextButton.icon(
                        onPressed: () => Navigator.of(context).push(
                          MaterialPageRoute(
                            builder: (_) => const ServerConfigPage(),
                          ),
                        ),
                        icon: const Icon(Icons.dns_outlined, size: 16),
                        label: Text(
                          '后端：${ApiClient().baseUrl}',
                          style: TextStyle(
                            fontSize: 12,
                            color: context.dimText,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            ),
          ),
        ),
      ),
    );
  }

  List<Widget> _cookieFields() {
    return [
      TextField(
        controller: _cookie,
        maxLines: 5,
        decoration: const InputDecoration(
          labelText: '好学 cookie',
          hintText: '浏览器登录好学后复制 cookie（c_secure_uid=...; c_secure_pass=...）',
        ),
      ),
      const SizedBox(height: 8),
      Text(
        '最稳路径，覆盖开了二级验证 / 挑战响应的好学账号',
        style: TextStyle(fontSize: 12, color: context.dimText),
      ),
      Align(
        alignment: Alignment.centerLeft,
        child: TextButton.icon(
          onPressed: () async {
            // APP 内浏览器登录好学 → 自动取 cookie 回填，免手动 F12（手机端友好）
            final cookie = await Navigator.of(context).push<String>(
              MaterialPageRoute(builder: (_) => const HxptWebLoginPage()),
            );
            if (cookie != null && cookie.isNotEmpty && mounted) {
              setState(() => _cookie.text = cookie);
            }
          },
          icon: const Icon(Icons.open_in_browser, size: 18),
          label: const Text('用浏览器登录好学（自动取 cookie）'),
        ),
      ),
      const SizedBox(height: 14),
      FilledButton(
        onPressed: _loading ? null : _cookieLogin,
        child: _loading
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Text('登录'),
      ),
    ];
  }
}
