// LoginPage 的 LoginOpsMixin mixin（含 setState 段；从 login_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/login/login_page.dart';

mixin LoginOpsMixin on State<LoginPage> {
  // 宿主提供的钩子。
  Future<void> _onLoginOk(Map<String, dynamic> res);

  bool _loading = false;
  bool _demoMode = false;
  bool _oidcEnabled = false;
  String _oidcName = 'OIDC / SSO';
  String? _err;
  // 本地密码登录
  final _user = TextEditingController();
  final _pass = TextEditingController();
  Future<void> _fetchAuthStatus() async {
    try {
      final s = await ApiClient().get('/auth/status');
      if (!mounted) return;
      setState(() {
        _demoMode = s['demo_mode'] == true;
        _oidcEnabled = s['oidc_enabled'] == true;
        _oidcName = (s['oidc_display_name'] as String?)?.isNotEmpty == true
            ? (s['oidc_display_name'] as String)
            : 'OIDC / SSO';
      });
    } catch (_) {}
  }

  // 本地密码登录（好学站点关闭后的主路径，#<issue>）：首次登录自动建号。
  Future<void> _localLogin() async {
    if (_user.text.isEmpty || _pass.text.isEmpty) {
      setState(() => _err = '请填写用户名和密码');
      return;
    }
    setState(() {
      _loading = true;
      _err = null;
    });
    try {
      final res = await ApiClient().post('/auth/local/login', {
        'username': _user.text.trim(),
        'password': _pass.text,
      });
      await _onLoginOk(res);
    } on ApiException catch (e) {
      if (mounted) setState(() => _err = e.message);
    } catch (_) {
      if (mounted) {
        setState(() => _err = '网络错误，请确认后端地址是否正确、后端服务是否已启动');
      }
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
