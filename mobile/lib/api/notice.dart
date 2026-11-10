// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/foundation.dart';
import '../core/api_client.dart';

enum NoticeLevel { info, success, warning, danger }

/// 通知中心 Source（对齐后端 notice.SourceXxx）。
const noticeSourceLabel = <String, String>{
  'site_anniv': '今日站庆',
  'new_mail': '新邮件',
  'assess_pass': '考核完成',
  'assess_fail': '考核失败',
  'announce': '新公告',
  'host_offline': '服务器掉线',
  'nas_offline': 'NAS 掉线',
  'reseed_done': '辅种完成',
  'dl_done': '下载完成',
  'site_offline': '站点掉线',
  'backup_done': '备份完成',
  'backup_import': '备份导入',
  'freeleech': '今日站免',
};

class Notice {
  final int id;
  final String source;
  final String title;
  final String body;
  final NoticeLevel level;
  final bool read;
  final DateTime createdAt;

  Notice({
    required this.id,
    required this.source,
    required this.title,
    required this.body,
    required this.level,
    required this.read,
    required this.createdAt,
  });

  factory Notice.fromJson(Map<String, dynamic> j) => Notice(
    id: (j['id'] as num?)?.toInt() ?? 0,
    source: j['source'] as String? ?? '',
    title: j['title'] as String? ?? '',
    body: j['body'] as String? ?? '',
    level: _parseLevel(j['level'] as String? ?? 'info'),
    read: j['read'] as bool? ?? false,
    createdAt:
        DateTime.tryParse(j['created_at'] as String? ?? '') ?? DateTime.now(),
  );
}

NoticeLevel _parseLevel(String s) => switch (s) {
  'success' => NoticeLevel.success,
  'warning' => NoticeLevel.warning,
  'danger' => NoticeLevel.danger,
  _ => NoticeLevel.info,
};

/// 全局未读数（铃铛角标监听，单例 ValueNotifier）。
final noticeUnread = ValueNotifier<int>(0);

class NoticeApi {
  final _api = ApiClient();

  /// 收件箱一页通知 + 取数账面：列表有单页上限，total/truncated 不接住就会被当成全部。
  Future<({List<Notice> items, int? total, bool truncated})> listPage({
    bool unread = false,
  }) async {
    final r = await _api.get('/notices${unread ? '?unread=true' : ''}');
    final list = (r['notices'] as List?) ?? const [];
    final window = noticeWindowFrom(r);
    return (
      items: list
          .map((e) => Notice.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: window.total,
      truncated: window.truncated,
    );
  }

  Future<List<Notice>> list({bool unread = false}) async =>
      (await listPage(unread: unread)).items;

  Future<int> unreadCount() async {
    final r = await _api.get('/notices?unread=true&limit=1');
    return noticeUnreadFrom(r);
  }

  Future<void> read(int id) =>
      _api.post('/notices/$id/read', <String, dynamic>{});
  Future<void> readAll() => _api.post('/notices/read-all', <String, dynamic>{});
  Future<void> clear() => _api.post('/notices/clear', <String, dynamic>{});
  Future<void> delete(int id) => _api.delete('/notices/$id');
}

/// 收件箱一页的取数账面。后端截断时必带 truncated；字段缺失（旧版后端/读库失败）
/// 一律按「可能还有更多」处理，不给「这就是全部」的假安心。
({int? total, bool truncated}) noticeWindowFrom(Map<String, dynamic> r) => (
  total: (r['total'] as num?)?.toInt(),
  truncated: r['truncated'] as bool? ?? true,
);

/// 未读数：优先取服务端同批回来的 total（?unread=true 时它就是未读总数）。
/// 只数这一页的话，未读超过单页上限（默认 200）角标就少报——
/// 用户以为「只剩这几条」，实际还有更多。total 读不出才退回数本页。
int noticeUnreadFrom(Map<String, dynamic> r) {
  final total = (r['total'] as num?)?.toInt();
  if (total != null) return total;
  return ((r['notices'] as List?) ?? const []).length;
}

/// 刷新全局未读计数（铃铛角标）。WS 推送新通知后调用。
Future<void> refreshNoticeUnread() async {
  try {
    noticeUnread.value = await NoticeApi().unreadCount();
  } catch (_) {
    /* 静默 */
  }
}
