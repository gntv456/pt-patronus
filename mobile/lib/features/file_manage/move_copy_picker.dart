// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 移动/复制目标选择器弹窗。
// （从 file_manage_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/file_manage/file_manage_page.dart';

/// 移动/复制目标目录选择器：在当前存储内浏览目录，选中后 pop 返回目标路径。
class _MoveCopyPicker extends StatefulWidget {
  final FileStorageApi api;
  final int storageId;
  final String initialPath;
  const _MoveCopyPicker({
    required this.api,
    required this.storageId,
    required this.initialPath,
  });
  @override
  State<_MoveCopyPicker> createState() => _MoveCopyPickerState();
}

class _MoveCopyPickerState extends State<_MoveCopyPicker> {
  String _path = '/';
  List<FileEntry> _dirs = const [];
  bool _loading = false;
  // 竞态守卫：序号不匹配的过期响应直接丢弃（与主页 _loadEntries 同模式）。
  int _loadSeq = 0;

  @override
  void initState() {
    super.initState();
    _path = widget.initialPath.isEmpty ? '/' : widget.initialPath;
    _load();
  }

  Future<void> _load() async {
    final my = ++_loadSeq;
    setState(() => _loading = true);
    try {
      final all = await widget.api.files(widget.storageId, _path);
      if (!mounted || my != _loadSeq) return;
      setState(() => _dirs = all.where((e) => e.isDir).toList());
    } catch (_) {
      if (!mounted || my != _loadSeq) return;
      setState(() => _dirs = const []);
    } finally {
      if (mounted && my == _loadSeq) setState(() => _loading = false);
    }
  }

  void _cd(String p) {
    setState(() => _path = p.isEmpty ? '/' : p);
    _load();
  }

  String _parentOf(String p) {
    final segs = p.split('/').where((s) => s.isNotEmpty).toList();
    if (segs.isEmpty) return '/';
    segs.removeLast();
    return segs.isEmpty ? '/' : '/${segs.join('/')}';
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            IconButton(
              icon: const Icon(Icons.arrow_upward, size: 20),
              onPressed: _path == '/' ? null : () => _cd(_parentOf(_path)),
            ),
            Expanded(
              child: Text(
                _path,
                style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ],
        ),
        if (_loading) const LinearProgressIndicator(minHeight: 2),
        SizedBox(
          height: 320,
          child: _dirs.isEmpty && !_loading
              ? Center(
                  child: Text('（无子文件夹）', style: TextStyle(color: context.dimText)),
                )
              : ListView.builder(
                  itemCount: _dirs.length,
                  itemBuilder: (_, i) {
                    final d = _dirs[i];
                    return ListTile(
                      leading: Icon(Icons.folder, color: PTColors.primary2),
                      title: Text(d.name),
                      trailing: const Icon(Icons.chevron_right, size: 20),
                      onTap: () => _cd(d.path),
                    );
                  },
                ),
        ),
        const SizedBox(height: 8),
        FilledButton.icon(
          onPressed: () => Navigator.pop(context, _path),
          icon: const Icon(Icons.check),
          label: Text('选此目录'),
        ),
      ],
    );
  }
}
