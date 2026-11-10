// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NoticePage 的动作面（单条已读 / 全部已读 / 删除 / 清空），从 notice_page.dart 拆出。
//
// 拆分之外还修了一处真实缺陷：四个动作从前一律 `catch (_) {}`。后端 #204/#205 把
// 「删/标一条已经不存在的记录」从 200 改成 404 之后，这里就变成「点了没反应、
// 红点还在、也没有任何一句解释」——用户只会反复点。现在把后端信封里的中文原因说出来。
part of 'package:ptpatronus/features/notice/notice_page.dart';

mixin NoticePageOpsMixin on State<NoticePage> {
  late final NoticeApi _api = widget.api ?? NoticeApi();
  List<Notice> _items = [];
  bool _loading = true;
  String _error = ''; // 加载失败文案：非空时空态显示错误 + 重试（替代静默吞错的「暂无通知」）
  String _windowNote = ''; // 单页上限截断说明；空 = 这一页就是全部
  StreamSubscription<Notice>? _sub;

  void _fail(Object e) {
    if (!mounted) return;
    final why = e is ApiException ? e.userMessage : '操作失败';
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text('操作失败：$why'), behavior: SnackBarBehavior.floating),
    );
  }

  Future<void> _read(Notice n) async {
    try {
      await _api.read(n.id);
      if (mounted) {
        setState(() {
          _items = _items.map((x) => x.id == n.id ? _markRead(x) : x).toList();
        });
      }
      refreshNoticeUnread();
    } catch (e) {
      _fail(e);
    }
  }

  Future<void> _readAll() async {
    try {
      await _api.readAll();
      if (mounted) {
        setState(() {
          _items = _items.map((x) => _markRead(x)).toList();
        });
      }
      refreshNoticeUnread();
    } catch (e) {
      _fail(e);
    }
  }

  Future<void> _delete(Notice n) async {
    try {
      await _api.delete(n.id);
      if (mounted) {
        setState(() => _items = _items.where((x) => x.id != n.id).toList());
      }
      refreshNoticeUnread();
    } catch (e) {
      _fail(e);
    }
  }

  Future<void> _clearAll() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('清空通知'),
        content: const Text('确定清空全部通知？此操作不可恢复。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: TextButton.styleFrom(foregroundColor: Colors.red),
            child: const Text('清空'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.clear();
      if (mounted) setState(() => _items = []);
      refreshNoticeUnread();
    } catch (e) {
      _fail(e);
    }
  }

  Notice _markRead(Notice n) => Notice(
    id: n.id,
    source: n.source,
    title: n.title,
    body: n.body,
    level: n.level,
    read: true,
    createdAt: n.createdAt,
  );
}
