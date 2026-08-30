// _FileManagePageState 的存储后端 mixin：全部状态字段 + 后端加载/切换/增删/连通性探测。
// mixin on State 可用 setState；从 file_manage_page.dart 拆出，行为不变。
part of 'package:ptpatronus/features/file_manage/file_manage_page.dart';

mixin FileManageStorageMixin on State<FileManagePage> {
  // 宿主 _FileManagePageState 提供的钩子（列表加载/toast），mixin 内调用。
  Future<void> _loadEntries();
  void _toast(String m);
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

  /// 统一连通性探测：fs 用 FileStorageApi.test（{ok,error}）；cloud 用 CloudApi.test。
  Future<bool> _testOk(_Backend b) async {
    if (b.kind == 'fs') return (await _api.test(b.id)).ok;
    final r = await _cloud.test(b.id);
    return r['ok'] == true;
  }
}
