// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// ReseedPage 的 ReseedOpsMixin mixin（含 setState 段；从 reseed_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/reseed/reseed_page.dart';

mixin ReseedOpsMixin on State<ReseedPage> {
  // 宿主提供的钩子。
  void _toast(String msg);

  final _api = ReseedApi();
  final _dlApi = DownloaderApi();
  final _tokenCtrl = TextEditingController();
  final _savePathCtrl = TextEditingController();

  bool _configured = false;
  String _masked = '';
  bool _savingToken = false;

  List<Downloader> _downloaders = [];
  int? _selectedDl;
  bool _reseeding = false;

  ReseedSetting? _setting;
  bool _loadingSetting = false;

  List<ReseedTask> _tasks = [];
  bool _loading = true;
  String _status = 'pending';
  final _resolving = <int>{};

  Future<void> _loadConfig() async {
    try {
      final c = await _api.getConfig();
      if (!mounted) return;
      setState(() {
        _configured = c['configured'] == true;
        _masked = (c['iyuu_token_masked'] as String?) ?? '';
      });
    } catch (_) {}
  }

  Future<void> _saveToken() async {
    final t = _tokenCtrl.text.trim();
    if (t.isEmpty) {
      _toast('请输入 IYUU token');
      return;
    }
    setState(() => _savingToken = true);
    try {
      await _api.setToken(t);
      _tokenCtrl.clear();
      _toast('IYUU token 已保存');
      await _loadConfig();
    } catch (_) {
      _toast('保存失败');
    } finally {
      if (mounted) setState(() => _savingToken = false);
    }
  }

  Future<void> _loadDownloaders() async {
    try {
      final ls = await _dlApi.list();
      if (!mounted) return;
      setState(() {
        _downloaders = ls;
        _selectedDl ??= ls.isNotEmpty ? ls.first.id : null;
      });
      if (_selectedDl != null) _loadSetting();
    } catch (_) {}
  }

  Future<void> _loadSetting() async {
    if (_selectedDl == null) return;
    setState(() => _loadingSetting = true);
    try {
      final s = await _api.setting(_selectedDl!);
      if (!mounted) return;
      setState(() {
        _setting = s;
        _savePathCtrl.text = s.savePath;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _setting = ReseedSetting(downloaderId: _selectedDl!);
          _savePathCtrl.text = '';
        });
      }
    } finally {
      if (mounted) setState(() => _loadingSetting = false);
    }
  }

  Future<void> _toggleAuto({bool? autoReseed, bool? autoReseedAdd}) async {
    if (_selectedDl == null || _setting == null) return;
    final next = _setting!.copyWith(
      autoReseed: autoReseed,
      autoReseedAdd: autoReseedAdd,
    );
    setState(() => _setting = next);
    try {
      await _api.saveSetting(next);
    } catch (_) {
      if (mounted) _toast('设置保存失败');
      await _loadSetting();
    }
  }

  // 保存保存路径 + 暂停添加（这两个字段不在开关即时保存，单独按钮提交）。
  Future<void> _savePathAndPause() async {
    if (_selectedDl == null || _setting == null) return;
    final next = _setting!.copyWith(
      savePath: _savePathCtrl.text.trim(),
      pauseAfterAdd: _setting!.pauseAfterAdd,
    );
    setState(() => _setting = next);
    try {
      await _api.saveSetting(next);
      _toast('已保存');
    } catch (_) {
      _toast('保存失败');
    }
  }

  Future<void> _togglePause(bool v) async {
    if (_selectedDl == null || _setting == null) return;
    final next = _setting!.copyWith(
      pauseAfterAdd: v,
      savePath: _savePathCtrl.text.trim(),
    );
    setState(() => _setting = next);
    try {
      await _api.saveSetting(next);
    } catch (_) {
      if (mounted) _toast('保存失败');
      await _loadSetting();
    }
  }

  Future<void> _loadTasks() async {
    setState(() => _loading = true);
    try {
      final ts = await _api.tasks(_status);
      if (!mounted) return;
      setState(() => _tasks = ts);
    } catch (_) {
      if (mounted) _toast('任务加载失败');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _startReseed() async {
    if (_selectedDl == null) {
      _toast('请选择下载器');
      return;
    }
    if (!_configured) {
      _toast('请先保存 IYUU token');
      return;
    }
    setState(() => _reseeding = true);
    try {
      final created = await _api.reseed(_selectedDl!);
      _toast('辅种完成，新增 ${created.length} 个候选');
      if (!mounted) return;
      setState(() => _status = 'pending');
      await _loadTasks();
    } catch (e) {
      _toast('辅种失败：$e');
    } finally {
      if (mounted) setState(() => _reseeding = false);
    }
  }

  Future<void> _resolve(ReseedTask t, String action) async {
    // 添加辅种推送下载器（不可逆）→ 二次确认；跳过可逆不确认。
    if (action == 'add') {
      final ok = await showDialog<bool>(
        context: context,
        builder: (_) => AlertDialog(
          title: const Text('添加辅种'),
          content: const Text('将向下载器添加该辅种任务，确认？'),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('添加'),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }
    setState(() => _resolving.add(t.id));
    try {
      await _api.resolve(t.id, action);
      _toast(action == 'add' ? '已添加辅种' : '已跳过');
      await _loadTasks();
    } catch (e) {
      _toast('操作失败：$e');
    } finally {
      if (mounted) setState(() => _resolving.remove(t.id));
    }
  }
}
