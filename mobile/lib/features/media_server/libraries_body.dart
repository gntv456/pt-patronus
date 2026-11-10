// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 media_server_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/media_server/media_server_page.dart';

/// 媒体库列表 + 每库扫描/刷新/封面预览。
class _LibrariesBody extends StatefulWidget {
  final int serverId;
  final List<MediaLibrary> libs;
  final MediaServerApi api;
  const _LibrariesBody({required this.serverId, required this.libs, required this.api});
  @override
  State<_LibrariesBody> createState() => _LibrariesBodyState();
}

class _LibrariesBodyState extends State<_LibrariesBody> {
  final _busy = <String>{}; // libraryId 正在操作
  Uint8List? _cover;
  bool _coverLoading = false;

  void _toast(String msg) =>
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _scan(MediaLibrary lib, {bool all = false}) async {
    final key = all ? '__all__' : lib.id;
    setState(() => _busy.add(key));
    try {
      final r = await widget.api.scan(widget.serverId, all ? '' : lib.id);
      _toast((r['message'] as String?) ?? (all ? '已触发全库扫描' : '已扫描：${lib.name}'));
    } catch (e) {
      _toast('扫描失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(key));
    }
  }

  Future<void> _refresh(MediaLibrary lib) async {
    setState(() => _busy.add(lib.id));
    try {
      final r = await widget.api.refresh(widget.serverId, lib.id);
      _toast((r['message'] as String?) ?? '已刷新：${lib.name}');
    } catch (e) {
      _toast('刷新失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(lib.id));
    }
  }

  Future<void> _cover_(MediaLibrary lib) async {
    setState(() {
      _coverLoading = true;
      _cover = null;
    });
    try {
      final bytes = await widget.api.cover(widget.serverId, lib.id);
      if (mounted) setState(() => _cover = bytes);
    } catch (e) {
      _toast('封面生成失败：$e');
    } finally {
      if (mounted) setState(() => _coverLoading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final webhook = 'POST /api/v1/mediaserver/${widget.serverId}/webhook';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Row(
          children: [
            TextButton.icon(
              icon: const Icon(Icons.refresh, size: 16),
              label: const Text('全库扫描'),
              onPressed: _busy.contains('__all__') ? null : () => _scan(widget.libs.isNotEmpty ? widget.libs.first : MediaLibrary(id: '', name: '', type: ''), all: true),
            ),
          ],
        ),
        Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: Text('播放回调：$webhook', style: TextStyle(fontSize: 11, color: context.dimText)),
        ),
        for (final lib in widget.libs)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(lib.name, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                      Text(lib.type.isEmpty ? '库 #${lib.id}' : '${lib.type} · #${lib.id}',
                          style: TextStyle(fontSize: 11, color: context.dimText)),
                    ],
                  ),
                ),
                if (_busy.contains(lib.id))
                  const Padding(
                    padding: EdgeInsets.only(right: 8),
                    child: SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2)),
                  ),
                TextButton(onPressed: _busy.contains(lib.id) ? null : () => _scan(lib), child: const Text('扫描')),
                TextButton(onPressed: _busy.contains(lib.id) ? null : () => _refresh(lib), child: const Text('刷新')),
                TextButton(onPressed: _busy.contains(lib.id) ? null : () => _cover_(lib), child: const Text('封面')),
              ],
            ),
          ),
        if (_coverLoading)
          const Padding(padding: EdgeInsets.all(16), child: Center(child: CircularProgressIndicator())),
        if (_cover != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Image.memory(_cover!, fit: BoxFit.contain),
            ),
          ),
      ],
    );
  }
}
