// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// CloudPage 的 CloudOpsMixin mixin（含 setState 段；从 cloud_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/cloud/cloud_page.dart';

mixin CloudOpsMixin on State<CloudPage> {

  final _api = CloudApi();

  List<CloudDrive> _drives = [];
  List<StrmRule> _rules = [];
  List<StrmRecord> _records = [];
  bool _loading = false;
  String? _error;
  final _testing = <int>{};
  final _quotas = <int, String>{};

  // 宿主 _CloudPageState 提供的钩子。
  void _toast(String msg);
  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([_api.listDrives(), _api.listRules(), _api.listRecords()]);
      if (!mounted) return;
      setState(() {
        _drives = results[0] as List<CloudDrive>;
        _rules = results[1] as List<StrmRule>;
        _records = results[2] as List<StrmRecord>;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  String _driveName(int id) => _drives.any((d) => d.id == id)
      ? _drives.firstWhere((d) => d.id == id).name
      : '#$id';

  // ---- 网盘 ----
  Future<void> _test(CloudDrive d) async {
    setState(() => _testing.add(d.id));
    try {
      final r = await _api.test(d.id);
      final ok = r['ok'] == true;
      _toast(ok ? '${d.name}：${r['info'] ?? '连接正常'}' : '连接失败：${r['error'] ?? '未知错误'}');
    } catch (e) {
      _toast('测试失败：$e');
    } finally {
      if (mounted) setState(() => _testing.remove(d.id));
    }
  }

  Future<void> _quota(CloudDrive d) async {
    try {
      final q = await _api.quota(d.id);
      final used = (q?['used'] as num?)?.toInt() ?? 0;
      final total = (q?['total'] as num?)?.toInt() ?? 0;
      if (!mounted) return;
      setState(() => _quotas[d.id] = '${_fmtSize(used)} / ${_fmtSize(total)}');
    } catch (e) {
      _toast('获取容量失败：$e');
    }
  }
}
