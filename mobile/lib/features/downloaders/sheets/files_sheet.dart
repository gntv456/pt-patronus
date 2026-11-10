// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 任务文件列表弹窗：重命名/优先级/多选操作。
// （从 downloaders_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// 文件优先级管理。
class _FilesSheet extends StatefulWidget {
  final int id;
  final String hash;
  final String type;
  const _FilesSheet({required this.id, required this.hash, required this.type});
  @override
  State<_FilesSheet> createState() => _FilesSheetState();
}

class _FilesSheetState extends State<_FilesSheet> {
  final _api = DownloaderApi();
  List<TorrentFile> _files = [];
  bool _loading = true;
  String _error = ''; // 加载失败文案；非空时显示「加载失败+重试」而非「无文件」（区分失败与真空）

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final f = await _api.files(widget.id, widget.hash);
      if (mounted) {
        setState(() {
          _files = f;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  List<DropdownMenuItem<int>> _prioOpts() => [
        for (final o in _dlCaps(widget.type).filePrio)
          DropdownMenuItem(value: o.value, child: Text(o.label)),
      ];

  Future<void> _setPrio(TorrentFile f, int p) async {
    try {
      await _api.setFilePriority(widget.id, widget.hash, [f.index], p);
      _toast('已更新');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.all(14),
          child: Text(
            '文件优先级',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
        ),
        const Divider(height: 1),
        Expanded(
          child: _loading
              ? const PTLoadingView()
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _files.isEmpty
              ? Center(
                  child: Text('无文件', style: TextStyle(color: context.dimText)),
                )
              : ListView.builder(
                  itemCount: _files.length,
                  itemBuilder: (_, i) {
                    final f = _files[i];
                    return ListTile(
                      dense: true,
                      title: Text(
                        f.name,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                      subtitle: Text(
                        '${_fmtBytes(f.size)} · ${(f.progress * 100).toStringAsFixed(0)}%',
                        style: const TextStyle(fontSize: 12),
                      ),
                      trailing: DropdownButton<int>(
                        value: f.priority,
                        items: _prioOpts(),
                        onChanged: (p) => p != null ? _setPrio(f, p) : null,
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}
