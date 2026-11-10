// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 基础冒烟测试：登录页可构建 + 品牌与用户名/密码表单存在。
//
// 直接 pump LoginPage（而非 PTPatronusApp）：后者在 _runtimeBaseUrl 未设置（测试不调
// loadBaseUrl）时会路由到 ServerConfigPage，登录表单便不在树中。
// 好学旧 Tab（账号/Cookie 分段）已随站点改版下线，本地密码表单是唯一登录入口。
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_nav_panel.dart';
import 'package:ptpatronus/features/login/login_page.dart';
import 'package:ptpatronus/features/login/server_config_page.dart';

void main() {
  testWidgets('Login shows brand and tab toggle', (WidgetTester tester) async {
    await tester.pumpWidget(const MaterialApp(home: LoginPage()));
    await tester.pump();
    expect(find.text('PTPatronus'), findsOneWidget);
    expect(find.byType(TextField), findsAtLeastNWidgets(2)); // 用户名 + 密码
  });

  testWidgets('Server config requires successful test before save', (
    WidgetTester tester,
  ) async {
    await tester.pumpWidget(const MaterialApp(home: ServerConfigPage()));
    await tester.enterText(find.byType(TextField), 'http://192.168.1.2:8088');
    await tester.tap(find.text('保存并进入'));
    await tester.pump();
    expect(find.text('请先测试连接成功，再保存进入'), findsOneWidget);
  });

  testWidgets('Mobile nav lists destinations', (WidgetTester tester) async {
    final controller = HomeController();
    addTearDown(controller.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: HomeShellScope(
            controller: controller,
            child: const NavGroupedList(),
          ),
        ),
      ),
    );

    await tester.pump();
    // 搜索已移出抽屉（hidden=true），改由顶栏 FAB / 命令面板入口（见 home_shell.dart）。
    // 这里验证抽屉中实际可见的导航项。
    expect(find.text('仪表盘'), findsOneWidget);
    expect(find.text('站点'), findsOneWidget);
  });
}
