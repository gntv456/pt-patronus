// STRM 生成弹窗。
// （从 file_manage_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/file_manage/file_manage_page.dart';

/// STRM 即时生成表单（云盘文件页选中目录）：输出目录/挂载根/模板/递归/保留扩展名。
/// 对齐 Web FileManager.vue 的 STRM 对话框字段；提交 pop 出 opts Map（drive_id/cloud_path 由调用方补）。
class _StrmSheet extends StatefulWidget {
  final String defaultDir; // 选中目录的云盘路径（仅展示）
  const _StrmSheet({required this.defaultDir});
  @override
  State<_StrmSheet> createState() => _StrmSheetState();
}

class _StrmSheetState extends State<_StrmSheet> {
  final _outputDir = TextEditingController();
  final _mountRoot = TextEditingController();
  final _template = TextEditingController();
  bool _recursive = true;
  bool _keepExt = true;

  @override
  void dispose() {
    _outputDir.dispose();
    _mountRoot.dispose();
    _template.dispose();
    super.dispose();
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: Text(
            '云盘目录：${widget.defaultDir}',
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
        ),
        TextField(
          controller: _outputDir,
          decoration: const InputDecoration(
            labelText: '输出目录',
            hintText: '本地 .strm 输出根目录，如 /Media/STRM',
          ),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _mountRoot,
          decoration: const InputDecoration(
            labelText: '挂载根',
            hintText: '如 /CloudDrive2/115（模板 {mount}）',
          ),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _template,
          decoration: const InputDecoration(
            labelText: '模板',
            hintText: '留空用默认 {mount}{cloudpath}',
          ),
        ),
        const SizedBox(height: 4),
        SwitchListTile(
          dense: true,
          contentPadding: EdgeInsets.zero,
          title: const Text('递归'),
          value: _recursive,
          onChanged: (v) => setState(() => _recursive = v),
        ),
        SwitchListTile(
          dense: true,
          contentPadding: EdgeInsets.zero,
          title: const Text('保留扩展名'),
          value: _keepExt,
          onChanged: (v) => setState(() => _keepExt = v),
        ),
        const SizedBox(height: 12),
        FilledButton(
          onPressed: () {
            final dir = _outputDir.text.trim();
            if (dir.isEmpty) {
              _toast('请填写输出目录');
              return;
            }
            Navigator.pop(context, {
              'output_dir': dir,
              'mount_root': _mountRoot.text.trim(),
              'template': _template.text.trim(),
              'recursive': _recursive,
              'keep_ext': _keepExt,
            });
          },
          child: const Text('生成'),
        ),
      ],
    );
  }
}
