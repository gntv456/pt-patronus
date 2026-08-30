// LoginPage 的 LoginOpsMixin mixin（含 setState 段；从 login_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/login/login_page.dart';

mixin LoginOpsMixin on State<LoginPage> {
  // 宿主提供的钩子。
  Future<void> _onLoginOk(Map<String, dynamic> res);

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
}
