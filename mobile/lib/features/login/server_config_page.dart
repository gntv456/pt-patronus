import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/config.dart';
import 'package:ptpatronus/core/log.dart';
import 'package:ptpatronus/main.dart';
import 'package:ptpatronus/theme/glass.dart';

/// 服务器配置页：填后端地址 → 测试连接 → 保存。
/// 分发场景必需——每个用户/部署的后端地址不同，不能靠编译期 --dart-define=API_BASE_URL。
/// 首次启动（未配过后端）由此页引导；登录页「切换服务器」也跳到这里。
class ServerConfigPage extends StatefulWidget {
  const ServerConfigPage({super.key});
  @override
  State<ServerConfigPage> createState() => _ServerConfigPageState();
}

class _ServerConfigPageState extends State<ServerConfigPage> {
  final _url = TextEditingController();
  bool _busy = false;
  String? _msg;
  bool _ok = false;
  String? _testedUrl;
  bool _allowInsecure = false;

  @override
  void initState() {
    super.initState();
    _url.text = ApiClient().baseUrl;
    _allowInsecure = ApiClient().allowInsecureHttp;
    _url.addListener(_onUrlChanged);
  }

  @override
  void dispose() {
    _url.removeListener(_onUrlChanged);
    _url.dispose();
    super.dispose();
  }

  String _trim(String s) => s.trim().replaceAll(RegExp(r'/+$'), '');

  void _onUrlChanged() {
    final changedAfterTest = _ok && _testedUrl != _trim(_url.text);
    if (!changedAfterTest) return;
    setState(() {
      _ok = false;
      _msg = '地址已修改，请重新测试连接';
    });
  }

  /// 校验后端地址：必须有 scheme+host；http 仅允许私网（公网 http 凭据可被嗅探）。
  /// 勾选「允许公网 http」后跳过私网检查（自负风险）。返回 null=合法，否则返回错误提示。
  String? _validateUrl(String url) {
    final u = Uri.tryParse(url);
    if (u == null ||
        u.host.isEmpty ||
        (u.scheme != 'http' && u.scheme != 'https')) {
      return '请填合法地址（以 http:// 或 https:// 开头）';
    }
    final h = u.host;
    final isPrivate =
        h == 'localhost' ||
        h == '127.0.0.1' ||
        h == '0.0.0.0' ||
        h == '10.0.2.2' || // Android 模拟器→宿主机
        h.startsWith('10.') ||
        h.startsWith('192.168.') ||
        RegExp(r'^172\.(1[6-9]|2\d|3[01])\.').hasMatch(h);
    if (u.scheme == 'http' && !isPrivate && !_allowInsecure) {
      return '公网地址请用 https://，否则登录凭据在共享网络下可被窃听';
    }
    return null;
  }

  Future<void> _test() async {
    final url = _trim(_url.text);
    if (url.isEmpty) {
      setState(() => _msg = '请填后端地址');
      return;
    }
    final verr = _validateUrl(url);
    if (verr != null) {
      setState(() => _msg = '✗ $verr');
      return;
    }
    setState(() {
      _busy = true;
      _msg = '测试中...';
      _ok = false;
    });
    try {
      final r = await http
          .get(Uri.parse('$url/api/v1/system/health'))
          .timeout(const Duration(seconds: 8));
      if (r.statusCode == 200) {
        setState(() {
          _msg = '✓ 连接成功';
          _ok = true;
          _testedUrl = url;
        });
      } else {
        setState(() => _msg = '✗ 后端返回 HTTP ${r.statusCode}');
      }
    } catch (e) {
      setState(() => _msg = '✗ 连接失败：$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save() async {
    final url = _trim(_url.text);
    ptLog('[save] enter, url="$url"');
    if (url.isEmpty) {
      setState(() => _msg = '请填后端地址');
      return;
    }
    final verr = _validateUrl(url);
    if (verr != null) {
      setState(() => _msg = '✗ $verr');
      return;
    }
    if (!_ok || _testedUrl != url) {
      setState(() => _msg = '请先测试连接成功，再保存进入');
      return;
    }
    try {
      await ApiClient().setBaseUrl(url);
      ptLog(
        '[save] setBaseUrl OK -> baseUrl=${ApiClient().baseUrl} hasCustom=${ApiClient().hasCustomBaseUrl}',
      );
      if (!mounted) return;
      // 保存后重算 app home：已配 + 未登录 → LoginPage（reload 走 appRootKey，稳）。
      PTPatronusApp.reload(context);
    } catch (e, st) {
      ptLog('[save] EXCEPTION: $e\n$st');
      // release 下异步回调异常会被静默吞掉 → 显式提示，避免「点了没反应」。
      if (mounted) setState(() => _msg = '✗ 保存失败：$e');
    }
  }

  @override
  Widget build(BuildContext context) {
    final compact = MediaQuery.widthOf(context) < 390;
    final pagePadding = compact ? 16.0 : 24.0;
    final cardPadding = compact ? 18.0 : 24.0;
    return Scaffold(
      body: GlassBackground(
        child: Center(
          child: SingleChildScrollView(
            padding: EdgeInsets.all(pagePadding),
            child: GlassCard(
              padding: EdgeInsets.all(cardPadding),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 380),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Row(
                      children: [
                        Icon(
                          Icons.shield_outlined,
                          size: 30,
                          color: PTColors.primary,
                        ),
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
                      '配置后端服务器地址',
                      style: TextStyle(color: context.dimText),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      '填入你部署的 docker 服务端地址（如 http://192.168.1.10:8088 或 https://你的域名），'
                      '客户端通过它连接后端。',
                      style: TextStyle(
                        color: context.dimText,
                        fontSize: 12,
                        height: 1.5,
                      ),
                    ),
                    const SizedBox(height: 20),
                    TextField(
                      controller: _url,
                      decoration: const InputDecoration(
                        labelText: '后端地址',
                        hintText: 'http://服务器IP:8088',
                        border: OutlineInputBorder(),
                      ),
                      keyboardType: TextInputType.url,
                      autocorrect: false,
                    ),
                    const SizedBox(height: 12),
                    Row(
                      children: [
                        Switch(
                          value: _allowInsecure,
                          onChanged: (v) async {
                            await ApiClient().setAllowInsecureHttp(v);
                            setState(() {
                              _allowInsecure = v;
                              _ok = false;
                              _msg = v ? '已放行公网 http：凭据将明文传输，风险自负' : null;
                            });
                          },
                        ),
                        const Expanded(
                          child: Text(
                            '允许公网 HTTP（不安全，凭据可被窃听）',
                            style: TextStyle(
                              color: Colors.orange,
                              fontSize: 12,
                            ),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 12),
                    if (compact)
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          OutlinedButton(
                            onPressed: _busy ? null : _test,
                            child: const Text('测试连接'),
                          ),
                          const SizedBox(height: 8),
                          FilledButton(
                            onPressed: _busy ? null : _save,
                            child: const Text('保存并进入'),
                          ),
                        ],
                      )
                    else
                      Row(
                        children: [
                          TextButton(
                            onPressed: _busy ? null : _test,
                            child: const Text('测试连接'),
                          ),
                          const Spacer(),
                          FilledButton(
                            onPressed: _busy ? null : _save,
                            child: const Text('保存并进入'),
                          ),
                        ],
                      ),
                    if (_msg != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: Text(
                          _msg!,
                          style: TextStyle(
                            color: _ok ? Colors.green : Colors.red,
                            fontSize: 13,
                          ),
                        ),
                      ),
                    const SizedBox(height: 16),
                    Text(
                      '编译期默认：${Config.baseUrl}',
                      style: TextStyle(color: context.dimText, fontSize: 11),
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
}
