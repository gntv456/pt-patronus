import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'features/home/home_shell.dart';
import 'features/login/login_page.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await ApiClient().loadToken();
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

class PTPatronusApp extends StatelessWidget {
  const PTPatronusApp({super.key});
  @override
  Widget build(BuildContext context) {
    final logged =
        ApiClient().token != null && ApiClient().token!.isNotEmpty;
    return MaterialApp(
      title: 'PTPatronus',
      debugShowCheckedModeBanner: false,
      theme: buildAppTheme(Brightness.light),
      darkTheme: buildAppTheme(Brightness.dark),
      home: logged ? const HomeShell() : const LoginPage(),
    );
  }
}
