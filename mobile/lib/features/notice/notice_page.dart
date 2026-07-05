import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/notice.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/core/ws_client.dart';

/// 通知中心列表页：初始拉取 + WS 实时追加；单条已读/删除 + 全部已读。
class NoticePage extends StatefulWidget {
  final ValueChanged<String>? onOpenSource;
  const NoticePage({super.key, this.onOpenSource});
  @override
  State<NoticePage> createState() => _NoticePageState();
}

class _NoticePageState extends State<NoticePage> {
  final _api = NoticeApi();
  List<Notice> _items = [];
  bool _loading = true;
  String _error = ''; // 加载失败文案：非空时空态显示错误 + 重试（替代静默吞错的「暂无通知」）
  StreamSubscription<Notice>? _sub;

  @override
  void initState() {
    super.initState();
    _load();
    _sub = WsClient().notices.listen(_onPush);
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = '';
    });
    try {
      _items = await _api.list();
    } catch (_) {
      _error = '通知加载失败';
    }
    if (mounted) setState(() => _loading = false);
    refreshNoticeUnread();
  }

  void _onPush(Notice n) {
    if (!_items.any((x) => x.id == n.id)) {
      if (mounted) setState(() => _items.insert(0, n));
    }
    refreshNoticeUnread();
  }

  @override
  void dispose() {
    _sub?.cancel();
    super.dispose();
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
    } catch (_) {}
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
    } catch (_) {}
  }

  Future<void> _delete(Notice n) async {
    try {
      await _api.delete(n.id);
      if (mounted) {
        setState(() => _items = _items.where((x) => x.id != n.id).toList());
      }
      refreshNoticeUnread();
    } catch (_) {}
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
    } catch (_) {}
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

  Color _levelColor(NoticeLevel l) => switch (l) {
    NoticeLevel.success => const Color(0xFF22C55E),
    NoticeLevel.warning => const Color(0xFFF39C12),
    NoticeLevel.danger => const Color(0xFFEF4444),
    NoticeLevel.info => const Color(0xFF3B82F6),
  };

  Future<void> _openNotice(Notice n) async {
    if (!n.read) await _read(n);
    final open = widget.onOpenSource;
    if (open == null || !_canOpenSource(n.source)) return;
    open(n.source);
    if (mounted) Navigator.of(context).pop();
  }

  bool _canOpenSource(String source) => const {
    'new_mail',
    'announce',
    'assess_pass',
    'assess_fail',
    'site_anniv',
    'freeleech',
    'reseed_done',
    'dl_done',
    'host_offline',
    'nas_offline',
    'site_offline',
    'backup_done',
    'backup_import',
  }.contains(source);
  IconData _levelIcon(NoticeLevel l) => switch (l) {
    NoticeLevel.success => Icons.check_circle,
    NoticeLevel.warning => Icons.warning_amber_rounded,
    NoticeLevel.danger => Icons.error_outline,
    NoticeLevel.info => Icons.info_outline,
  };

  String _relTime(DateTime d) {
    final diff = DateTime.now().difference(d).inSeconds;
    if (diff < 60) return '刚刚';
    if (diff < 3600) return '${diff ~/ 60} 分钟前';
    if (diff < 86400) return '${diff ~/ 3600} 小时前';
    return '${diff ~/ 86400} 天前';
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          title: const Text('通知中心'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            PTIconAction(
              icon: Icons.refresh,
              tooltip: '刷新',
              loading: _loading,
              onPressed: _loading ? null : _load,
            ),
            PopupMenuButton<String>(
              icon: const Icon(Icons.more_vert),
              iconSize: 20,
              padding: EdgeInsets.zero,
              constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
              tooltip: '更多',
              onSelected: (v) {
                if (v == 'readall') {
                  _readAll();
                } else if (v == 'clear') {
                  _clearAll();
                }
              },
              itemBuilder: (_) => [
                const PopupMenuItem(
                  value: 'readall',
                  child: Row(
                    children: [
                      Icon(Icons.done_all, size: 18),
                      SizedBox(width: 8),
                      Text('全部已读'),
                    ],
                  ),
                ),
                const PopupMenuItem(
                  value: 'clear',
                  child: Row(
                    children: [
                      Icon(Icons.delete_sweep_outlined, size: 18),
                      SizedBox(width: 8),
                      Text('清空通知'),
                    ],
                  ),
                ),
              ],
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: _load,
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _items.isEmpty
              ? ListView(
                  children: [
                    const SizedBox(height: 120),
                    Center(
                      child: Text(
                        _error.isNotEmpty ? _error : '暂无通知',
                        style: const TextStyle(color: Colors.grey),
                      ),
                    ),
                    if (_error.isNotEmpty) ...[
                      const SizedBox(height: 12),
                      Center(
                        child: TextButton.icon(
                          onPressed: _load,
                          icon: const Icon(Icons.refresh, size: 18),
                          label: const Text('重试'),
                        ),
                      ),
                    ],
                  ],
                )
              : ListView.builder(
                  padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
                  itemCount: _items.length,
                  itemBuilder: (_, i) {
                    final n = _items[i];
                    final c = _levelColor(n.level);
                    return Dismissible(
                      key: ValueKey(n.id),
                      direction: DismissDirection.endToStart,
                      background: Container(
                        alignment: Alignment.centerRight,
                        padding: const EdgeInsets.only(right: 20),
                        decoration: BoxDecoration(
                          color: Colors.red.withValues(alpha: 0.12),
                          borderRadius: BorderRadius.circular(20),
                        ),
                        child: const Icon(Icons.delete, color: Colors.red),
                      ),
                      onDismissed: (_) => _delete(n),
                      child: GlassCard(
                        onTap: () => _openNotice(n),
                        padding: const EdgeInsets.all(12),
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Container(
                              width: 34,
                              height: 34,
                              alignment: Alignment.center,
                              decoration: BoxDecoration(
                                color: c.withValues(alpha: 0.14),
                                borderRadius: BorderRadius.circular(10),
                              ),
                              child: Icon(
                                _levelIcon(n.level),
                                color: c,
                                size: 18,
                              ),
                            ),
                            const SizedBox(width: 10),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Row(
                                    children: [
                                      if (noticeSourceLabel[n.source] != null)
                                        Container(
                                          margin: const EdgeInsets.only(
                                            right: 6,
                                          ),
                                          padding: const EdgeInsets.symmetric(
                                            horizontal: 6,
                                            vertical: 1,
                                          ),
                                          decoration: BoxDecoration(
                                            color: c.withValues(alpha: 0.12),
                                            borderRadius: BorderRadius.circular(
                                              999,
                                            ),
                                          ),
                                          child: Text(
                                            noticeSourceLabel[n.source]!,
                                            style: TextStyle(
                                              fontSize: 11,
                                              color: c,
                                              fontWeight: FontWeight.w600,
                                            ),
                                          ),
                                        ),
                                      Expanded(
                                        child: Text(
                                          n.title,
                                          style: TextStyle(
                                            fontWeight: FontWeight.w600,
                                            fontSize: 13,
                                          ),
                                          maxLines: 1,
                                          overflow: TextOverflow.ellipsis,
                                        ),
                                      ),
                                      Text(
                                        _relTime(n.createdAt),
                                        style: const TextStyle(
                                          fontSize: 11,
                                          color: Colors.grey,
                                        ),
                                      ),
                                    ],
                                  ),
                                  if (n.body.isNotEmpty)
                                    Padding(
                                      padding: const EdgeInsets.only(top: 3),
                                      child: Text(
                                        n.body,
                                        style: const TextStyle(
                                          fontSize: 12,
                                          color: Colors.grey,
                                          height: 1.4,
                                        ),
                                      ),
                                    ),
                                ],
                              ),
                            ),
                            if (!n.read)
                              Padding(
                                padding: const EdgeInsets.only(left: 6, top: 4),
                                child: CircleAvatar(
                                  radius: 3.5,
                                  backgroundColor: c,
                                ),
                              ),
                          ],
                        ),
                      ),
                    );
                  },
                ),
        ),
      ),
    );
  }
}
