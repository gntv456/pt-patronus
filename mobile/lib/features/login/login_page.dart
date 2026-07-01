import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_logo.dart';
import '../home/home_shell.dart';
import 'server_config_page.dart';
import 'hxpt_web_login_page.dart';

/// 登录页：双模 —— 好学账号（验证码两步 + 2FA）/ Cookie 登录。
class LoginPage extends StatefulWidget {
  const LoginPage({super.key});
  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> {
  int _tab = 0; // 0=账号 1=cookie
  bool _loading = false;
  String? _err;

  // 账号登录
  final _user = TextEditingController();
  final _pass = TextEditingController();
  final _twoStep = TextEditingController();
  final _imgStr = TextEditingController();
  Map<String, dynamic>? _captcha; // {imagehash, image, need_captcha, challenge_mode, extra_form}

  // Cookie 登录
  final _cookie = TextEditingController();

  @override
  void initState() {
    super.initState();
    _fetchCaptcha();
  }

  Future<void> _fetchCaptcha() async {
    try {
      final c = await ApiClient().post('/auth/hxpt/captcha', <String, dynamic>{});
      if (!mounted) return;
      setState(() {
        _captcha = c;
        if (c['challenge_mode'] == true) _tab = 1; // 挑战响应模式 → 引导 cookie
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _err = '取验证码失败：${e.message}（可改用 Cookie 登录）');
    } catch (_) {
      if (!mounted) return;
      setState(() => _err = '取验证码失败，可改用 Cookie 登录');
    }
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

  Future<void> _accountLogin() async {
    if (_user.text.isEmpty || _pass.text.isEmpty) {
      setState(() => _err = '请填写好学账号和密码');
      return;
    }
    if (_captcha?['challenge_mode'] == true) {
      setState(() {
        _tab = 1;
        _err = '好学已开启挑战响应认证，请改用 Cookie 登录';
      });
      return;
    }
    setState(() {
      _loading = true;
      _err = null;
    });
    try {
      final needCap = _captcha?['need_captcha'] == true;
      final res = await ApiClient().post('/auth/hxpt/login', {
        'username': _user.text,
        'password': _pass.text,
        // 显式回传 challenge_mode（与 Web 端一致）：当前端判 false 才发，但保留字段避免后端默认值漂移。
        'challenge_mode': _captcha?['challenge_mode'] ?? false,
        if (_twoStep.text.isNotEmpty) 'two_step_code': _twoStep.text,
        if (needCap) 'imagehash': _captcha?['imagehash'] ?? '',
        if (needCap) 'imagestring': _imgStr.text,
        if (_captcha?['extra_form'] != null) 'extra_form': _captcha!['extra_form'],
      });
      await _onLoginOk(res);
    } on ApiException catch (e) {
      if (mounted) setState(() => _err = e.message);
      if (e.status == 401) {
        // 凭据/验证码错 → 刷新验证码
        _imgStr.clear();
        await _fetchCaptcha();
      }
    } catch (_) {
      if (mounted) setState(() => _err = '网络错误，请确认后端地址（Config.baseUrl）');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _cookieLogin() async {
    if (_cookie.text.trim().isEmpty) {
      setState(() => _err = '请粘贴好学 cookie');
      return;
    }
    setState(() {
      _loading = true;
      _err = null;
    });
    try {
      final res = await ApiClient().post('/auth/hxpt/cookie-login', {
        'cookie': _cookie.text.trim(),
      });
      await _onLoginOk(res);
    } on ApiException catch (e) {
      if (mounted) setState(() => _err = e.message);
    } catch (_) {
      if (mounted) setState(() => _err = '网络错误，请确认后端地址（Config.baseUrl）');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _onLoginOk(Map<String, dynamic> res) async {
    await ApiClient().setToken(res['token'] as String?);
    final u = res['user'];
    if (u is Map<String, dynamic>) {
      await ApiClient().setUser(u); // 持久化身份（用户名/admin 徽章），重启后恢复
    }
    if (!mounted) return;
    Navigator.of(context).pushReplacement(
      MaterialPageRoute(builder: (_) => const HomeShell()),
    );
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
            child: GlassCard(
              padding: const EdgeInsets.all(28),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 360),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Row(children: [
                      PTLogo(size: 30),
                      SizedBox(width: 10),
                      Text('PTPatronus',
                          style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700)),
                    ]),
                    const SizedBox(height: 4),
                    const Text('PT 守护神 · 好学 APP 专属',
                        style: TextStyle(color: Colors.grey)),
                    const SizedBox(height: 20),
                    SegmentedButton<int>(
                      segments: const [
                        ButtonSegment(value: 0, label: Text('好学账号')),
                        ButtonSegment(value: 1, label: Text('Cookie 登录')),
                      ],
                      selected: {_tab},
                      onSelectionChanged: (s) => setState(() => _tab = s.first),
                    ),
                    const SizedBox(height: 16),
                    if (_tab == 0) ..._accountFields() else ..._cookieFields(),
                    if (_err != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 10),
                        child: Text(_err!,
                            style: const TextStyle(color: Colors.redAccent, fontSize: 13)),
                      ),
                    const SizedBox(height: 10),
                    // 切换后端地址（分发场景：每个用户后端不同）
                    Center(
                      child: TextButton.icon(
                        onPressed: () => Navigator.of(context).push(
                          MaterialPageRoute(builder: (_) => const ServerConfigPage()),
                        ),
                        icon: const Icon(Icons.dns_outlined, size: 16),
                        label: Text(
                          '后端：${ApiClient().baseUrl}',
                          style: const TextStyle(fontSize: 12, color: Colors.grey),
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
    );
  }

  List<Widget> _accountFields() {
    final needCap = _captcha?['need_captcha'] == true;
    return [
      TextField(
          controller: _user,
          decoration: const InputDecoration(labelText: '好学用户名')),
      const SizedBox(height: 12),
      TextField(
        controller: _pass,
        obscureText: true,
        decoration: const InputDecoration(labelText: '好学密码'),
      ),
      const SizedBox(height: 12),
      TextField(
        controller: _twoStep,
        decoration: const InputDecoration(
            labelText: '二级验证码（开了 2FA 才填，6 位）'),
      ),
      if (needCap) ...[
        const SizedBox(height: 12),
        Row(children: [
          if (_captchaBytes != null)
            GestureDetector(
              onTap: _fetchCaptcha,
              child: Image.memory(_captchaBytes!,
                  height: 40, gaplessPlayback: true),
            ),
          const SizedBox(width: 8),
          Expanded(
            child: TextField(
              controller: _imgStr,
              decoration: const InputDecoration(labelText: '验证码'),
            ),
          ),
        ]),
      ],
      const SizedBox(height: 22),
      FilledButton(
        onPressed: _loading ? null : _accountLogin,
        child: _loading
            ? const SizedBox(
                width: 18, height: 18,
                child: CircularProgressIndicator(strokeWidth: 2))
            : const Text('登录'),
      ),
      if (_captcha?['challenge_mode'] == true)
        const Padding(
          padding: EdgeInsets.only(top: 8),
          child: Text(
              '好学已开启挑战响应认证，账号登录不可用，请切到 Cookie 登录',
              style: TextStyle(fontSize: 12, color: Colors.grey)),
        ),
    ];
  }

  List<Widget> _cookieFields() {
    return [
      TextField(
        controller: _cookie,
        maxLines: 5,
        decoration: const InputDecoration(
            labelText: '好学 cookie',
            hintText: '浏览器登录好学后复制 cookie（c_secure_uid=...; c_secure_pass=...）'),
      ),
      const SizedBox(height: 8),
      const Text(
          '最稳路径，覆盖开了二级验证 / 挑战响应的好学账号',
          style: TextStyle(fontSize: 12, color: Colors.grey)),
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
                width: 18, height: 18,
                child: CircularProgressIndicator(strokeWidth: 2))
            : const Text('登录'),
      ),
    ];
  }
}
