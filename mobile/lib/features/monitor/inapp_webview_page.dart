// 从 monitor_detail_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/monitor/monitor_detail_page.dart';

/// WebView 降级容器（鸿蒙无 media_kit 原生时，跳后端 Web 版看流）。
class InAppWebViewPage extends StatelessWidget {
  final String initialUrl;
  const InAppWebViewPage({super.key, required this.initialUrl});
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('实时画面（Web 版）')),
      body: InAppWebView(
        initialUrlRequest: URLRequest(url: WebUri(initialUrl)),
      ),
    );
  }
}
