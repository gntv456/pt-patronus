// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/monitor.dart';

// #308 移动端「编辑设备」必须把 PUT 全量替换里的每一项都回读出来。
// 后端 PUT /api/v1/monitor/:id 是 `t.Thresholds = in.Thresholds` +
// `t.CredentialID = in.CredentialID`（阈值 JSON 同时承载 HTTP 期望状态/响应阈值/
// 进程名/脚本命令），漏传即被存成空值/0：界面照样 200 成功，下一轮采集却报
// 「脚本命令不能为空」或鉴权失败。Web 同名表单两项都回读（credential_id /
// thresholds），可作 A/B 对照。
String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  group('监控设备 PUT 载荷回读', () {
    test('GET 回来的 credential_id 与 thresholds 要进模型', () {
      final t = MonitorTarget.fromJson(<String, dynamic>{
        'id': 3,
        'name': 'nas-a',
        'type': 'nas',
        'credential_id': 7,
        'thresholds': '{"script_command":"df -P","response_ms":500}',
      });
      expect(t.credentialId, 7, reason: '漏读这一项，编辑保存就会解绑凭据');
      expect(t.thresholds, contains('script_command'));
    });

    test('缺省字段不炸（后端可能回 0/空串）', () {
      final t = MonitorTarget.fromJson(<String, dynamic>{'id': 1, 'name': 'h'});
      expect(t.credentialId, 0);
      expect(t.thresholds, '');
    });

    test('TargetInput.toJson 必须带上这两项（PUT 是全量替换）', () {
      final t = MonitorTarget.fromJson(<String, dynamic>{
        'id': 3,
        'name': 'nas-a',
        'credential_id': 7,
        'thresholds': '{"response_ms":500}',
      });
      final input = TargetInput(
        name: t.name,
        type: t.type,
        credentialId: t.credentialId,
        thresholds: t.thresholds,
      );
      expect(input.toJson()['credential_id'], 7);
      expect(input.toJson()['thresholds'], '{"response_ms":500}');
    });

    test('新建设备不带凭据时回 0（不是漏键）', () {
      expect(
        TargetInput(name: 'n', type: 'script').toJson()['credential_id'],
        0,
      );
    });
  });

  group('设备表单提交接线', () {
    final sheet = _code('lib/features/monitor/deviceformopsmixin.dart');

    test('提交前确有构造 TargetInput（否则下面两条判据是空的）', () {
      expect(sheet.indexOf('final input = TargetInput('), greaterThan(-1));
    });

    test('编辑设备要回读 thresholds，改名字不再抹掉告警规则', () {
      final start = sheet.indexOf('final input = TargetInput(');
      final call = sheet.substring(start, sheet.indexOf(');', start));
      expect(
        call,
        contains('thresholds: widget.target?.thresholds'),
        reason: '表单没有阈值编辑项，只能回读原值',
      );
      expect(
        call,
        contains('credentialId: widget.target?.credentialId'),
        reason: '表单没有凭据选择器，漏传 = 后端把 credential_id 存成 0',
      );
    });
  });
}
