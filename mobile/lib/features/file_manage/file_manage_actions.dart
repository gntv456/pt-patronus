// _FileManagePageState 的文件操作 mixin：下载/上传/建目录/重命名/移动复制/STRM。
// （含 setState 段；从 file_manage_page.dart 拆出，行为不变。）
part of 'package:ptpatronus/features/file_manage/file_manage_page.dart';

mixin FileManageActionsMixin on FileManageStorageMixin {
  // 宿主 _FileManagePageState 提供的钩子。
  @override
  void _toast(String m);
  @override
  Future<void> _loadEntries();
  Future<String?> _prompt(String label, String hint, {String initial = ''});
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
}
