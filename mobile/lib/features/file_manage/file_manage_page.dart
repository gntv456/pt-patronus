import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'dart:async';
import 'package:dio/dio.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:path_provider/path_provider.dart';
import 'package:ptpatronus/api/file_storage.dart';
import 'package:ptpatronus/api/monitor.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:share_plus/share_plus.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 文件管理页：存储后端 CRUD（WebDAV/Alist/SMB/Agent）→ 浏览目录 → 下载/上传/建目录/删除。
/// 对齐 Web FileManager.vue。
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

  FileStorage? get _current =>
      _storages.where((x) => x.id == _storageId).firstOrNull;

  // —— 存储后端 CRUD（对齐 Web FileManager.vue 的「添加存储」+ 类型含 Agent）——
  Future<void> _addStorage() async {
    final ok = await showAdaptiveSheet(
      context,
      title: '添加存储',
      wideWidth: 480,
      builder: (_) => const _StorageFormSheet(),
    );
    if (ok == true) {
      _loadStorages();
    }
  }

  Future<void> _editStorage() async {
    final s = _current;
    if (s == null) return;
    final ok = await showAdaptiveSheet(
      context,
      title: '编辑存储',
      wideWidth: 480,
      builder: (_) => _StorageFormSheet(editing: s),
    );
    if (ok == true) _loadStorages();
  }

  Future<void> _deleteStorage() async {
    final s = _current;
    if (s == null) return;
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除存储'),
        content: Text('删除存储后端「${s.name}」？仅移除配置，不影响远端文件。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            style: FilledButton.styleFrom(backgroundColor: Colors.redAccent),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.delete(s.id);
      _toast('已删除');
      _storageId = null;
      await _loadStorages();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
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
    final cancel = CancelToken();
    final progress = ValueNotifier<double>(-1); // -1=未知大小，否则 0..1
    final dir = await getTemporaryDirectory();
    final savePath = '${dir.path}/${e.name}';
    if (!mounted) return;
    // 流式下载写盘 + 进度/取消对话框（避免大文件全量入内存 OOM——审计 P1）
    unawaited(
      showDialog(
        context: context,
        barrierDismissible: false,
        builder: (_) => AlertDialog(
          title: const Text('下载中'),
          content: ValueListenableBuilder<double>(
            valueListenable: progress,
            builder: (_, p, _) => Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  e.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 13),
                ),
                const SizedBox(height: 12),
                LinearProgressIndicator(value: p < 0 ? null : p),
                if (p >= 0)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(
                      '${(p * 100).toStringAsFixed(0)}%',
                      style: const TextStyle(fontSize: 12, color: Colors.grey),
                    ),
                  ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () {
                if (!cancel.isCancelled) cancel.cancel('user');
              },
              child: const Text('取消'),
            ),
          ],
        ),
      ),
    );
    try {
      await _api.downloadToFile(
        id,
        e.path,
        savePath,
        onProgress: (r, t) => progress.value = t > 0 ? r / t : -1,
        cancelToken: cancel,
      );
      if (!mounted) return;
      Navigator.of(context).pop(); // 关进度框
      await Share.shareXFiles([XFile(savePath)]);
      _toast('已下载，请在分享面板选择保存位置');
    } on DioException catch (err) {
      if (mounted) Navigator.of(context).pop();
      if (mounted && !CancelToken.isCancel(err)) _toast('下载失败：${err.message}');
    } on MissingPluginException {
      if (mounted) Navigator.of(context).pop();
      _toast('当前平台不支持分享/保存，请用 Web 端下载');
    } catch (err) {
      if (mounted) Navigator.of(context).pop();
      _toast(err.toString().replaceFirst('Exception: ', ''));
    } finally {
      progress.dispose();
    }
  }

  Future<void> _upload() async {
    final id = _storageId;
    if (id == null) return;
    setState(() => _busy = true);
    try {
      final result = await FilePicker.platform.pickFiles(
        withData: false,
      ); // 不读入内存：用 fromPath 流式上传
      if (result == null || result.files.isEmpty) return;
      final f = result.files.first;
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
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, ctrl.text.trim()),
            child: const Text('确定'),
          ),
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
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
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
      return const Text(
        '无存储后端',
        style: TextStyle(color: Colors.grey, fontSize: 13),
      );
    }
    return DropdownButton<int>(
      value: _storageId,
      underline: const SizedBox(),
      hint: const Text('选择存储'),
      items: _storages
          .map(
            (s) => DropdownMenuItem(
              value: s.id,
              child: Text('${s.name}（${s.type}）'),
            ),
          )
          .toList(),
      onChanged: (id) {
        if (id != null && id != _storageId) _selectStorage(id);
      },
    );
  }

  Widget _pathBar() {
    final segs = _path.split('/').where((s) => s.isNotEmpty).toList();
    // 单行横向滚动面包屑，深路径永不折行（取代旧 Wrap 折多行吃竖向空间）。
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 6),
      child: SizedBox(
        height: 38,
        child: SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: [
              InkWell(
                onTap: segs.isEmpty ? null : () => _cd(_parentOf(_path)),
                borderRadius: BorderRadius.circular(8),
                child: const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 6, vertical: 8),
                  child: Icon(Icons.arrow_upward, size: 18),
                ),
              ),
              ActionChip(label: const Text('根'), onPressed: () => _cd('/')),
              ...segs.asMap().entries.map(
                (e) => ActionChip(
                  label: Text(e.value),
                  onPressed: () =>
                      _cd('/${segs.sublist(0, e.key + 1).join('/')}'),
                ),
              ),
            ],
          ),
        ),
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
                Text(
                  e.name,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 15,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                const SizedBox(height: 2),
                Text(
                  e.isDir ? '文件夹' : _fmtBytes(e.size),
                  style: const TextStyle(color: Colors.grey, fontSize: 12),
                ),
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
          child: Text(
            _error ?? '加载失败',
            textAlign: TextAlign.center,
            style: const TextStyle(color: Colors.redAccent, fontSize: 14),
          ),
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
    return PTPageScaffold(
      title: '文件管理',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          onPressed: hasStorage ? _loadEntries : null,
        ),
        PTBarAction(
          icon: Icons.create_new_folder,
          label: '新建文件夹',
          onPressed: (hasStorage && !_busy) ? _mkdir : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.add,
          label: '添加存储',
          onPressed: _addStorage,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.edit,
          label: '编辑当前存储',
          onPressed: _current != null ? _editStorage : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.delete,
          label: '删除当前存储',
          onPressed: _current != null ? _deleteStorage : null,
          overflow: true,
        ),
      ],
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
            child: Row(
              children: [
                _storageDropdown(),
                const Spacer(),
                if (_busy)
                  const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
              ],
            ),
          ),
          if (hasStorage) _pathBar(),
          if (_loading) const LinearProgressIndicator(minHeight: 2),
          Expanded(
            child: _storages.isEmpty && !_loading
                ? const Center(
                    child: Text(
                      '暂无存储后端，请点右上角更多添加 WebDAV / Alist / SMB / Agent',
                      style: TextStyle(color: Colors.grey),
                    ),
                  )
                : !hasStorage
                ? const SizedBox.shrink()
                : (_error != null && _entries.isEmpty
                      ? _errorView()
                      : _entries.isEmpty && !_loading
                      ? const Center(
                          child: Text(
                            '空目录',
                            style: TextStyle(color: Colors.grey),
                          ),
                        )
                      : ListView.builder(
                          padding: const EdgeInsets.fromLTRB(12, 4, 12, 80),
                          itemCount: _entries.length,
                          itemBuilder: (_, i) => _entryRow(_entries[i]),
                        )),
          ),
        ],
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

/// 存储后端新建/编辑表单：name/type(webdav/alist/smb/agent)/agent选择器/url/user/pass/base_path。
/// 编辑态密码留空=不修改。type=agent 时用 Agent 选择器替代地址凭据（对齐 Web FileManager.vue）。
class _StorageFormSheet extends StatefulWidget {
  final FileStorage? editing; // null=新建
  const _StorageFormSheet({this.editing});
  @override
  State<_StorageFormSheet> createState() => _StorageFormSheetState();
}

class _StorageFormSheetState extends State<_StorageFormSheet> {
  final _api = FileStorageApi();
  final _agentApi = AgentApi();
  final _name = TextEditingController();
  final _url = TextEditingController();
  final _username = TextEditingController();
  final _password = TextEditingController();
  final _basePath = TextEditingController(text: '/');
  String _type = 'webdav';
  List<Agent> _agents = const [];
  int _agentId = 0;
  bool _saving = false;
  bool _loadingAgents = true;

  bool get _isEdit => widget.editing != null;

  @override
  void initState() {
    super.initState();
    final e = widget.editing;
    if (e != null) {
      _name.text = e.name;
      _type = e.type.isEmpty ? 'webdav' : e.type;
      _url.text = e.url;
      _username.text = e.username;
      _basePath.text = e.basePath.isEmpty ? '/' : e.basePath;
      _agentId = e.agentId;
    }
    _agentApi
        .list()
        .then((l) {
          if (mounted) {
            setState(() {
              _agents = l;
              _loadingAgents = false;
            });
          }
        })
        .catchError((_) {
          if (mounted) setState(() => _loadingAgents = false);
        });
  }

  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    _username.dispose();
    _password.dispose();
    _basePath.dispose();
    super.dispose();
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _submit() async {
    final name = _name.text.trim();
    if (name.isEmpty) {
      _toast('请填写名称');
      return;
    }
    if (_type == 'agent') {
      if (_agentId == 0) {
        _toast('请选择 Agent');
        return;
      }
    } else if (_url.text.trim().isEmpty) {
      _toast('请填写地址');
      return;
    }
    setState(() => _saving = true);
    try {
      final in_ = FileStorageInput(
        name: name,
        type: _type,
        url: _url.text.trim(),
        username: _username.text.trim(),
        password: _password.text,
        basePath: _basePath.text.trim().isEmpty ? '/' : _basePath.text.trim(),
        agentId: _agentId,
      );
      if (_isEdit) {
        await _api.update(widget.editing!.id, in_);
        _toast('已更新');
      } else {
        await _api.create(in_);
        _toast('已添加');
      }
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: _name,
          decoration: const InputDecoration(labelText: '名称'),
        ),
        const SizedBox(height: 12),
        DropdownButtonFormField<String>(
          initialValue: _type,
          decoration: const InputDecoration(labelText: '类型'),
          items: const [
            DropdownMenuItem(value: 'webdav', child: Text('WebDAV')),
            DropdownMenuItem(value: 'alist', child: Text('Alist')),
            DropdownMenuItem(value: 'smb', child: Text('SMB')),
            DropdownMenuItem(value: 'agent', child: Text('Agent')),
          ],
          onChanged: (v) => setState(() => _type = v ?? 'webdav'),
        ),
        if (_type == 'agent') ...[
          const SizedBox(height: 12),
          // agents 异步加载：加载完再建下拉，initialValue 才能正确预选编辑态的 agent。
          _loadingAgents
              ? const Padding(
                  padding: EdgeInsets.symmetric(vertical: 8),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
                  ),
                )
              : DropdownButtonFormField<int>(
                  initialValue:
                      (_agentId == 0 || _agents.any((a) => a.id == _agentId))
                      ? _agentId
                      : 0,
                  decoration: const InputDecoration(labelText: 'Agent'),
                  items: [
                    const DropdownMenuItem(value: 0, child: Text('选择 Agent')),
                    ..._agents.map(
                      (a) => DropdownMenuItem(
                        value: a.id,
                        child: Text('${a.name}${a.online ? '' : '（离线）'}'),
                      ),
                    ),
                  ],
                  onChanged: (v) => setState(() => _agentId = v ?? 0),
                ),
          const SizedBox(height: 8),
          const Text(
            '选择已部署 ptpatronus-agent 的设备（飞牛/绿联等），根路径填 agent 机器上的绝对路径。设备离线或版本过低（需 0.2.0+）时文件操作不可用。',
            style: TextStyle(fontSize: 11, color: Colors.grey, height: 1.5),
          ),
        ] else ...[
          const SizedBox(height: 12),
          TextField(
            controller: _url,
            decoration: const InputDecoration(
              labelText: '地址',
              hintText: 'http://host:port',
            ),
            keyboardType: TextInputType.url,
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _username,
            decoration: const InputDecoration(labelText: '用户名'),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _password,
            obscureText: true,
            decoration: InputDecoration(
              labelText: '密码',
              hintText: _isEdit ? '留空不修改' : null,
            ),
          ),
        ],
        const SizedBox(height: 12),
        TextField(
          controller: _basePath,
          decoration: const InputDecoration(labelText: '根路径'),
        ),
        const SizedBox(height: 20),
        FilledButton(
          onPressed: _saving ? null : _submit,
          child: _saving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: Colors.white,
                  ),
                )
              : Text(_isEdit ? '保存' : '添加'),
        ),
      ],
    );
  }
}
