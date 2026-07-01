import 'package:flutter/material.dart';
import 'package:flutter_inappwebview/flutter_inappwebview.dart';

/// 好学 WebView 登录页：用户在内嵌浏览器手动登录好学，点「完成」提取全部 cookie
/// （含 HttpOnly 的 c_secure_uid / c_secure_pass 等），以字符串返回给调用方
/// （登录页 Cookie tab 回填到 cookie 输入框，再走 /auth/hxpt/cookie-login）。
///
/// 比手动 F12 复制 cookie 省事，手机端尤其方便。复用 flutter_inappwebview
/// （与站点 WebViewLogin 同插件），但本页只回填、不上传后端。
class HxptWebLoginPage extends StatefulWidget {
  final String initialUrl;
  const HxptWebLoginPage({super.key, this.initialUrl = 'https://www.hxpt.org'});

  @override
  State<HxptWebLoginPage> createState() => _HxptWebLoginPageState();
}

class _HxptWebLoginPageState extends State<HxptWebLoginPage> {
  bool _busy = false;
  double _progress = 0;

  Future<void> _done() async {
    setState(() => _busy = true);
    try {
      final cookieManager = CookieManager.instance();
      final cookies =
          await cookieManager.getCookies(url: WebUri(widget.initialUrl));
      final cookieStr = cookies.map((c) => '${c.name}=${c.value}').join('; ');
      if (cookieStr.isEmpty) {
        _toast('未取到 cookie，请先在页面完成登录');
        return;
      }
      if (!mounted) return;
      Navigator.of(context).pop(cookieStr); // 回填给登录页
    } catch (e) {
      _toast('取 cookie 失败：$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('登录好学'),
        actions: [
          TextButton(
            onPressed: _busy ? null : _done,
            child: const Text('完成，取 cookie',
                style: TextStyle(color: Colors.white)),
          ),
        ],
      ),
      body: Stack(
        children: [
          InAppWebView(
            initialUrlRequest: URLRequest(url: WebUri(widget.initialUrl)),
            initialSettings: InAppWebViewSettings(
              javaScriptEnabled: true,
              useShouldOverrideUrlLoading: true,
            ),
            onProgressChanged: (_, p) => setState(() => _progress = p / 100),
          ),
          if (_progress > 0 && _progress < 1)
            LinearProgressIndicator(value: _progress, minHeight: 3),
          if (_busy)
            Container(
              color: Colors.black38,
              child: const Center(child: CircularProgressIndicator()),
            ),
        ],
      ),
    );
  }
}
