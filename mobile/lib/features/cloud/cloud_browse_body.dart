// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 cloud_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/cloud/cloud_page.dart';

/// 网盘目录浏览（可作纯浏览 or 选择目录回调）。
class _BrowseBody extends StatefulWidget {
  final CloudDrive drive;
  final CloudApi api;
  final void Function(String path)? onPick;
  const _BrowseBody({required this.drive, required this.api, this.onPick});
  @override
  State<_BrowseBody> createState() => _BrowseBodyState();
}

class _BrowseBodyState extends State<_BrowseBody> {
  String _path = '/';
  List<CloudFile> _files = [];
  bool _loading = false;

  @override
  void initState() {
    super.initState();
    _path = widget.drive.rootPath.isEmpty ? '/' : widget.drive.rootPath;
    _fetch();
  }

  Future<void> _fetch() async {
    setState(() => _loading = true);
    try {
      final files = await widget.api.browse(widget.drive.id, _path);
      if (mounted) setState(() => _files = files);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('浏览失败：$e')));
        setState(() => _files = []);
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _enter(CloudFile f) {
    if (!f.isDir) return;
    setState(() => _path = f.path);
    _fetch();
  }

  void _up() {
    if (_path == '/') return;
    final parts = _path.split('/').where((s) => s.isNotEmpty).toList()..removeLast();
    setState(() => _path = '/${parts.join('/')}');
    _fetch();
  }

  @override
  Widget build(BuildContext context) {
    final dirs = _files.where((f) => f.isDir).toList();
    final files = _files.where((f) => !f.isDir).toList();
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Row(children: [
          TextButton.icon(icon: const Icon(Icons.arrow_upward, size: 16), label: const Text('上级'), onPressed: _path == '/' ? null : _up),
          Expanded(child: Text(_path, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600), maxLines: 1, overflow: TextOverflow.ellipsis)),
          if (widget.onPick != null)
            FilledButton(onPressed: () => widget.onPick!(_path), child: const Text('使用此目录')),
        ]),
        if (_loading) const Padding(padding: EdgeInsets.all(16), child: PTLoadingView()),
        if (!_loading)
          Flexible(
            child: ListView(shrinkWrap: true, children: [
              for (final d in dirs)
                ListTile(dense: true, leading: const Icon(Icons.folder, size: 20, color: Color(0xFF4CC9F0)), title: Text(d.name, style: const TextStyle(fontSize: 13)), onTap: () => _enter(d)),
              for (final f in files)
                ListTile(dense: true, leading: Icon(Icons.movie_outlined, size: 20, color: context.dimText), title: Text(f.name, style: const TextStyle(fontSize: 13), maxLines: 1, overflow: TextOverflow.ellipsis), trailing: Text(_fmtSize(f.size), style: TextStyle(fontSize: 11, color: context.dimText))),
              if (dirs.isEmpty && files.isEmpty)
                Padding(padding: const EdgeInsets.all(20), child: Center(child: Text('（空目录）', style: TextStyle(color: context.dimText)))),
            ]),
          ),
      ],
    );
  }
}
