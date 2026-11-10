// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/system.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/features/assistant/assistant_page.dart';
import 'package:ptpatronus/features/doctor/doctor_page.dart';
import 'package:ptpatronus/features/system/logs_page.dart';

// AI 助手 / 运行日志 / 诊断三页背后都是 RequireAdmin（GET /agent/*、/system/logs、/system/doctor），
// 入口却挂在顶栏与命令面板上。这里证两件事：非管理员只剩一句说明（且没发请求），
// 以及诊断「跑挂了」不能再画成「没有数据」。
void main() {
  tearDown(() => ApiClient().user = null); // 单例跨用例，别忘了清账

  Future<void> pumpRole(WidgetTester t, Widget page, bool isAdmin) async {
    ApiClient().user = {'is_admin': isAdmin};
    await t.pumpWidget(MaterialApp(home: page));
    await t.pump();
    await t.pump(const Duration(milliseconds: 50));
  }

  testWidgets('非管理员：三页只说「仅管理员可访问」', (t) async {
    for (final page in [const AssistantPage(), const LogsPage(), const DoctorPage()]) {
      await pumpRole(t, page, false);
      expect(find.text('仅管理员可访问'), findsOneWidget);
    }
    // 诊断页的两个动作按钮也是必然 403 的写操作
    await pumpRole(t, const DoctorPage(), false);
    expect(find.byTooltip('重新诊断'), findsNothing);
    expect(find.byTooltip('尝试修复'), findsNothing);
  });

  testWidgets('诊断跑失败：说「诊断失败：原因」，不是「无数据」', (t) async {
    await pumpRole(
      t,
      DoctorPage(run: (_) async => throw Exception('需要管理员权限')),
      true,
    );
    expect(find.textContaining('诊断失败'), findsOneWidget);
    expect(find.text('无数据'), findsNothing);
  });

  testWidgets('正控：管理员拿到报告就照常渲染发现清单', (t) async {
    await pumpRole(
      t,
      DoctorPage(
        run: (_) async => const DoctorReport(status: 'degraded', total: 2, warn: 2),
      ),
      true,
    );
    expect(find.textContaining('状态：degraded'), findsOneWidget);
    expect(find.textContaining('诊断失败'), findsNothing);
  });
}
