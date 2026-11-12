// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 任务弹窗：订阅 WsClient 实时种子流，顶部速率面板 + 添加入口 + 每行操作菜单。
// （从 downloaders_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// 任务弹窗：订阅 WsClient 实时种子流，顶部速率面板 + 添加入口 + 每行操作菜单。
class _TorrentSheet extends StatefulWidget {
  final Downloader downloader;
  const _TorrentSheet({required this.downloader});
  @override
  State<_TorrentSheet> createState() => _TorrentSheetState();
}

class _TorrentSheetState extends State<_TorrentSheet> {
  final _ws = WsClient();
  final _api = DownloaderApi();
  List<Torrent> _torrents = [];
  int? _dlRate;
  int? _upRate;
  String _error = '';
  bool _connecting = true;
  bool _wsDead = false; // WS 连不上（ticket 被拒/握手失败）：展示错误态而非无限转圈
  Timer? _connectTimeout;
  StreamSubscription? _sub;
  StreamSubscription? _stateSub;

  int get _id => widget.downloader.id;
  String get _type => widget.downloader.type;

  @override
  void initState() {
    super.initState();
    _ws.connect();
    // 兜底超时：WS 无任何状态回调（如 ticket 挂起）时，10s 后走 REST 拉一次，
    // 不让弹窗无限转圈。
    _connectTimeout = Timer(const Duration(seconds: 10), _restFallback);
    _stateSub = _ws.connectionState.listen((st) {
      if (!mounted) return;
      if (st == WsState.failed || st == WsState.stopped) {
        setState(() => _wsDead = true);
        _connectTimeout?.cancel();
        _restFallback();
      } else if (st == WsState.connected) {
        _connectTimeout?.cancel();
      }
    });
    _sub = _ws.snapshots.listen((list) {
      final match = list.where((s) => s.downloaderId == _id).toList();
      if (match.isEmpty) return; // 本次广播不含此下载器，保留上次显示
      final s = match.first;
      if (!mounted) return;
      setState(() {
        _torrents = s.torrents;
        _dlRate = s.dlRate;
        _upRate = s.upRate;
        _error = s.error;
        _connecting = false;
        _wsDead = false; // 收到推送即覆盖 REST 兜底横幅
      });
    });
  }

  @override
  void dispose() {
    _sub?.cancel(); // 仅取消本页订阅，不断开共享的 WsClient
    _stateSub?.cancel();
    _connectTimeout?.cancel();
    super.dispose();
  }

  /// REST 兜底：WS 不可达时直接拉一次种子列表（静态快照，无实时速率）。
  Future<void> _restFallback() async {
    try {
      final list = await _api.torrents(_id);
      if (!mounted) return;
      setState(() {
        _torrents = list;
        _connecting = false;
      });
    } catch (_) {/* REST 也失败：保持现有状态（_wsDead 已置错误态） */}
  }

  int get _active =>
      _torrents.where((t) => t.state.contains('download')).length;
  int get _seeding => _torrents
      .where((t) => t.state.contains('upload') || t.state.contains('seed'))
      .length;
  int get _paused => _torrents
      .where((t) => t.state.contains('pause') || t.state.contains('stop'))
      .length;

  @override
  Widget build(BuildContext context) {
    final d = widget.downloader;
    Widget body;
    if (_wsDead && _torrents.isEmpty) {
      body = Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.cloud_off, size: 36, color: context.dimText),
            const SizedBox(height: 10),
            Text('实时推送连不上', style: TextStyle(color: context.dimText)),
            const SizedBox(height: 12),
            FilledButton.tonal(
              onPressed: () {
                setState(() {
                  _wsDead = false;
                  _connecting = true;
                });
                _connectTimeout =
                    Timer(const Duration(seconds: 10), _restFallback);
                _ws.connect();
              },
              child: const Text('重试'),
            ),
          ],
        ),
      );
    } else if (_connecting) {
      body = Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CircularProgressIndicator(),
            const SizedBox(height: 12),
            Text('连接实时推送…', style: TextStyle(color: context.dimText)),
          ],
        ),
      );
    } else if (_error.isNotEmpty) {
      body = Center(
        child: Text(
          '拉取失败：$_error',
          style: TextStyle(color: Colors.redAccent),
        ),
      );
    } else if (_torrents.isEmpty) {
      body = Center(
        child: Text('无任务', style: TextStyle(color: context.dimText)),
      );
    } else {
      body = ListView.separated(
        itemCount: _torrents.length,
        separatorBuilder: (_, _) =>
            const Divider(height: 1, indent: 12, endIndent: 12),
        itemBuilder: (_, i) => _row(_torrents[i]),
      );
    }
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(14),
          child: Row(
            children: [
              Text(
                '${d.name} · 任务',
                style: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const Spacer(),
              if (_dlCaps(_type).categoryCRUD)
                IconButton(
                  icon: const Icon(Icons.category_outlined),
                  onPressed: _showCategoryManager,
                  tooltip: '分类管理',
                ),
              IconButton(
                icon: const Icon(Icons.add_circle_outline),
                onPressed: _add,
                tooltip: '添加种子',
              ),
              const SizedBox(width: 4),
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                  color: _ws.isConnected ? PTColors.success : context.dimText,
                  shape: BoxShape.circle,
                ),
              ),
            ],
          ),
        ),
        _statsRow(),
        const Divider(height: 1),
        Expanded(child: body),
      ],
    );
  }

  /// 左滑删除：确认是否删文件 → 乐观移除列表 → SnackBar 4s 可撤销 → 超时才真正请求删除。
  Future<bool> _swipeDelete(Torrent t) async {
    final df = await _confirmDelete(t.name);
    if (df == null) return false;
    final idx = _torrents.indexWhere((x) => x.hash == t.hash);
    if (idx < 0) return false;
    final removed = _torrents[idx];
    setState(() => _torrents.removeAt(idx));
    var undone = false;
    if (!mounted) return true;
    final messenger = ScaffoldMessenger.of(context);
    messenger.clearSnackBars();
    final controller = messenger.showSnackBar(
      SnackBar(
        content: Text('已删除「${removed.name}」'),
        duration: const Duration(seconds: 4),
        action: SnackBarAction(
          label: '撤销',
          onPressed: () {
            undone = true;
            if (!mounted) return;
            setState(() {
              final at = idx.clamp(0, _torrents.length);
              _torrents.insert(at, removed);
            });
          },
        ),
      ),
    );
    await controller.closed;
    if (!undone) {
      try {
        await _api.removeTorrents(_id, [removed.hash], deleteFiles: df);
      } catch (e) {
        if (mounted) {
          setState(() {
            final at = idx.clamp(0, _torrents.length);
            _torrents.insert(at, removed);
          });
          _toast(_cleanErr(e));
        }
      }
    }
    return true;
  }
}
