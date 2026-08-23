import 'package:flutter/material.dart';
import 'package:flutter_inappwebview/flutter_inappwebview.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/api/site.dart';

// WebView 登录页：用户在内嵌浏览器手动登录站点，点「完成」提取全部 cookie
// （含 HttpOnly）拼接上传后端 /sites/:id/cookie，后端 AES 加密存储。
// 这是移动端（含鸿蒙）获取站点 cookie 的唯一可靠途径（Web 端受 CORS 限制）。
class WebViewLoginPage extends StatefulWidget {
  final Site site;
  const WebViewLoginPage({super.key, required this.site});

  @override
  State<WebViewLoginPage> createState() => _WebViewLoginPageState();
}

class _WebViewLoginPageState extends State<WebViewLoginPage> {
  InAppWebViewController? _ctrl;
  bool _extracting = false;
  double _progress = 0;

  // 仅放行该站点域内跳转，拦截外部钓鱼（审计 P1）
  Future<NavigationActionPolicy> _shouldOverride(InAppWebViewController c, NavigationAction nav) async {
    final host = nav.request.url?.host ?? '';
    final siteHost = Uri.tryParse(widget.site.baseUrl)?.host ?? '';
    if (siteHost.isEmpty) return NavigationActionPolicy.ALLOW;
    return (host == siteHost || host.endsWith('.$siteHost'))
        ? NavigationActionPolicy.ALLOW
        : NavigationActionPolicy.CANCEL;
  }

  Future<void> _done() async {
    setState(() => _extracting = true);
    try {
      final cookieManager = CookieManager.instance();
      final cookies = await cookieManager.getCookies(url: WebUri(widget.site.baseUrl));
      final cookieStr = cookies.map((c) => '${c.name}=${c.value}').join('; ');
      if (cookieStr.isEmpty) {
        _toast('未取到 cookie，请先在页面完成登录');
        return;
      }
      await SiteApi().uploadCookie(widget.site.id, cookieStr);
      if (!mounted) return;
      _toast('Cookie 已上传（${cookies.length} 条）');
      Navigator.of(context).pop(true);
    } catch (e) {
      _toast('上传失败：$e');
    } finally {
      if (mounted) setState(() => _extracting = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return PopScope(
      // 返回键：WebView 内 goBack；无历史才退出页面（审计 P1）
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
          title: Text('登录 ${widget.site.name}'),
          actions: [
            TextButton(
              onPressed: _extracting ? null : _done,
              child: const Text('完成，上传 cookie', style: TextStyle(color: Colors.white)),
            ),
          ],
        ),
        body: Stack(
          children: [
            InAppWebView(
              initialUrlRequest: URLRequest(url: WebUri(widget.site.baseUrl)),
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
            if (_extracting)
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
