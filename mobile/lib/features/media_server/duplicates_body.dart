// 从 media_server_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/media_server/media_server_page.dart';

/// 重复检测 sheet 主体：选库 → 检测 → 展示重复组。
class _DuplicatesBody extends StatefulWidget {
  final int serverId;
  final MediaServerApi api;
  const _DuplicatesBody({required this.serverId, required this.api});
  @override
  State<_DuplicatesBody> createState() => _DuplicatesBodyState();
}

class _DuplicatesBodyState extends State<_DuplicatesBody> {
  List<MediaLibrary> _libs = const [];
  String? _libId;
  List<DuplicateGroup> _groups = const [];
  bool _loading = true;
  bool _checking = false;

  @override
  void initState() {
    super.initState();
    _loadLibs();
  }

  Future<void> _loadLibs() async {
    try {
      _libs = await widget.api.libraries(widget.serverId);
      _libId = _libs.isNotEmpty ? _libs.first.id : null;
    } catch (_) {}
    if (mounted) setState(() => _loading = false);
  }

  Future<void> _run() async {
    if (_libId == null) return;
    setState(() => _checking = true);
    try {
      _groups = await widget.api.duplicates(widget.serverId, _libId!);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('查重失败：$e')));
      }
    } finally {
      if (mounted) setState(() => _checking = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const PTLoadingView();
    return Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        Expanded(
          child: DropdownButton<String>(
            value: _libId,
            isExpanded: true,
            hint: const Text('选择媒体库'),
            items: _libs
                .map((l) => DropdownMenuItem(value: l.id, child: Text(l.name)))
                .toList(),
            onChanged: (v) => setState(() => _libId = v),
          ),
        ),
        const SizedBox(width: 8),
        FilledButton(onPressed: _checking ? null : _run, child: const Text('检测')),
      ]),
      const SizedBox(height: 8),
      Flexible(
        child: _checking
            ? const PTLoadingView()
            : ListView(shrinkWrap: true, children: [
                Text('共 ${_groups.length} 组疑似重复',
                    style: TextStyle(fontSize: 12, color: context.dimText)),
                ..._groups.map((g) => Card(
                      margin: const EdgeInsets.symmetric(vertical: 4),
                      child: Padding(
                        padding: const EdgeInsets.all(8),
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Row(children: [
                            Expanded(
                              child: Text('${g.title}${g.year > 0 ? ' (${g.year})' : ''}',
                                  style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                            ),
                            Text('${g.count} 份',
                                style: const TextStyle(fontSize: 12, color: Colors.orange)),
                          ]),
                          ...g.items.map((n) => Text('· $n',
                              style: TextStyle(fontSize: 11, color: context.dimText))),
                        ]),
                      ),
                    )),
              ]),
      ),
    ]);
  }
}
