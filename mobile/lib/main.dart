import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'features/home/home_shell.dart';
import 'features/login/login_page.dart';
import 'features/login/server_config_page.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await ApiClient().loadToken();
  await ApiClient().loadBaseUrl();
  // 玻璃降级用户偏好（性能模式）；覆盖 Web 默认。try-catch 防 SharedPreferences 平台异常致白屏（与 MediaKit 同）。
  try {
    await loadGlassPrefs();
  } catch (_) {}
  // media_kit（摄像头 RTSP→HLS 预览）；鸿蒙无原生实现时跳过，播放器运行时降级提示。
  try {
    MediaKit.ensureInitialized();
  } catch (_) {}
  runApp(const PTPatronusApp());
}

class PTPatronusApp extends StatefulWidget {
  const PTPatronusApp({super.key});
  @override
  State<PTPatronusApp> createState() => _PTPatronusAppState();

  /// 触发 home 重算（后端地址配置变化、登录态变化后调用）。
  static void reload(BuildContext context) {
    context.findAncestorStateOfType<_PTPatronusAppState>()?.reload();
  }
}

class _PTPatronusAppState extends State<PTPatronusApp> {
  void reload() => setState(() {});

  @override
  Widget build(BuildContext context) {
    final api = ApiClient();
    final Widget home;
    if (!api.hasCustomBaseUrl) {
      // 首次启动：未配后端地址 → 引导配置（分发场景每个用户后端不同）
      home = const ServerConfigPage();
    } else if (api.token != null && api.token!.isNotEmpty) {
      home = const HomeShell();
    } else {
      home = const LoginPage();
    }
    return MaterialApp(
      title: 'PTPatronus',
      debugShowCheckedModeBanner: false,
      theme: buildAppTheme(Brightness.light),
      darkTheme: buildAppTheme(Brightness.dark),
      home: home,
    );
  }
}
