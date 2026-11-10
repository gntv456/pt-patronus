// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/cloud.dart';
import 'package:ptpatronus/api/file_storage.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/features/file_manage/file_manage_page.dart';

// /storages 整组是 RequireAdmin，而这一页同时挂着「存储后端」与「云盘」两侧读数。
// 旧写法是 `await _api.list()` 先抛（非管理员必然 403），云盘那张清单就永远拿不到，
// 界面只剩一句「暂无存储后端，请点右上角更多添加」——普通用户既读不到也添加不了。
// 注：注入「必挂」的一路必须用 Future<T>.error(...) 而不是 `() async => throw ...`。
// 后者的静态类型是 Future<Never>，catchError 的返回值过不了「必须是 future 的类型」这条运行时检查，
// 会冒出 ArgumentError 走外层 catch——用例照样红/照样绿，但走的是产品里根本不会有的路径（假账）。
void main() {
  tearDown(() => ApiClient().user = null);

  Future<void> pumpWith(
    WidgetTester t, {
    required bool isAdmin,
    Future<List<FileStorage>> Function()? storages,
    Future<List<CloudDrive>> Function()? drives,
  }) async {
    ApiClient().user = {'is_admin': isAdmin};
    await t.pumpWidget(
      MaterialApp(
        home: FileManagePage(listStorages: storages, listDrives: drives),
      ),
    );
    await t.pump();
    await t.pump(const Duration(milliseconds: 50));
  }

  String bodyText(WidgetTester t) =>
      (t.widgetList<Text>(find.byType(Text)).map((x) => x.data ?? '').join(' '));

  testWidgets('存储后端读取失败：要说清原因，不能推给「你去添加」', (t) async {
    await pumpWith(
      t,
      isAdmin: true,
      storages: () => Future<List<FileStorage>>.error(Exception('boom')),
      drives: () async => <CloudDrive>[],
    );
    expect(bodyText(t), contains('存储后端读取失败'));
    expect(bodyText(t), isNot(contains('请点右上角更多添加')));
  });

  testWidgets('非管理员：说「仅管理员可见」，也不发必然 403 的存储请求', (t) async {
    var called = false;
    await pumpWith(
      t,
      isAdmin: false,
      storages: () async {
        called = true;
        return <FileStorage>[];
      },
      drives: () async => <CloudDrive>[],
    );
    expect(called, isFalse);
    expect(bodyText(t), contains('存储后端仅管理员可见'));
    expect(bodyText(t), isNot(contains('请点右上角更多添加')));
  });

  testWidgets('云盘侧失败同样要出声（旧写法吞成空数组，什么都不说）', (t) async {
    await pumpWith(
      t,
      isAdmin: true,
      storages: () async => <FileStorage>[],
      drives: () => Future<List<CloudDrive>>.error(Exception('drives 500')),
    );
    expect(bodyText(t), contains('云盘列表读取失败'));
  });

  // 隔离性本身：fs 侧炸了，云盘那台仍然要进清单并显示在后端下拉里
  // （旧写法 `await _api.list()` 先抛，清单为空，界面只剩一句「去添加存储」）。
  testWidgets('存储后端失败不拖没云盘：云盘照常进后端下拉', (t) async {
    await pumpWith(
      t,
      isAdmin: true,
      storages: () => Future<List<FileStorage>>.error(Exception('boom')),
      drives: () async => [
        CloudDrive(
          id: 7,
          name: '我的115',
          type: '115',
          rootPath: '/',
          isDefault: true,
          createdAt: DateTime(2026, 10, 13),
          updatedAt: DateTime(2026, 10, 13),
        ),
      ],
    );
    // 选中的云盘后端会立刻发起一次真实列目录（测试里必然失败），
    // 给它一拍落地，避免断言跑在半成品帧上。
    await t.pump(const Duration(seconds: 1));
    expect(find.textContaining('我的115'), findsWidgets, reason: '云盘后端必须出现在后端下拉里');
    expect(bodyText(t), contains('存储后端读取失败'));
    expect(bodyText(t), isNot(contains('请点右上角更多添加')));
  });
}
