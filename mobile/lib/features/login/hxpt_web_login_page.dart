import 'package:flutter/material.dart';
import 'package:flutter_inappwebview/flutter_inappwebview.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

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
  InAppWebViewController? _ctrl;
  bool _busy = false;
  double _progress = 0;

  // 仅放行好学域内跳转，拦截 302/meta-refresh/window.location 跳到外部钓鱼页（审计 P1）
  Future<NavigationActionPolicy> _shouldOverride(InAppWebViewController c, NavigationAction nav) async {
    final host = nav.request.url?.host ?? '';
    return host.endsWith('hxpt.org') ? NavigationActionPolicy.ALLOW : NavigationActionPolicy.CANCEL;
  }

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
    return PopScope(
      // 返回键：WebView 内 goBack；无历史才退出页面（避免按返回直接丢登录流程——审计 P1）
      canPop: false,
      onPopInvokedWithResult: (didPop, _) async {
        if (didPop) return;
        if (_ctrl != null && await _ctrl!.canGoBack()) {
          await _ctrl!.goBack();
        } else {
          if (!context.mounted) return;
          Navigator.of(context).pop();
        }
      },
      child: Scaffold(
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
              onWebViewCreated: (c) => _ctrl = c,
              shouldOverrideUrlLoading: _shouldOverride,
              onProgressChanged: (_, p) => setState(() => _progress = p / 100),
            ),
            if (_progress > 0 && _progress < 1)
              LinearProgressIndicator(value: _progress, minHeight: 3),
            if (_busy)
              Container(
                color: Colors.black38,
                child: const PTLoadingView(),
              ),
          ],
        ),
      ),
    );
  }
}
