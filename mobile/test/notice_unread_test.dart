// 铃铛角标的未读数：必须用服务端同行的 total，不能只数被上限截断的那一页。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/notice.dart';
import 'package:ptpatronus/features/common/capped_list_notice.dart';

void main() {
  test('未读超过单页上限：用 total 而不是本页条数', () {
    final r = <String, dynamic>{
      'notices': List.generate(200, (i) => {'id': i}),
      'total': 900,
      'limit': 200,
      'truncated': true,
    };
    expect(noticeUnreadFrom(r), 900);
  });

  test('total=0 是真零，不退回数本页', () {
    expect(noticeUnreadFrom(<String, dynamic>{'total': 0}), 0);
  });

  test('旧响应没有 total：退回数本页，而不是报 0', () {
    final r = <String, dynamic>{
      'notices': [
        {'id': 1},
        {'id': 2},
      ],
    };
    expect(noticeUnreadFrom(r), 2);
  });

  test('正控：total 与本页一致时取谁都一样', () {
    final r = <String, dynamic>{
      'notices': [
        {'id': 1},
      ],
      'total': 1,
      'truncated': false,
    };
    expect(noticeUnreadFrom(r), 1);
  });

  group('收件箱窗口账面', () {
    test('被单页上限截断：total 与 truncated 一起接住', () {
      final w = noticeWindowFrom(<String, dynamic>{
        'notices': List.generate(200, (i) => {'id': i}),
        'total': 900,
        'truncated': true,
      });
      expect(w.total, 900);
      expect(w.truncated, isTrue);
      expect(
        cappedListNotice(
          shown: 200,
          total: w.total,
          truncated: w.truncated,
          unknownTotal: (n) => '仅显示最近 $n 条通知，可能还有更多',
          knownTotal: (n, t) => '通知共 $t 条，仅显示最近 $n 条',
        ),
        '通知共 900 条，仅显示最近 200 条',
      );
    });

    test('缺 truncated 字段：按「可能还有更多」处理，不给假安心', () {
      final w = noticeWindowFrom(<String, dynamic>{
        'notices': [
          {'id': 1},
        ],
      });
      expect(w.total, isNull);
      expect(w.truncated, isTrue);
      expect(
        cappedListNotice(
          shown: 1,
          total: w.total,
          truncated: w.truncated,
          unknownTotal: (n) => '仅显示最近 $n 条通知，可能还有更多',
          knownTotal: (n, t) => '通知共 $t 条，仅显示最近 $n 条',
        ),
        '仅显示最近 1 条通知，可能还有更多',
      );
    });

    test('正控：后端说没截断就不出声', () {
      final w = noticeWindowFrom(<String, dynamic>{'total': 3, 'truncated': false});
      expect(w.total, 3);
      expect(w.truncated, isFalse);
      expect(
        cappedListNotice(
          shown: 3,
          total: w.total,
          truncated: w.truncated,
          unknownTotal: (n) => '仅显示最近 $n 条通知，可能还有更多',
          knownTotal: (n, t) => '通知共 $t 条，仅显示最近 $n 条',
        ),
        isEmpty,
      );
    });
  });
}
