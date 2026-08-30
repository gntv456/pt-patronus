// 从 downloaders_page.dart 拆出（part of 主文件，共享私有类型/工具函数；行为不变）。
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// 添加种子：URL / 磁力 + .torrent 文件选择 + 高级（暂停）。
class _AddTorrentSheet extends StatefulWidget {
  final Downloader downloader;
  const _AddTorrentSheet({required this.downloader});
  @override
  State<_AddTorrentSheet> createState() => _AddTorrentSheetState();
}

class _AddTorrentSheetState extends State<_AddTorrentSheet> {
  final _api = DownloaderApi();
  final _urlCtrl = TextEditingController();
  bool _loading = false;
  bool _paused = false;

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _addUrl() async {
    if (_urlCtrl.text.isEmpty) return;
    setState(() => _loading = true);
    try {
      await _api.addTorrent(
        widget.downloader.id,
        _urlCtrl.text,
        opts: {'paused': _paused},
      );
      _toast('已添加');
      if (mounted) Navigator.pop(context);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _pickFile() async {
    setState(() => _loading = true);
    try {
      final result = await appFilePicker.pickFiles(
        type: 'custom',
        allowedExtensions: ['torrent'],
      );
      if (result == null || result.isEmpty) return;
      final f = result.first;
      http.MultipartFile mf;
      if (f.path != null) {
        mf = await http.MultipartFile.fromPath('file', f.path!);
      } else {
        mf = http.MultipartFile.fromBytes(
          'file',
          f.bytes ?? <int>[],
          filename: f.name,
        );
      }
      await _api.uploadTorrent(widget.downloader.id, mf);
      _toast('已上传');
      if (mounted) Navigator.pop(context);
    } catch (e) {
      // 鸿蒙等无原生实现时，file_picker 会抛 MissingPluginException，提示用链接
      _toast('选文件不可用，请用链接添加');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            '添加种子到「${widget.downloader.name}」',
            style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 14),
          TextField(
            controller: _urlCtrl,
            decoration: const InputDecoration(
              hintText: 'magnet:?xt=... 或 .torrent 下载链接',
              border: OutlineInputBorder(),
            ),
            minLines: 1,
            maxLines: 3,
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              Expanded(
                child: FilledButton.icon(
                  onPressed: _loading ? null : _addUrl,
                  icon: const Icon(Icons.link),
                  label: const Text('添加链接'),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: _loading ? null : _pickFile,
                  icon: const Icon(Icons.upload_file),
                  label: const Text('.torrent 文件'),
                ),
              ),
            ],
          ),
          SwitchListTile(
            value: _paused,
            onChanged: _loading ? null : (v) => setState(() => _paused = v),
            title: const Text('以暂停状态添加'),
            contentPadding: EdgeInsets.zero,
            dense: true,
          ),
        ],
      ),
    );
  }
}

