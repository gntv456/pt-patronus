import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/notice.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/features/common/capped_list_notice.dart';

part 'noticepageopsmixin.dart';

/// 通知中心列表页：初始拉取 + WS 实时追加；单条已读/删除 + 全部已读。
class NoticePage extends StatefulWidget {
  final ValueChanged<String>? onOpenSource;
  // 取数注入缝：不注入就走真实 NoticeApi。窗口账面（共 N 条 / 仅显示最近 n 条）
  // 只有能从外面喂进「截断的一页」才测得到渲染层，光测 cappedListNotice 纯函数证不了接线。
  final Future<({List<Notice> items, int? total, bool truncated})> Function()?
  loadPage;
  // 动作面（已读/删除/清空）的注入缝：失败要出声这条得能从外面喂进错误才测得到。
  final NoticeApi? api;
  const NoticePage({super.key, this.onOpenSource, this.loadPage, this.api});
  @override
  State<NoticePage> createState() => _NoticePageState();
}

class _NoticePageState extends State<NoticePage> with NoticePageOpsMixin {
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
      final page = await (widget.loadPage ?? _api.listPage)();
      _items = page.items;
      _windowNote = cappedListNotice(
        shown: page.items.length,
        total: page.total,
        truncated: page.truncated,
        unknownTotal: (n) => '仅显示最近 $n 条通知，可能还有更多',
        knownTotal: (n, t) => '通知共 $t 条，仅显示最近 $n 条',
      );
    } catch (_) {
      _error = '通知加载失败';
      _windowNote = '';
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
          bottom: _windowNote.isEmpty
              ? null
              : PreferredSize(
                  preferredSize: const Size.fromHeight(34),
                  child: Material(
                    color: const Color(0xFFF39C12).withValues(alpha: 0.12),
                    child: Center(
                      child: Text(
                        _windowNote,
                        style: TextStyle(fontSize: 12, color: context.dimText),
                      ),
                    ),
                  ),
                ),
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
              ? const PTLoadingView()
              : _items.isEmpty
              ? ListView(
                  children: [
                    const SizedBox(height: 120),
                    Center(
                      child: Text(
                        _error.isNotEmpty ? _error : '暂无通知',
                        style: TextStyle(color: context.dimText),
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
                      child: PTListItemEntrance(
                        index: i,
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
                                              borderRadius:
                                                  BorderRadius.circular(999),
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
                                          style: TextStyle(
                                            fontSize: 11,
                                            color: context.dimText,
                                          ),
                                        ),
                                      ],
                                    ),
                                    if (n.body.isNotEmpty)
                                      Padding(
                                        padding: const EdgeInsets.only(top: 3),
                                        child: Text(
                                          n.body,
                                          style: TextStyle(
                                            fontSize: 12,
                                            color: context.dimText,
                                            height: 1.4,
                                          ),
                                        ),
                                      ),
                                  ],
                                ),
                              ),
                              if (!n.read)
                                Padding(
                                  padding: const EdgeInsets.only(
                                    left: 6,
                                    top: 4,
                                  ),
                                  child: CircleAvatar(
                                    radius: 3.5,
                                    backgroundColor: c,
                                  ),
                                ),
                            ],
                          ),
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
