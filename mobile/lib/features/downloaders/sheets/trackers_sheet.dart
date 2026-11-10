// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Tracker 列表弹窗：新增/删行即时生效。
// （从 downloaders_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// Tracker 管理：列表 + 增/删。
class _TrackersSheet extends StatefulWidget {
  final int id;
  final String hash;
  const _TrackersSheet({required this.id, required this.hash});
  @override
  State<_TrackersSheet> createState() => _TrackersSheetState();
}

class _TrackersSheetState extends State<_TrackersSheet> {
  final _api = DownloaderApi();
  final _newCtrl = TextEditingController();
  List<Tracker> _trackers = [];
  bool _loading = true;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final t = await _api.trackers(widget.id, widget.hash);
      if (mounted) {
        setState(() {
          _trackers = t;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  Future<void> _add() async {
    if (_newCtrl.text.trim().isEmpty) return;
    try {
      await _api.addTrackers(widget.id, widget.hash, [_newCtrl.text.trim()]);
      _newCtrl.clear();
      _toast('已添加');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  Future<void> _remove(String url) async {
    try {
      await _api.removeTrackers(widget.id, widget.hash, [url]);
      _toast('已移除');
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
            'Tracker 管理',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _newCtrl,
                  decoration: const InputDecoration(
                    hintText: 'https://tracker.../announce',
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(tooltip: '添加 Tracker', icon: const Icon(Icons.add), onPressed: _add),
            ],
          ),
        ),
        const Divider(height: 16),
        Expanded(
          child: _loading
              ? const PTLoadingView()
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _trackers.isEmpty
              ? Center(
                  child: Text(
                    '无 Tracker',
                    style: TextStyle(color: context.dimText),
                  ),
                )
              : ListView.builder(
                  itemCount: _trackers.length,
                  itemBuilder: (_, i) {
                    final t = _trackers[i];
                    return ListTile(
                      dense: true,
                      title: Text(
                        t.url,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontSize: 13),
                      ),
                      trailing: IconButton(
                        tooltip: '删除该 Tracker',
                        icon: const Icon(Icons.delete_outline, size: 20),
                        onPressed: () => _remove(t.url),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}
