import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_logo.dart';
import 'package:url_launcher/url_launcher.dart';
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
  bool _demoMode = false;
  bool _oidcEnabled = false;
  bool _authSiteOn = false; // 后端是否开启好学 VIP 准入（用于登录页显示准入说明）
  String _oidcName = 'OIDC / SSO';
  String? _err;

  // 账号登录
  final _user = TextEditingController();
  final _pass = TextEditingController();
  final _twoStep = TextEditingController();
  final _imgStr = TextEditingController();
  Map<String, dynamic>?
  _captcha; // {imagehash, image, need_captcha, challenge_mode, extra_form}

  // Cookie 登录
  final _cookie = TextEditingController();

  @override
  void initState() {
    super.initState();
    // 首屏先保持安静：验证码依赖后端/上游站点，失败不应在用户选择登录方式前制造错误感。
    Future.microtask(() => _fetchCaptcha(silent: true));
    Future.microtask(_fetchAuthStatus);
  }

  Future<void> _fetchAuthStatus() async {
    try {
      final s = await ApiClient().get('/auth/status');
      if (!mounted) return;
      setState(() {
        _demoMode = s['demo_mode'] == true;
        _oidcEnabled = s['oidc_enabled'] == true;
        _authSiteOn = s['auth_site_on'] == true;
        _oidcName = (s['oidc_display_name'] as String?)?.isNotEmpty == true
            ? (s['oidc_display_name'] as String)
            : 'OIDC / SSO';
      });
    } catch (_) {}
  }

  Future<bool> _fetchCaptcha({bool silent = false}) async {
    try {
      final c = await ApiClient().post(
        '/auth/hxpt/captcha',
        <String, dynamic>{},
      );
      if (!mounted) return false;
      setState(() {
        _captcha = c;
        if (!silent) _err = null;
        // 不再强制切 cookie：后端已实现挑战响应登录（见 nexusphp_crypto.go），账号登录可直接试，失败再切 cookie。
      });
      return true;
    } on ApiException catch (e) {
      if (!mounted) return false;
      if (!silent) {
        setState(() => _err = '取验证码失败：${e.message}（可改用 Cookie 登录）');
      }
    } catch (_) {
      if (!mounted) return false;
      if (!silent) {
        setState(() => _err = '取验证码失败，可改用 Cookie 登录');
      }
    }
    return false;
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
    if (_captcha == null) {
      final ok = await _fetchCaptcha();
      if (!ok) return;
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
        // 显式回传 challenge_mode（与 Web 端一致）：驱动后端挑战响应分支（nexusphp_crypto 算 response 提交）。
        'challenge_mode': _captcha?['challenge_mode'] ?? false,
        if (_twoStep.text.isNotEmpty) 'two_step_code': _twoStep.text,
        if (needCap) 'imagehash': _captcha?['imagehash'] ?? '',
        if (needCap) 'imagestring': _imgStr.text,
        if (_captcha?['extra_form'] != null)
          'extra_form': _captcha!['extra_form'],
      });
      await _onLoginOk(res);
    } on ApiException catch (e) {
      if (mounted) setState(() => _err = e.message);
      if (e.status == 409) {
        // 后端判定挑战响应模式账号登录不可用（兜底分支，正常跑不到）→ 引导切 Cookie
        if (mounted) setState(() => _tab = 1);
      } else if (e.status == 401) {
        // 凭据/验证码错 → 刷新验证码
        _imgStr.clear();
        await _fetchCaptcha(silent: true);
      }
    } catch (_) {
      if (mounted) setState(() => _err = '网络错误，请确认后端地址是否正确、后端服务是否已启动');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _cookieLogin() async {
    final ck = _cookie.text.trim();
    if (ck.isEmpty) {
      setState(() => _err = '请粘贴好学 cookie');
      return;
    }
    // 格式预校验：cookie 至少含一个「键=值」字段，避免提交明显非法串再等服务端拒（对齐 Web）。
    if (!ck.contains('=')) {
      setState(
        () =>
            _err = 'cookie 格式有误：应含键=值字段（如 c_secure_uid=...; c_secure_pass=...）',
      );
      return;
    }
    setState(() {
      _loading = true;
      _err = null;
    });
    try {
      final res = await ApiClient().post('/auth/hxpt/cookie-login', {
        'cookie': ck,
      });
      await _onLoginOk(res);
    } on ApiException catch (e) {
      if (mounted) setState(() => _err = e.message);
    } catch (_) {
      if (mounted) setState(() => _err = '网络错误，请确认后端地址是否正确、后端服务是否已启动');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _demoLogin() async {
    setState(() {
      _loading = true;
      _err = null;
    });
    try {
      final res = await ApiClient().post(
        '/auth/demo-login',
        <String, dynamic>{},
      );
      await _onLoginOk(res);
    } on ApiException catch (e) {
      if (mounted) setState(() => _err = e.message);
    } catch (_) {
      if (mounted) setState(() => _err = '演示登录未启用或后端不可达');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  // OIDC/SSO：移动端 deep-link（ptpatronus://）回调尚未接通——直接拉起系统浏览器会令牌丢失，
  // 用户卡在浏览器无法返回 App。改为弹说明引导用账号/Cookie 登录（保留「仍打开浏览器」兜底）。
  Future<void> _oidcLogin() async {
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('$_oidcName 暂不可用'),
        content: const Text(
          '移动端 SSO 自动回调（deep-link）尚未接通：在浏览器完成授权后，令牌无法自动回到 App。\n\n'
          '请改用「好学账号」或「Cookie」标签页登录。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: const Text('知道了'),
          ),
          TextButton(
            onPressed: () {
              Navigator.of(ctx).pop();
              final uri = Uri.tryParse(
                '${ApiClient().baseUrl}/api/v1/auth/oidc/start',
              );
              if (uri != null) {
                launchUrl(uri, mode: LaunchMode.externalApplication);
              }
            },
            child: const Text('仍打开浏览器'),
          ),
        ],
      ),
    );
  }

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
                    const Text(
                      'PT 守护神 · 好学 APP 专属',
                      style: TextStyle(color: Colors.grey),
                    ),
                    if (_oidcEnabled == false && _authSiteOn)
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Text(
                          '本服务面向好学 (hxpt.org) APP 专属 VIP 用户；请先在好学获得 VIP 身份后再登录。首个通过验证的用户将成为管理员。',
                          textAlign: TextAlign.center,
                          style: TextStyle(color: Colors.grey.shade600, fontSize: 11, height: 1.4),
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
                          style: const TextStyle(
                            fontSize: 12,
                            color: Colors.grey,
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
    );
  }

  List<Widget> _accountFields() {
    final needCap = _captcha?['need_captcha'] == true;
    return [
      TextField(
        controller: _user,
        decoration: const InputDecoration(labelText: '好学用户名'),
      ),
      const SizedBox(height: 12),
      TextField(
        controller: _pass,
        obscureText: true,
        decoration: const InputDecoration(labelText: '好学密码'),
      ),
      const SizedBox(height: 12),
      TextField(
        controller: _twoStep,
        decoration: const InputDecoration(labelText: '二级验证码（开了 2FA 才填，6 位）'),
      ),
      if (needCap) ...[
        const SizedBox(height: 12),
        Row(
          children: [
            if (_captchaBytes != null)
              GestureDetector(
                onTap: () => _fetchCaptcha(),
                child: Image.memory(
                  _captchaBytes!,
                  height: 40,
                  gaplessPlayback: true,
                ),
              ),
            const SizedBox(width: 8),
            Expanded(
              child: TextField(
                controller: _imgStr,
                decoration: const InputDecoration(labelText: '验证码'),
              ),
            ),
          ],
        ),
      ],
      const SizedBox(height: 22),
      FilledButton(
        onPressed: _loading ? null : _accountLogin,
        child: _loading
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Text('登录'),
      ),
      if (_captcha?['challenge_mode'] == true)
        const Padding(
          padding: EdgeInsets.only(top: 8),
          child: Text(
            '好学已开启挑战响应认证，后端会自动处理，直接登录即可；如登录失败可改用 Cookie 登录',
            style: TextStyle(fontSize: 12, color: Colors.grey),
          ),
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
          hintText: '浏览器登录好学后复制 cookie（c_secure_uid=...; c_secure_pass=...）',
        ),
      ),
      const SizedBox(height: 8),
      const Text(
        '最稳路径，覆盖开了二级验证 / 挑战响应的好学账号',
        style: TextStyle(fontSize: 12, color: Colors.grey),
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
