// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/cloud.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'cloud_browse_body.dart';
part 'drive_form.dart';
part 'cloud_rule_form.dart';

part 'cloudopsmixin.dart';
part 'cloud_widgets.dart';

/// 云盘 · STRM 页（对齐 Web Cloud.vue）：115/夸克/123 网盘 CRUD + 浏览 + STRM 规则 + 生成 + 订阅追更。
/// 三 Tab：网盘 / STRM 规则 / STRM 记录。复用后端 /cloud/*。
class CloudPage extends StatefulWidget {
  const CloudPage({super.key});
  @override
  State<CloudPage> createState() => _CloudPageState();
}

class _CloudPageState extends State<CloudPage>
    with CloudOpsMixin, SingleTickerProviderStateMixin {
  late final TabController _tab = TabController(length: 3, vsync: this);


  @override
  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _tab.dispose();
    super.dispose();
  }

  Future<void> _deleteDrive(CloudDrive d) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除网盘'),
        content: Text('确认删除「${d.name}」？其下 STRM 规则与记录将一并清除。'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: PTColors.danger),
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.removeDrive(d.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _openCreateDrive() async {
    await showAdaptiveSheet(
      context,
      title: '添加网盘',
      builder: (_) => _DriveForm(
        onSubmit: (body) async {
          try {
            await _api.createDrive(body);
            _toast('已创建');
            await _load();
            return true;
          } catch (e) {
            _toast('创建失败：$e');
            return false;
          }
        },
      ),
    );
  }

  Future<void> _openEditDrive(CloudDrive d) async {
    await showAdaptiveSheet(
      context,
      title: '编辑网盘',
      builder: (_) => _DriveForm(
        initial: d,
        onSubmit: (body) async {
          try {
            await _api.updateDrive(d.id, body);
            _toast('已保存');
            await _load();
            return true;
          } catch (e) {
            _toast('保存失败：$e');
            return false;
          }
        },
      ),
    );
  }

  Future<void> _browse(CloudDrive d) async {
    await showAdaptiveSheet(
      context,
      title: '浏览 · ${d.name}',
      builder: (_) => _BrowseBody(drive: d, api: _api),
    );
  }

  // ---- 规则 ----
  Future<void> _generate(StrmRule r) async {
    _toast('正在生成…');
    try {
      final res = await _api.generateRule(r.id);
      final gen = res['generated'] ?? 0;
      final skip = res['skipped'] ?? 0;
      final errs = (res['errors'] as List?) ?? const [];
      _toast('${r.name}：新增 $gen，跳过 $skip${errs.isNotEmpty ? '，失败 ${errs.length}' : ''}');
      await _load();
    } catch (e) {
      _toast('生成失败：$e');
    }
  }

  Future<void> _generateAll() async {
    try {
      await _api.generateAll();
      _toast('已触发全部规则生成');
      await Future.delayed(const Duration(seconds: 1));
      await _load();
    } catch (e) {
      _toast('请求失败：$e');
    }
  }

  Future<void> _syncSubs() async {
    try {
      await _api.syncSubscriptions();
      _toast('订阅追更匹配完成');
      await _load();
    } catch (e) {
      _toast('请求失败：$e');
    }
  }

  Future<void> _deleteRule(StrmRule r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除规则'),
        content: Text('确认删除规则「${r.name}」？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: PTColors.danger),
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.removeRule(r.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _openCreateRule() async {
    if (_drives.isEmpty) {
      _toast('请先添加网盘');
      return;
    }
    await showAdaptiveSheet(
      context,
      title: '添加 STRM 规则',
      builder: (_) => _RuleForm(
        drives: _drives,
        onSubmit: (body) async {
          try {
            await _api.createRule(body);
            _toast('已创建');
            await _load();
            return true;
          } catch (e) {
            _toast('创建失败：$e');
            return false;
          }
        },
        onBrowse: (d) => _pickDir(d),
      ),
    );
  }

  Future<void> _openEditRule(StrmRule r) async {
    await showAdaptiveSheet(
      context,
      title: '编辑规则',
      builder: (_) => _RuleForm(
        drives: _drives,
        initial: r,
        onSubmit: (body) async {
          try {
            await _api.updateRule(r.id, body);
            _toast('已保存');
            await _load();
            return true;
          } catch (e) {
            _toast('保存失败：$e');
            return false;
          }
        },
        onBrowse: (d) => _pickDir(d),
      ),
    );
  }

  Future<String?> _pickDir(CloudDrive d) async {
    String? picked;
    await showAdaptiveSheet(
      context,
      title: '选择目录 · ${d.name}',
      builder: (_) => _BrowseBody(
        drive: d,
        api: _api,
        onPick: (p) {
          picked = p;
          Navigator.of(context).pop();
        },
      ),
    );
    return picked;
  }

  Future<void> _deleteRecord(StrmRecord r) async {
    try {
      await _api.removeRecord(r.id);
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  void _onAdd() {
    if (_tab.index == 0) {
      _openCreateDrive();
    } else if (_tab.index == 1) {
      _openCreateRule();
    } else {
      _toast('记录页无需添加');
    }
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '云盘 · STRM',
      actions: [
        PTBarAction(icon: Icons.refresh, label: '刷新', loading: _loading, onPressed: _loading ? null : _load),
        PTBarAction(icon: Icons.add, label: '添加', onPressed: _onAdd),
      ],
      body: Column(
        children: [
          if (_loading && _drives.isNotEmpty) const LinearProgressIndicator(minHeight: 2),
          Material(
            color: Colors.transparent,
            child: TabBar(
              controller: _tab,
              tabs: const [
                Tab(text: '网盘'),
                Tab(text: 'STRM 规则'),
                Tab(text: '记录'),
              ],
            ),
          ),
          Expanded(
            child: _error != null && _drives.isEmpty
                ? PTEmptyState(icon: Icons.error_outline, text: '加载失败', description: _error, onRetry: _load)
                : TabBarView(
                    controller: _tab,
                    children: [_drivesTab(), _rulesTab(), _recordsTab()],
                  ),
          ),
        ],
      ),
    );
  }

}

String _fmtSize(int n) {
  if (n <= 0) return '-';
  const g = 1 << 30;
  if (n >= g) return '${(n / g).toStringAsFixed(1)} GB';
  if (n >= 1 << 20) return '${(n / (1 << 20)).toStringAsFixed(1)} MB';
  return '${(n / (1 << 10)).toStringAsFixed(1)} KB';
}
