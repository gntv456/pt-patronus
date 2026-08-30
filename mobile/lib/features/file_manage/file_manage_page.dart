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

part 'file_manage_actions.dart';
part 'file_manage_storage.dart';
part 'file_manage_widgets.dart';
part 'move_copy_picker.dart';
part 'storage_form_sheet.dart';
part 'strm_sheet.dart';

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

class _FileManagePageState extends State<FileManagePage>
    with FileManageStorageMixin, FileManageActionsMixin {

  @override
  Widget build(BuildContext context) => _buildScaffold(context);
  @override
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }
  @override
  void initState() {
    super.initState();
    _loadBackends();
  }

  @override
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

  @override
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
