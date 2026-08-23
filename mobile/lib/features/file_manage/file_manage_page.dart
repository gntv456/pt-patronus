import 'package:ptpatronus/core/platform/file_picker.dart';
import 'package:flutter/material.dart';
import 'dart:async';
import 'package:dio/dio.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:path_provider/path_provider.dart';
import 'package:ptpatronus/api/cloud.dart';
import 'package:ptpatronus/api/file_storage.dart';
import 'package:ptpatronus/api/monitor.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:share_plus/share_plus.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 文件管理页：存储后端 CRUD（本机/WebDAV/Alist/SMB/Agent）+ 云盘只读浏览 → 下载/上传/建目录/删除/STRM。
/// 对齐 Web FileManager.vue。
/// 存储连通性探测状态（瞬时，仅用于 UI 展示）。
enum _Conn { checking, ok, fail }

/// 存储后端统一引用：fs（本机/WebDAV/Alist/SMB/Agent/Rclone，可读写）｜cloud（115/夸克/123，只读 + STRM）。
/// 用 kind+id 复合 key 区分——fs 与 cloud 的 id 是独立 DB 序列（都从 1 起），单 int 会冲突。
/// 对齐 Web FileManager.vue 的 StorageItem。
class _Backend {
  final String kind; // 'fs' | 'cloud'
  final FileStorage? fs;
  final CloudDrive? drive;
  const _Backend({this.kind = 'fs', this.fs, this.drive});

  int get id => kind == 'fs' ? (fs?.id ?? 0) : (drive?.id ?? 0);
  String get name => kind == 'fs' ? (fs?.name ?? '') : (drive?.name ?? '');
  String get typeName => kind == 'fs' ? (fs?.type ?? '') : (drive?.type ?? '');
  bool get readOnly => kind == 'cloud';

  /// 进入该后端时的虚拟根路径。
  /// fs.local 根固定 '/'（basePath 是服务端物理根，不能当虚拟路径）；
  /// fs 其他类型用 basePath；cloud 用 root_path。均空兜底 '/'。
  String get rootPath {
    if (kind == 'fs') {
      final f = fs;
      if (f == null) return '/';
      if (f.type == 'local') return '/';
      return f.basePath.isEmpty ? '/' : f.basePath;
    }
    final d = drive;
    return (d == null || d.rootPath.isEmpty) ? '/' : d.rootPath;
  }

  /// 复合 key（'fs:3' / 'cloud:1'）：连通性状态 Map 索引 + 下拉去重。
  String get key => '$kind:$id';

  @override
  bool operator ==(Object o) => o is _Backend && o.key == key;
  @override
  int get hashCode => key.hashCode;
}

class FileManagePage extends StatefulWidget {
  const FileManagePage({super.key});
  @override
  State<FileManagePage> createState() => _FileManagePageState();
}

class _FileManagePageState extends State<FileManagePage> {
  final _api = FileStorageApi();
  final _cloud = CloudApi();
  List<_Backend> _backends = [];
  _Backend? _cur; // 当前后端（fs 可读写 / cloud 只读）
  String _path = '/';
  // 连通性探测状态（瞬时，不持久化）：复合 key → 状态
  final Map<String, _Conn> _status = {};
  List<FileEntry> _entries = const [];
  bool _loading = false;
  bool _busy = false; // 下载/上传/建目录/STRM 进行中
  String? _error; // 列目录失败（首次/空时显错误态+重试）
  // 请求竞态守卫：每次加载自增序号，过期响应（序号不匹配）被丢弃，
  // 防止切换后端/进目录后旧响应覆盖新视图（路径=B 但列表=A 的错乱）。
  int _loadSeq = 0;

  bool get _readOnly => _cur?.readOnly ?? false;

  @override
  void initState() {
    super.initState();
    _loadBackends();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _loadBackends() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    // _select 已触发 → 其 _loadEntries 接管 _loading，finally 不再覆盖（否则进度条闪烁）。
    var selected = false;
    try {
      // fs 与 cloud 并行加载；云盘接口失败不应阻断本地存储（对齐 Web Promise.all + catch([])）。
      final lateCloud = _cloud.listDrives().catchError((_) => <CloudDrive>[]);
      final fs = await _api.list();
      final drives = await lateCloud;
      final next = <_Backend>[
        ...fs.map((f) => _Backend(kind: 'fs', fs: f)),
        ...drives.map((d) => _Backend(kind: 'cloud', drive: d)),
      ];
      _backends = next;
      unawaited(_probeAll());
      if (next.isNotEmpty) {
        _select(next.first);
        selected = true;
        return;
      }
    } catch (e) {
      _error = e.toString();
    } finally {
      // 仅在未触发 _select（无后端/出错分支）时收尾 _loading。
      if (mounted && !selected) setState(() => _loading = false);
    }
  }

  void _select(_Backend b) {
    setState(() {
      _cur = b;
      _path = b.rootPath;
      _entries = const [];
      _error = null;
    });
    _loadEntries();
  }

  // —— 存储后端 CRUD（对齐 Web FileManager.vue 的「添加存储」+ 类型含 Agent）——
  Future<void> _addStorage() async {
    final ok = await showAdaptiveSheet(
      context,
      title: '添加存储',
      wideWidth: 480,
      builder: (_) => const _StorageFormSheet(),
    );
    if (ok == true) {
      _loadBackends();
    }
  }

  Future<void> _editStorage() async {
    final b = _cur;
    if (b == null || b.fs == null) return; // 仅 fs 可在本页编辑（云盘在「云盘」页管理）
    final ok = await showAdaptiveSheet(
      context,
      title: '编辑存储',
      wideWidth: 480,
      builder: (_) => _StorageFormSheet(editing: b.fs!),
    );
    if (ok == true) _loadBackends();
  }

  Future<void> _deleteStorage() async {
    final b = _cur;
    if (b == null || b.fs == null) return; // 仅 fs 可在本页删除
    final s = b.fs!;
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
      _cur = null;
      await _loadBackends();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  // —— 连通性探测 ——
  // 加载后自动并发探测所有后端（静默，仅更新下拉项状态点）。
  Future<void> _probeAll() async {
    if (!mounted) return;
    setState(() {
      for (final b in _backends) {
        _status[b.key] = _Conn.checking;
      }
    });
    await Future.wait(_backends.map((b) => _probe(b)));
  }

  /// 统一连通性探测：fs 用 FileStorageApi.test（{ok,error}）；cloud 用 CloudApi.test。
  Future<bool> _testOk(_Backend b) async {
    if (b.kind == 'fs') return (await _api.test(b.id)).ok;
    final r = await _cloud.test(b.id);
    return r['ok'] == true;
  }

  Future<void> _probe(_Backend b) async {
    try {
      final ok = await _testOk(b);
      if (!mounted) return;
      setState(() {
        _status[b.key] = ok ? _Conn.ok : _Conn.fail;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _status[b.key] = _Conn.fail;
      });
    }
  }

  // 手动测试当前后端（即时 toast 反馈 + 更新状态点）。
  Future<void> _testCurrent() async {
    final b = _cur;
    if (b == null) return;
    setState(() => _status[b.key] = _Conn.checking);
    try {
      final ok = await _testOk(b);
      if (!mounted) return;
      setState(() {
        _status[b.key] = ok ? _Conn.ok : _Conn.fail;
      });
      _toast(ok ? '「${b.name}」连接正常' : '「${b.name}」连接失败');
    } catch (e) {
      if (!mounted) return;
      setState(() => _status[b.key] = _Conn.fail);
      _toast('「${b.name}」连接失败：${e.toString().replaceFirst('Exception: ', '')}');
    }
  }

  String _statusDot(String key) {
    final st = _status[key];
    if (st == _Conn.ok) return '🟢 ';
    if (st == _Conn.fail) return '🔴 ';
    if (st == _Conn.checking) return '⚪ ';
    return '';
  }

  Future<void> _loadEntries() async {
    final b = _cur;
    if (b == null) return;
    // 竞态守卫：序号不匹配的过期响应直接丢弃，防止旧请求晚到覆盖新视图。
    final my = ++_loadSeq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final List<FileEntry> entries;
      if (b.kind == 'fs') {
        entries = await _api.files(b.id, _path);
      } else {
        // 云盘只读浏览：CloudFile → FileEntry（无 mod_time）。
        final files = await _cloud.browse(b.id, _path);
        entries = files
            .map((f) => FileEntry(
                  name: f.name,
                  path: f.path,
                  isDir: f.isDir,
                  size: f.size,
                  modTime: '',
                ))
            .toList();
      }
      // 丢弃过期响应：切换后端/进目录后旧请求晚到，不能覆盖新视图。
      if (!mounted || my != _loadSeq) return;
      entries.sort((a, b2) {
        if (a.isDir != b2.isDir) return a.isDir ? -1 : 1; // 目录在前
        return a.name.toLowerCase().compareTo(b2.name.toLowerCase());
      });
      _entries = entries;
    } catch (e) {
      if (!mounted || my != _loadSeq) return;
      final msg = e.toString().replaceFirst('Exception: ', '');
      if (_entries.isEmpty) {
        _error = msg;
      } else {
        _toast(msg);
      }
    } finally {
      // 仅当本次仍是最新序号时才收尾 _loading，避免覆盖更新一轮加载的状态。
      if (mounted && my == _loadSeq) setState(() => _loading = false);
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
    final b = _cur;
    if (b == null || b.readOnly) return;
    final id = b.id;
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
                      style: TextStyle(fontSize: 12, color: context.dimText),
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
    final b = _cur;
    if (b == null || b.readOnly) return;
    final id = b.id;
    setState(() => _busy = true);
    try {
      final result = await appFilePicker.pickFiles(
        withData: false,
      ); // 不读入内存：用 fromPath 流式上传
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
    final b = _cur;
    if (b == null || b.readOnly) return;
    final id = b.id;
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

  Future<String?> _prompt(String label, String hint, {String initial = ''}) async {
    final ctrl = TextEditingController(text: initial);
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
    final b = _cur;
    if (b == null || b.readOnly) return;
    final id = b.id;
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

  Future<void> _rename(FileEntry e) async {
    final b = _cur;
    if (b == null || b.readOnly) return;
    final id = b.id;
    final name = await _prompt('重命名', '新名称', initial: e.name);
    if (name == null || name.isEmpty) return;
    setState(() => _busy = true);
    try {
      await _api.rename(id, e.path, name);
      _toast('已重命名');
      await _loadEntries();
    } catch (e2) {
      _toast(e2.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _moveCopy(FileEntry e, String mode) async {
    final b = _cur;
    if (b == null || b.readOnly) return;
    final id = b.id;
    final target = await showAdaptiveSheet<String>(
      context,
      title: mode == 'move' ? '移动到…' : '复制到…',
      wideWidth: 520,
      builder: (_) => _MoveCopyPicker(api: _api, storageId: id, initialPath: _path),
    );
    if (target == null || target.isEmpty) return;
    setState(() => _busy = true);
    try {
      if (mode == 'move') {
        await _api.move(id, e.path, target);
      } else {
        await _api.copy(id, e.path, target);
      }
      _toast(mode == 'move' ? '已移动' : '已复制');
      await _loadEntries();
    } catch (e2) {
      _toast(e2.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// 云盘：对选中目录即时生成 STRM（不持久化为规则）。仅 cloud 可用。
  /// path 为目标目录的云盘路径（可来自 entry.path 或当前路径 _path，使根目录也能生成）。
  Future<void> _strm(String path) async {
    final b = _cur;
    if (b == null || b.kind != 'cloud' || b.drive == null) return;
    final driveId = b.drive!.id;
    final res = await showAdaptiveSheet<Map<String, dynamic>>(
      context,
      title: '生成 STRM',
      wideWidth: 480,
      builder: (_) => _StrmSheet(defaultDir: path),
    );
    if (res == null) return;
    setState(() => _busy = true);
    try {
      final r = await _cloud.generate({
        'drive_id': driveId,
        'cloud_path': path,
        'output_dir': res['output_dir'] ?? '',
        'mount_root': res['mount_root'] ?? '',
        'template': res['template'] ?? '',
        'recursive': res['recursive'] ?? true,
        'keep_ext': res['keep_ext'] ?? true,
      });
      final gen = r['generated'] ?? 0;
      final skip = r['skipped'] ?? 0;
      final errs = (r['errors'] as List?) ?? const [];
      _toast('STRM 已生成：新增 $gen，跳过 $skip${errs.isNotEmpty ? '，失败 ${errs.length}' : ''}');
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Widget _backendDropdown(BuildContext context) {
    if (_backends.isEmpty) {
      return Text(
        '无存储后端',
        style: TextStyle(color: context.dimText, fontSize: 13),
      );
    }
    return DropdownButton<_Backend>(
      value: _cur,
      underline: const SizedBox(),
      hint: const Text('选择存储'),
      items: _backends
          .map(
            (b) => DropdownMenuItem(
              value: b,
              child: Text(
                  '${_statusDot(b.key)}${b.kind == 'cloud' ? '☁ ' : ''}${b.name}（${b.typeName}）'),
            ),
          )
          .toList(),
      onChanged: (v) {
        if (v != null && v != _cur) _select(v);
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

  Widget _entryRow(BuildContext context, FileEntry e) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 10, 8, 10),
      onTap: e.isDir ? () => _cd(e.path) : null,
      child: Row(
        children: [
          Icon(
            e.isDir ? Icons.folder : Icons.insert_drive_file,
            size: 24,
            color: e.isDir ? PTColors.primary2 : context.dimText,
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
                  style: TextStyle(color: context.dimText, fontSize: 12),
                ),
              ],
            ),
          ),
          if (!e.isDir && !_readOnly)
            IconButton(
              icon: const Icon(Icons.download, size: 20),
              tooltip: '下载',
              onPressed: _busy ? null : () => _download(e),
            ),
          if (_readOnly && e.isDir)
            IconButton(
              icon: const Icon(Icons.link, size: 20),
              tooltip: '生成 STRM',
              onPressed: _busy ? null : () => _strm(e.path),
            ),
          if (!_readOnly)
            PopupMenuButton<String>(
              icon: const Icon(Icons.more_vert, size: 20),
              tooltip: '更多',
              enabled: !_busy,
              onSelected: (cmd) {
                if (cmd == 'rename') {
                  _rename(e);
                } else if (cmd == 'move') {
                  _moveCopy(e, 'move');
                } else if (cmd == 'copy') {
                  _moveCopy(e, 'copy');
                } else if (cmd == 'delete') {
                  _delete(e);
                }
              },
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'rename', child: Text('重命名')),
                PopupMenuItem(value: 'move', child: Text('移动')),
                PopupMenuItem(value: 'copy', child: Text('复制')),
                PopupMenuItem(value: 'delete', child: Text('删除')),
              ],
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
    final b = _cur;
    final hasStorage = b != null;
    final readOnly = b?.readOnly ?? false;
    return PTPageScaffold(
      title: '文件管理',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          onPressed: hasStorage ? _loadEntries : null,
        ),
        // 云盘只读模式：对当前目录（含根目录，根无列表行无操作入口）生成 STRM。
        if (readOnly)
          PTBarAction(
            icon: Icons.link,
            label: '生成 STRM',
            onPressed: _busy ? null : () => _strm(_path),
          ),
        PTBarAction(
          icon: Icons.create_new_folder,
          label: '新建文件夹',
          onPressed: (hasStorage && !_busy && !readOnly) ? _mkdir : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.add,
          label: '添加存储',
          onPressed: _addStorage,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.network_check,
          label: '测试连接',
          onPressed: hasStorage ? _testCurrent : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.edit,
          label: '编辑当前存储',
          onPressed: (hasStorage && b.fs != null) ? _editStorage : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.delete,
          label: '删除当前存储',
          onPressed: (hasStorage && b.fs != null) ? _deleteStorage : null,
          overflow: true,
        ),
      ],
      floatingActionButton: (!hasStorage || readOnly)
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
                _backendDropdown(context),
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
            child: _backends.isEmpty && !_loading
                ? Center(
                    child: Text(
                      '暂无存储后端，请点右上角更多添加 本机 / WebDAV / Alist / SMB / Agent（云盘在「云盘」页管理）',
                      style: TextStyle(color: context.dimText),
                    ),
                  )
                : !hasStorage
                ? const SizedBox.shrink()
                : (_error != null && _entries.isEmpty
                      ? _errorView()
                      : _entries.isEmpty && !_loading
                      ? Center(
                          child: Text(
                            '空目录',
                            style: TextStyle(color: context.dimText),
                          ),
                        )
                      : ListView.builder(
                          padding: const EdgeInsets.fromLTRB(12, 4, 12, 80),
                          itemCount: _entries.length,
                          itemBuilder: (ctx, i) => _entryRow(ctx, _entries[i]),
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

/// 存储后端新建/编辑表单：name/type(webdav/alist/smb/agent/local)/agent选择器/url/user/pass/base_path。
/// 编辑态密码留空=不修改。type=agent 用 Agent 选择器替代地址凭据；type=local 仅需根路径（本机物理目录）（对齐 Web FileManager.vue）。
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
    } else if (_type == 'rclone') {
      if (_username.text.trim().isEmpty) {
        _toast('请填写 Remote 名');
        return;
      }
    } else if (_type != 'local' && _url.text.trim().isEmpty) {
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
            DropdownMenuItem(value: 'local', child: Text('本机')),
            DropdownMenuItem(value: 'webdav', child: Text('WebDAV')),
            DropdownMenuItem(value: 'alist', child: Text('Alist')),
            DropdownMenuItem(value: 'smb', child: Text('SMB')),
            DropdownMenuItem(value: 'agent', child: Text('Agent')),
            DropdownMenuItem(value: 'rclone', child: Text('Rclone')),
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
          Text(
            '选择已部署 ptpatronus-agent 的设备（飞牛/绿联等），根路径填 agent 机器上的绝对路径。设备离线或版本过低（需 0.2.0+）时文件操作不可用。',
            style: TextStyle(fontSize: 11, color: context.dimText, height: 1.5),
          ),
        ] else if (_type == 'local') ...[
          const SizedBox(height: 12),
          Text(
            '直接读写 PTPatronus 服务所在机器的本地目录，无需开网络服务。根路径填本机绝对路径（如 Windows D:\\Media 或 Linux /mnt/nas）。后端进程拥有该目录完整权限，请仅配置可信目录。',
            style: TextStyle(fontSize: 11, color: context.dimText, height: 1.5),
          ),
        ] else if (_type == 'rclone') ...[
          const SizedBox(height: 12),
          TextField(
            controller: _url,
            decoration: const InputDecoration(
              labelText: 'rclone 二进制路径',
              hintText: '留空=用 PATH 中的 rclone',
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _username,
            decoration: const InputDecoration(
              labelText: 'Remote 名',
              hintText: 'rclone.conf 中的 [section]，如 myremote',
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _password,
            minLines: 6,
            maxLines: 12,
            decoration: InputDecoration(
              labelText: 'rclone.conf 内容',
              hintText: _isEdit ? '留空不修改' : '粘贴 rclone.conf INI 全文',
              alignLabelWithHint: true,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            '通过 rclone CLI 访问远端（非挂载）。后端把 conf 临时写盘 + --config 调用；操作需服务端机器装有 rclone。根路径填 remote 内的子路径。',
            style: TextStyle(fontSize: 11, color: context.dimText, height: 1.5),
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
                      leading: const Icon(Icons.folder, color: PTColors.primary2),
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
