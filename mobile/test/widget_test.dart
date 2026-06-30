// 基础冒烟测试：应用可构建 + 登录页标题存在。
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/main.dart';

void main() {
  testWidgets('App builds and shows login brand', (WidgetTester tester) async {
    await tester.pumpWidget(const PTPatronusApp());
    await tester.pumpAndSettle();
    expect(find.text('PTPatronus'), findsOneWidget);
    expect(find.byType(SegmentedButton<int>), findsOneWidget);
  });
}
