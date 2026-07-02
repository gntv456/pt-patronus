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

  @override
  void initState() {
    super.initState();
    _url.text = ApiClient().baseUrl;
  }

  @override
  void dispose() {
    _url.dispose();
    super.dispose();
  }

  String _trim(String s) => s.trim().replaceAll(RegExp(r'/+$'), '');

  /// 校验后端地址：必须有 scheme+host；http 仅允许私网（公网 http 凭据可被嗅探）。
  /// 返回 null=合法，否则返回错误提示。
  String? _validateUrl(String url) {
    final u = Uri.tryParse(url);
    if (u == null || u.host.isEmpty || (u.scheme != 'http' && u.scheme != 'https')) {
      return '请填合法地址（以 http:// 或 https:// 开头）';
    }
    final h = u.host;
    final isPrivate = h == 'localhost' || h == '127.0.0.1' || h == '0.0.0.0' ||
        h == '10.0.2.2' || // Android 模拟器→宿主机
        h.startsWith('10.') ||
        h.startsWith('192.168.') ||
        RegExp(r'^172\.(1[6-9]|2\d|3[01])\.').hasMatch(h);
    if (u.scheme == 'http' && !isPrivate) {
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
    try {
      await ApiClient().setBaseUrl(url);
      ptLog('[save] setBaseUrl OK -> baseUrl=${ApiClient().baseUrl} hasCustom=${ApiClient().hasCustomBaseUrl}');
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
                      Text('🛡️', style: TextStyle(fontSize: 30)),
                      SizedBox(width: 10),
                      Text('PTPatronus',
                          style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700)),
                    ]),
                    const SizedBox(height: 4),
                    const Text('配置后端服务器地址', style: TextStyle(color: Colors.grey)),
                    const SizedBox(height: 8),
                    const Text(
                        '填入你部署的 docker 服务端地址（如 http://192.168.1.10:8088 或 https://你的域名），'
                        '客户端通过它连接后端。',
                        style: TextStyle(color: Colors.grey, fontSize: 12, height: 1.5)),
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
                    Row(children: [
                      TextButton(
                          onPressed: _busy ? null : _test, child: const Text('测试连接')),
                      const Spacer(),
                      FilledButton(
                          onPressed: _busy ? null : _save, child: const Text('保存并进入')),
                    ]),
                    if (_msg != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: Text(_msg!,
                            style: TextStyle(
                                color: _ok ? Colors.green : Colors.red, fontSize: 13)),
                      ),
                    const SizedBox(height: 16),
                    Text('编译期默认：${Config.baseUrl}',
                        style: const TextStyle(color: Colors.grey, fontSize: 11)),
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
