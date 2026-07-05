// 基础冒烟测试：登录页可构建 + 品牌 + 账号/Cookie 分段切换存在。
//
// 直接 pump LoginPage（而非 PTPatronusApp）：后者在 _runtimeBaseUrl 未设置（测试不调
// loadBaseUrl）时会路由到 ServerConfigPage，登录页的 SegmentedButton 便不在树中。
// 登录页自带 brand（PTLogo + "PTPatronus"）与账号/Cookie 分段，直接验证即可。
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
    expect(find.byType(SegmentedButton<int>), findsOneWidget);
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
    expect(find.text('搜索'), findsOneWidget);
    expect(find.text('仪表盘'), findsOneWidget);
  });
}
