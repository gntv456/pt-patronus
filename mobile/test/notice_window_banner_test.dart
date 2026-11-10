// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/notice.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/features/notice/notice_page.dart';

// 收件箱单页有上限（后端默认 200、封顶 1000）。#184 已经把 total/truncated 取回来并算成
// 一句窗口说明，这一轮验的是「那句话到底出没出现在界面上」：
// 只测 cappedListNotice 纯函数证不了接线——账面算得再对，不渲染就等于没说。
typedef NoticePageData = ({List<Notice> items, int? total, bool truncated});

// 动作面替身：#205/#206 之后后端对「已经不存在的通知」回 404，
// 页面必须把那句中文说出来——旧写法是 catch (_) {}，点了没反应、红点还在。
class _FakeApi extends NoticeApi {
  _FakeApi({this.readErr});
  final Object? readErr;
  @override
  Future<void> read(int id) {
    final e = readErr;
    return e == null ? Future.value() : Future<void>.error(e);
  }
}

void main() {
  Future<void> pump(
    WidgetTester t,
    Future<NoticePageData> Function() loadPage, {
    NoticeApi? api,
  }) async {
    await t.pumpWidget(
      MaterialApp(
        home: NoticePage(loadPage: loadPage, api: api),
      ),
    );
    await t.pump();
    await t.pump(const Duration(milliseconds: 80));
  }

  List<Notice> rows(int n) => List.generate(
    n,
    (i) => Notice(
      id: i + 1,
      source: 'announce',
      title: 'n$i',
      body: '',
      level: NoticeLevel.info,
      read: false,
      createdAt: DateTime(2026, 10, 13),
    ),
  );

  testWidgets('截断且总数已知：条数与总数同时说出来', (t) async {
    await pump(t, () async => (items: rows(200), total: 512, truncated: true));
    expect(find.textContaining('共 512 条'), findsOneWidget);
    expect(find.textContaining('仅显示最近 200 条'), findsOneWidget);
  });

  testWidgets('截断但总数读不出：说「可能还有更多」，不许伪造成共 0 条', (t) async {
    await pump(t, () async => (items: rows(200), total: null, truncated: true));
    expect(find.textContaining('可能还有更多'), findsOneWidget);
    // 判「伪造出的总数」而不是裸子串「0 条」——后者会被「最近 200 条」满足（假红）。
    expect(find.textContaining('共 0 条'), findsNothing);
  });

  testWidgets('正控：这一页就是全部时不出窗口说明', (t) async {
    await pump(t, () async => (items: rows(2), total: 2, truncated: false));
    expect(find.textContaining('仅显示'), findsNothing);
    expect(find.textContaining('通知共'), findsNothing);
  });

  testWidgets('取数失败要出声（旧写法会画成「暂无通知」）', (t) async {
    await pump(t, () => Future<NoticePageData>.error(Exception('boom')));
    expect(find.textContaining('通知加载失败'), findsOneWidget);
    // 失败时不许把「共 0 条 / 仅显示最近 0 条」这类伪造账面端上来
    expect(find.textContaining('仅显示'), findsNothing);
  });

  testWidgets('点开一条通知时标已读失败要出声（旧写法静默吞掉）', (t) async {
    await pump(
      t,
      () async => (items: rows(2), total: 2, truncated: false),
      api: _FakeApi(
        readErr: ApiException(404, 'gone', userMessage: '通知不存在，可能已被删除，请刷新通知列表'),
      ),
    );
    await t.tap(find.text('n0'));
    await t.pumpAndSettle();
    expect(find.byType(SnackBar), findsOneWidget);
    expect(find.textContaining('通知不存在'), findsOneWidget);
    // 失败不能伪装成已处理：未读标记得留在原处
    expect(find.textContaining('共 0 条'), findsNothing);
  });

  testWidgets('正控：标已读成功不出失败提示', (t) async {
    await pump(
      t,
      () async => (items: rows(2), total: 2, truncated: false),
      api: _FakeApi(),
    );
    await t.tap(find.text('n1'));
    await t.pumpAndSettle();
    expect(find.byType(SnackBar), findsNothing);
  });
}
