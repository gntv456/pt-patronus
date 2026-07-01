import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:ptpatronus/api/file_storage.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:share_plus/share_plus.dart';

/// 文件管理页：选存储后端 → 浏览目录 → 下载 / 上传 / 新建文件夹。对齐 Web FileManager.vue。
/// 不做删除（后端无路由，Web 也没做）、不做存储 CRUD（请在 Web 端配置）。
class FileManagePage extends StatefulWidget {
  const FileManagePage({super.key});
  @override
  State<FileManagePage> createState() => _FileManagePageState();
}

class _FileManagePageState extends State<FileManagePage> {
  final _api = FileStorageApi();
  List<FileStorage> _storages = [];
  int? _storageId;
  String _path = '/';
  List<FileEntry> _entries = const [];
  bool _loading = false;
  bool _busy = false; // 下载/上传/建目录进行中
  String? _error; // 列目录失败（首次/空时显错误态+重试）

  @override
  void initState() {
    super.initState();
    _loadStorages();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _loadStorages() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _storages = await _api.list();
      if (_storages.isNotEmpty) {
        _selectStorage(_storages.first.id);
        return;
      }
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _selectStorage(int id) {
    final s = _storages.where((x) => x.id == id).toList();
    final base = s.isNotEmpty ? s.first.basePath : '';
    setState(() {
      _storageId = id;
      _path = base.isEmpty ? '/' : base;
      _entries = const [];
      _error = null;
    });
    _loadEntries();
  }

  Future<void> _loadEntries() async {
    final id = _storageId;
    if (id == null) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final entries = await _api.files(id, _path);
      entries.sort((a, b) {
        if (a.isDir != b.isDir) return a.isDir ? -1 : 1; // 目录在前
        return a.name.toLowerCase().compareTo(b.name.toLowerCase());
      });
      _entries = entries;
    } catch (e) {
      final msg = e.toString().replaceFirst('Exception: ', '');
      if (_entries.isEmpty) {
        _error = msg;
      } else {
        _toast(msg);
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _cd(String path) {
    setState(() {
      _path = path.isEmpty ? '/' : path;
      _entries = const [];
    });
    _loadEntries();
  }

  String _parentOf(String p) {
    final segs = p.split('/').where((s) => s.isNotEmpty).toList();
    if (segs.isEmpty) return '/';
    segs.removeLast();
    return segs.isEmpty ? '/' : '/${segs.join('/')}';
  }

  Future<void> _download(FileEntry e) async {
    final id = _storageId;
    if (id == null) return;
    setState(() => _busy = true);
    try {
      final bytes = await _api.download(id, e.path);
      await Share.shareXFiles([
        XFile.fromData(bytes, mimeType: 'application/octet-stream', name: e.name),
      ]);
      _toast('已下载，请在分享面板选择保存位置');
    } on MissingPluginException {
      _toast('当前平台不支持分享/保存，请用 Web 端下载');
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _upload() async {
    final id = _storageId;
    if (id == null) return;
    setState(() => _busy = true);
    try {
      final result = await FilePicker.platform.pickFiles(withData: true);
      if (result == null || result.files.isEmpty) return;
      final f = result.files.first;
      http.MultipartFile mf;
      if (f.path != null) {
        mf = await http.MultipartFile.fromPath('file', f.path!);
      } else {
        mf = http.MultipartFile.fromBytes('file', f.bytes ?? <int>[], filename: f.name);
      }
      await _api.upload(id, _path, mf);
      _toast('已上传');
      await _loadEntries();
    } on MissingPluginException {
      _toast('选文件不可用');
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _mkdir() async {
    final id = _storageId;
    if (id == null) return;
    final name = await _prompt('新建文件夹', '文件夹名称');
    if (name == null || name.isEmpty) return;
    final target = _path == '/' ? '/$name' : '$_path/$name';
    setState(() => _busy = true);
    try {
      await _api.mkdir(id, target);
      _toast('已创建');
      await _loadEntries();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<String?> _prompt(String label, String hint) async {
    final ctrl = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(label),
        content: TextField(
          controller: ctrl,
          decoration: InputDecoration(hintText: hint),
          autofocus: true,
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('取消')),
          TextButton(
              onPressed: () => Navigator.pop(context, ctrl.text.trim()),
              child: const Text('确定')),
        ],
      ),
    );
  }

  Future<void> _delete(FileEntry e) async {
    final id = _storageId;
    if (id == null) return;
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除'),
        content: Text('确认删除「${e.name}」？${e.isDir ? '（目录及其内容）' : ''}'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    setState(() => _busy = true);
    try {
      await _api.remove(id, e.path);
      _toast('已删除');
      await _loadEntries();
    } catch (err) {
      _toast(err.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Widget _storageDropdown() {
    if (_storages.isEmpty) {
      return const Text('无存储后端', style: TextStyle(color: Colors.grey, fontSize: 13));
    }
    return DropdownButton<int>(
      value: _storageId,
      underline: const SizedBox(),
      hint: const Text('选择存储'),
      items: _storages
          .map((s) => DropdownMenuItem(
                value: s.id,
                child: Text('${s.name}（${s.type}）'),
              ))
          .toList(),
      onChanged: (id) {
        if (id != null && id != _storageId) _selectStorage(id);
      },
    );
  }

  Widget _pathBar() {
    final segs = _path.split('/').where((s) => s.isNotEmpty).toList();
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 6),
      child: Wrap(
        spacing: 6,
        runSpacing: 4,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          IconButton(
            icon: const Icon(Icons.arrow_upward, size: 18),
            visualDensity: VisualDensity.compact,
            onPressed: segs.isEmpty ? null : () => _cd(_parentOf(_path)),
          ),
          ActionChip(label: const Text('根'), onPressed: () => _cd('/')),
          ...segs.asMap().entries.map(
                (e) => ActionChip(
                  label: Text(e.value),
                  onPressed: () => _cd('/${segs.sublist(0, e.key + 1).join('/')}'),
                ),
              ),
        ],
      ),
    );
  }

  Widget _entryRow(FileEntry e) => Padding(
        padding: const EdgeInsets.only(bottom: 10),
        child: GlassCard(
          padding: const EdgeInsets.fromLTRB(14, 10, 8, 10),
          onTap: e.isDir ? () => _cd(e.path) : null,
          child: Row(
            children: [
              Icon(
                e.isDir ? Icons.folder : Icons.insert_drive_file,
                size: 24,
                color: e.isDir ? PTColors.primary2 : Colors.grey,
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(e.name,
                        style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis),
                    const SizedBox(height: 2),
                    Text(e.isDir ? '文件夹' : _fmtBytes(e.size),
                        style: const TextStyle(color: Colors.grey, fontSize: 12)),
                  ],
                ),
              ),
              if (!e.isDir)
                IconButton(
                  icon: const Icon(Icons.download, size: 20),
                  onPressed: _busy ? null : () => _download(e),
                ),
              IconButton(
                icon: const Icon(Icons.delete_outline, size: 20),
                onPressed: _busy ? null : () => _delete(e),
              ),
            ],
          ),
        ),
      );

  Widget _errorView() => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, size: 40, color: Colors.redAccent),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Text(_error ?? '加载失败',
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: Colors.redAccent, fontSize: 14)),
            ),
            const SizedBox(height: 12),
            FilledButton.tonalIcon(
              icon: const Icon(Icons.refresh, size: 18),
              label: const Text('重试'),
              onPressed: _loadEntries,
            ),
          ],
        ),
      );

  @override
  Widget build(BuildContext context) {
    final hasStorage = _storageId != null;
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('文件管理'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(
              icon: const Icon(Icons.create_new_folder),
              onPressed: hasStorage && !_busy ? _mkdir : null,
            ),
            IconButton(
              icon: const Icon(Icons.refresh),
              onPressed: hasStorage ? _loadEntries : null,
            ),
          ],
        ),
        floatingActionButton: !hasStorage
            ? null
            : FloatingActionButton(
                onPressed: _busy ? null : _upload,
                child: const Icon(Icons.upload_file),
              ),
        body: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 4, 16, 0),
              child: Row(children: [
                _storageDropdown(),
                const Spacer(),
                if (_busy) const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2)),
              ]),
            ),
            if (hasStorage) _pathBar(),
            if (_loading) const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: _storages.isEmpty && !_loading
                  ? const Center(
                      child: Text('请在 Web 端添加存储后端（WebDAV / Alist / SMB / Agent）',
                          style: TextStyle(color: Colors.grey)))
                  : !hasStorage
                      ? const SizedBox.shrink()
                      : (_error != null && _entries.isEmpty
                          ? _errorView()
                          : _entries.isEmpty && !_loading
                              ? const Center(
                                  child: Text('空目录', style: TextStyle(color: Colors.grey)))
                              : ListView.builder(
                                  padding: const EdgeInsets.fromLTRB(12, 4, 12, 80),
                                  itemCount: _entries.length,
                                  itemBuilder: (_, i) => _entryRow(_entries[i]),
                                )),
            ),
          ],
        ),
      ),
    );
  }
}

/// 字节 → 人类可读（对齐 downloaders_page 的 _fmtBytes）。
String _fmtBytes(int n) {
  if (n <= 0) return '-';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  double s = n.toDouble();
  var i = 0;
  while (s >= 1024 && i < units.length - 1) {
    s /= 1024;
    i++;
  }
  return i == 0 ? '${s.toInt()} B' : '${s.toStringAsFixed(1)} ${units[i]}';
}
