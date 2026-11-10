// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:media_kit/media_kit.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'features/home/home_shell.dart';
import 'features/login/login_page.dart';
import 'features/login/server_config_page.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await ApiClient().loadToken();
  await ApiClient().loadBaseUrl();
  // 401 全局清洗回调：断 WS（停旧 token 重连循环）+ 重算 home（token 已清 → 回登录页）。
  // 通过静态回调注入，避免 core(api_client) → features/ws 反向 import 循环。
  ApiClient.onSessionExpired = () {
    try {
      WsClient().disconnect();
    } catch (_) {}
    PTPatronusApp.reload();
  };
  // 玻璃降级用户偏好（性能模式）；覆盖 Web 默认。try-catch 防 SharedPreferences 平台异常致白屏（与 MediaKit 同）。
  try {
    await loadGlassPrefs();
  } catch (_) {}
  // 主题模式偏好（跟随系统 / 浅色 / 深色）
  try {
    await loadThemeModePrefs();
  } catch (_) {}
  // media_kit（摄像头 RTSP→HLS 预览）；鸿蒙无原生实现时跳过，播放器运行时降级提示。
  try {
    MediaKit.ensureInitialized();
  } catch (_) {}
  runApp(PTPatronusApp(key: appRootKey));
}

/// App 根 GlobalKey：跨页面触发 home 重算，不依赖 findAncestorStateOfType(context)
/// （release AOT + MaterialApp 内嵌 Navigator 下，从 home 页 context 向上查 AppState 偶尔取不到，
///  保存后端地址后 reload 成 no-op → 卡在服务器配置页进不去）。
final appRootKey = GlobalKey<_PTPatronusAppState>(debugLabel: 'PTPatronusApp');

class PTPatronusApp extends StatefulWidget {
  const PTPatronusApp({super.key});
  @override
  State<PTPatronusApp> createState() => _PTPatronusAppState();

  /// 触发 home 重算（后端地址配置变化、登录态变化后调用）。
  static void reload([BuildContext? _]) {
    final s = appRootKey.currentState;
    debugPrint('[reload] currentState=${s != null}');
    s?.reload();
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
    return ValueListenableBuilder<ThemeMode>(
      valueListenable: themeModeNotifier,
      builder: (context, mode, _) => MaterialApp(
        title: 'PTPatronus',
        debugShowCheckedModeBanner: false,
        // i18n：业务文案为中文硬编码（产品定位中文用户），这里固定 zh locale 让
        // Material/日期选择器等框架组件文案与业务文案语言一致——否则非中文系统
        // 下日期控件是英文、页面是中文的混杂体验（移动端审查 P1-4）。
        locale: const Locale('zh', 'CN'),
        supportedLocales: const [Locale('zh', 'CN'), Locale('en', 'US')],
        localizationsDelegates: const [
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        theme: buildAppTheme(Brightness.light),
        darkTheme: buildAppTheme(Brightness.dark),
        themeMode: mode,
        home: home,
      ),
    );
  }
}
