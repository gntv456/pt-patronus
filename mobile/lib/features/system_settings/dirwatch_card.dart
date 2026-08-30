// 从 system_settings_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/system_settings/system_settings_page.dart';

class _DirwatchCard extends StatefulWidget {
  final DirwatchApi api;
  final DirwatchConfig initial;
  const _DirwatchCard({required this.api, required this.initial});
  @override
  State<_DirwatchCard> createState() => _DirwatchCardState();
}

class _DirwatchRow {
  final TextEditingController path;
  final TextEditingController libraryPath;
  _DirwatchRow({String path = '', String libraryPath = ''})
      : path = TextEditingController(text: path),
        libraryPath = TextEditingController(text: libraryPath);

  void dispose() {
    path.dispose();
    libraryPath.dispose();
  }
}

class _DirwatchCardState extends State<_DirwatchCard> {
  late bool _enabled = widget.initial.enabled;
  late String _monitorMode = widget.initial.monitorMode.isEmpty
      ? 'auto'
      : widget.initial.monitorMode;
  late final _debounceCtrl = TextEditingController(
    text: '${widget.initial.debounceMs}',
  );
  late final _pollCtrl = TextEditingController(
    text: '${widget.initial.pollIntervalSec}',
  );
  late final List<_DirwatchRow> _rows = widget.initial.dirs
      .map((d) => _DirwatchRow(path: d.path, libraryPath: d.libraryPath))
      .toList();
  bool _saving = false;

  @override
  void dispose() {
    _debounceCtrl.dispose();
    _pollCtrl.dispose();
    for (final r in _rows) {
      r.dispose();
    }
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  void _addRow() => setState(() => _rows.add(_DirwatchRow()));

  void _removeRow(int i) => setState(() {
        _rows[i].dispose();
        _rows.removeAt(i);
      });

  Future<void> _save() async {
    setState(() => _saving = true);
    // 后端整体替换式 PUT（dirwatch.Config 直接绑定），故必须带上全部字段。
    final cfg = DirwatchConfig(
      enabled: _enabled,
      dirs: _rows
          .where((r) => r.path.text.trim().isNotEmpty)
          .map((r) => DirwatchDir(
                path: r.path.text.trim(),
                libraryPath: r.libraryPath.text.trim(),
              ))
          .toList(),
      monitorMode: _monitorMode,
      debounceMs: int.tryParse(_debounceCtrl.text.trim()) ?? 2000,
      pollIntervalSec: int.tryParse(_pollCtrl.text.trim()) ?? 0,
    );
    try {
      final updated = await widget.api.update(cfg);
      if (!mounted) return;
      // 同步本地行控制器到服务端结果。
      for (final r in _rows) {
        r.dispose();
      }
      setState(() {
        _enabled = updated.enabled;
        _monitorMode = updated.monitorMode.isEmpty ? 'auto' : updated.monitorMode;
        _debounceCtrl.text = '${updated.debounceMs}';
        _pollCtrl.text = '${updated.pollIntervalSec}';
        _rows
          ..clear()
          ..addAll(updated.dirs
              .map((d) => _DirwatchRow(path: d.path, libraryPath: d.libraryPath)));
      });
      _toast('目录监视配置已保存（需重启后端生效）');
    } catch (e) {
      _toast('保存失败：$e');
      await _reload();
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _reload() async {
    try {
      final c = await widget.api.get();
      if (!mounted) return;
      for (final r in _rows) {
        r.dispose();
      }
      setState(() {
        _enabled = c.enabled;
        _monitorMode = c.monitorMode.isEmpty ? 'auto' : c.monitorMode;
        _debounceCtrl.text = '${c.debounceMs}';
        _pollCtrl.text = '${c.pollIntervalSec}';
        _rows
          ..clear()
          ..addAll(c.dirs.map(
              (d) => _DirwatchRow(path: d.path, libraryPath: d.libraryPath)));
      });
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const GlassIcon(
                glyph: 'files',
                from: Color(0xFF6C5CE7),
                to: Color(0xFF00B894),
                size: 32,
              ),
              const SizedBox(width: 10),
              const Expanded(
                child: Text('目录监视',
                    style:
                        TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              ),
              Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '实时监视本地目录，发现媒体文件即喂给整理流水线'
            '（补 DoneHook + cron 覆盖不到的手动投放 / 非托管来源）。',
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              SizedBox(
                width: 96,
                child: Text('监视目录',
                    style: TextStyle(
                        fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
              ),
              const Spacer(),
              TextButton.icon(
                icon: const Icon(Icons.add, size: 16),
                label: const Text('添加目录'),
                onPressed: _addRow,
              ),
            ],
          ),
          const SizedBox(height: 4),
          for (int i = 0; i < _rows.length; i++)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: TextField(
                      controller: _rows[i].path,
                      decoration: const InputDecoration(
                        isDense: true,
                        hintText: '源目录 /downloads/movies',
                        border: OutlineInputBorder(),
                      ),
                    ),
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: TextField(
                      controller: _rows[i].libraryPath,
                      decoration: const InputDecoration(
                        isDense: true,
                        hintText: '媒体库 /library/movies',
                        border: OutlineInputBorder(),
                      ),
                    ),
                  ),
                  IconButton(
                    tooltip: '删除',
                    icon: Icon(Icons.delete_outline,
                        size: 20, color: PTColors.danger),
                    onPressed: () => _removeRow(i),
                  ),
                ],
              ),
            ),
          if (_rows.isEmpty)
            Padding(
              padding: EdgeInsets.symmetric(vertical: 8),
              child: Text('暂无监视目录，点「添加目录」新建',
                  style: TextStyle(fontSize: 12, color: context.dimText)),
            ),
          const SizedBox(height: 4),
          _field(
            '监视模式',
            DropdownButton<String>(
              value: _monitorMode == 'compatibility' ? 'compatibility' : 'auto',
              isExpanded: true,
              items: const [
                DropdownMenuItem(
                    value: 'auto', child: Text('auto（fsnotify 事件驱动）')),
                DropdownMenuItem(
                    value: 'compatibility',
                    child: Text('compatibility（轮询兜底）')),
              ],
              onChanged: (v) => setState(() => _monitorMode = v ?? 'auto'),
            ),
          ),
          _field(
            '事件去抖（毫秒）',
            TextField(
              controller: _debounceCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
          ),
          if (_monitorMode == 'compatibility')
            _field(
              '轮询间隔（秒，仅 compatibility 模式）',
              TextField(
                controller: _pollCtrl,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(
                  isDense: true,
                  border: OutlineInputBorder(),
                ),
              ),
            ),
          const SizedBox(height: 8),
          FilledButton.icon(
            onPressed: _saving ? null : _save,
            icon: _saving
                ? const SizedBox(
                    width: 16, height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                : const Icon(Icons.save_outlined, size: 18),
            label: const Text('保存'),
          ),
        ],
      ),
    );
  }

  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }
}
