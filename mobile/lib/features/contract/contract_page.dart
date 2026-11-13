// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/contract.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'contract_form.dart';

part 'contractopsmixin.dart';
part 'contract_widgets.dart';

/// 保种契约页（对齐 Web ContractView）：每站一条契约（分享率/做种/上传下限），
/// 卡片列表 + 新建/编辑表单 + 手动检查 + 删除。复用后端 /contract/rules。
class ContractPage extends StatefulWidget {
  const ContractPage({super.key});
  @override
  State<ContractPage> createState() => _ContractPageState();
}

class _ContractPageState extends State<ContractPage> with ContractOpsMixin {

  @override
  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  @override
  String _siteName(int id) {
    for (final s in _sites) {
      if (s.id == id) return s.name;
    }
    return '站点 #$id';
  }
  @override
  String _statusLabel(String s) {
    switch (s) {
      case 'ok':
        return '达标';
      case 'breach':
        return '未达标';
      case 'error':
        return '出错';
      default:
        return '未检查';
    }
  }
  @override
  void initState() {
    super.initState();
    _load();
    _ensureSites();
  }

  @override
  Future<void> _showCheckResult(ContractCheckResult res) {
    return showAdaptiveSheet(
      context,
      title: '检查结果 · ${res.siteName}',
      builder: (_) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: _statusBadge(res.status),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              _statCell('分享率', res.ratio.toStringAsFixed(2)),
              _statCell('做种数', '${res.seeding}'),
              _statCell('上传', '${res.uploadGb.toStringAsFixed(1)} GiB'),
            ],
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              if (res.seedingOfficial > 0)
                _statCell('官种', '${res.seedingOfficial}')
              else
                const Spacer(),
              _statCell('体积', '${res.seedingSizeGb.toStringAsFixed(1)} GiB'),
              if (res.seedingOfficial == 0) const Spacer(),
            ],
          ),
          if (res.unsupported.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              '不支持：${res.unsupported}',
              style: const TextStyle(fontSize: 12, color: PTColors.warn),
            ),
          ],
          if (res.detail.isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(res.detail, style: const TextStyle(fontSize: 13)),
          ],
          if (res.breaches.isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(
              '未达标项',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w600,
                color: context.dimText,
              ),
            ),
            const SizedBox(height: 4),
            for (final b in res.breaches)
              Padding(
                padding: const EdgeInsets.only(top: 2, left: 4),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      '· ',
                      style: TextStyle(color: PTColors.warn, fontSize: 13),
                    ),
                    Expanded(
                      child: Text(b, style: const TextStyle(fontSize: 13)),
                    ),
                  ],
                ),
              ),
          ],
        ],
      ),
    );
  }

  Future<void> _delete(ContractRule r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除保种契约'),
        content: Text('确认删除「${_siteName(r.siteId)}」的保种契约？此操作不可撤销。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
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
      await _api.remove(r.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _openCreate() async {
    await _ensureSites();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '新建保种契约',
      builder: (_) => _ContractForm(
        sites: _sites,
        onSubmit: (body) async {
          try {
            await _api.upsert(body);
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

  Future<void> _openEdit(ContractRule r) async {
    await _ensureSites();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '编辑保种契约',
      builder: (_) => _ContractForm(
        sites: _sites,
        initial: r,
        onSubmit: (body) async {
          try {
            await _api.upsert(body);
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

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '保种契约',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _load,
        ),
        PTBarAction(
          icon: Icons.add,
          label: '新建',
          onPressed: _openCreate,
        ),
      ],
      body: Column(
        children: [
          // 刷新已有列表时顶部细条；首次加载在 _content 居中 spinner。
          if (_loading && _list.isNotEmpty)
            const LinearProgressIndicator(minHeight: 2),
                  // 下拉刷新（与全仓 30 处 RefreshIndicator 同一心智；加载中不可再拉）。
        Expanded(
          child: RefreshIndicator(
            onRefresh: _load,
            child: _content(),
          ),
        ),
        ],
      ),
    );
  }

}
