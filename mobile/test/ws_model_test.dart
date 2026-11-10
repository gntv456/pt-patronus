// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// WS 实时推送消息解析测试：匹配后端 ws.TorrentSnapshot + 顶层消息信封形状。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/core/ws_client.dart';

void main() {
  test('TorrentSnapshot.fromJson parses downloader + torrents + dl_rate', () {
    final s = TorrentSnapshot.fromJson({
      'downloader_id': 1,
      'name': 'QB',
      'error': '',
      'torrents': [
        {
          'hash': 'aa',
          'name': 'Movie 2024',
          'size': 1610612736,
          'progress': 0.5,
          'state': 'downloading',
          'dl_rate': 2097152,
          'up_rate': 10240,
        },
      ],
    });
    expect(s.downloaderId, 1);
    expect(s.name, 'QB');
    expect(s.error, '');
    expect(s.torrents.length, 1);
    expect(s.torrents[0].downRate, 2097152);
    expect(s.torrents[0].upRate, 10240);
    expect(s.torrents[0].progress, 0.5);
  });

  test('TorrentSnapshot.fromJson surfaces per-downloader error', () {
    final s = TorrentSnapshot.fromJson({
      'downloader_id': 2,
      'name': 'TR',
      'error': 'cannot connect',
      'torrents': [],
    });
    expect(s.downloaderId, 2);
    expect(s.error, 'cannot connect');
    expect(s.torrents, isEmpty);
  });

  test('WS message envelope: type=torrents + data array', () {
    // 验证 WsClient._onData 期望的顶层形状（type + data 数组）。
    final msg = <String, dynamic>{
      'type': 'torrents',
      'data': [
        {'downloader_id': 1, 'name': 'A', 'torrents': <Map<String, dynamic>>[]},
        {'downloader_id': 2, 'name': 'B', 'error': 'x', 'torrents': <Map<String, dynamic>>[]},
      ],
    };
    expect(msg['type'], 'torrents');
    final list = (msg['data'] as List)
        .map((e) => TorrentSnapshot.fromJson(e as Map<String, dynamic>))
        .toList();
    expect(list.length, 2);
    expect(list[0].downloaderId, 1);
    expect(list[1].error, 'x');
  });

  test('HRAlert.fromJson parses site/title/status/deadline', () {
    final a = HRAlert.fromJson({
      'site': '好学',
      'title': 'Some Torrent',
      'status': '未达标',
      'deadline': '10:00:00',
    });
    expect(a.site, '好学');
    expect(a.title, 'Some Torrent');
    expect(a.status, '未达标');
    expect(a.deadline, '10:00:00');
  });

  test('HRAlert.fromJson tolerates missing fields', () {
    final a = HRAlert.fromJson({'site': 'X'});
    expect(a.site, 'X');
    expect(a.title, '');
    expect(a.deadline, '');
  });

  test('WS hr-alert envelope: type=hr-alert + data array', () {
    final msg = <String, dynamic>{
      'type': 'hr-alert',
      'data': [
        {'site': '好学', 'title': 'A', 'status': '未达标', 'deadline': '1:00:00'},
      ],
    };
    expect(msg['type'], 'hr-alert');
    final list = (msg['data'] as List)
        .map((e) => HRAlert.fromJson(e as Map<String, dynamic>))
        .toList();
    expect(list.length, 1);
    expect(list[0].status, '未达标');
  });

  test('WS cookie-alert envelope: type=cookie-alert + string array', () {
    final msg = <String, dynamic>{
      'type': 'cookie-alert',
      'data': ['好学', '馒头'],
    };
    expect(msg['type'], 'cookie-alert');
    final sites = (msg['data'] as List).map((e) => e as String).toList();
    expect(sites.length, 2);
    expect(sites[0], '好学');
    expect(sites[1], '馒头');
  });
}
