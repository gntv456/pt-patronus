// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/brushflow.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/filter_rules.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/features/common/capped_list_notice.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'brushflow_form.dart';
part 'brushflow_form_widgets.dart';

part 'brushflowopsmixin.dart';
part 'brushflow_widgets.dart';

/// 刷流引擎页（对齐 Web BrushFlow.vue）：任务卡片列表 + 新建/编辑表单 +
/// 选种 / 扫删 / 历史种子 / 删除。复用后端 /brushflow/*。
class BrushFlowPage extends StatefulWidget {
  const BrushFlowPage({super.key});
  @override
  State<BrushFlowPage> createState() => _BrushFlowPageState();
}

class _BrushFlowPageState extends State<BrushFlowPage> with BrushFlowOpsMixin {

  @override
  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }
  @override
  void initState() {
    super.initState();
    _load();
    _ensureRefs();
  }

  Future<void> _delete(BrushFlowTask t) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除刷流任务'),
        content: Text('确认删除任务「${t.name}」？此操作不可撤销。'),
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
      await _api.remove(t.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _showSeeds(BrushFlowTask t) async {
    List<BrushFlowSeed> seeds = [];
    String notice = '';
    String? err;
    try {
      final page = await _api.seedsPage(t.id);
      seeds = page.items;
      notice = cappedListNotice(
        shown: page.items.length,
        total: page.total,
        truncated: page.truncated,
        unknownTotal: (n) => '已显示最近 $n 条种子记录，但总数读取失败——可能还有更多',
        knownTotal: (n, total) => '仅显示最近 $n 条种子记录（该任务共 $total 条，更早的未加载）',
      );
    } catch (e) {
      err = '$e';
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '种子历史 · ${t.name}',
      builder: (_) {
        if (err != null) {
          return Padding(
            padding: const EdgeInsets.all(16),
            child: Text(
              '加载失败：$err',
              style: const TextStyle(color: PTColors.danger, fontSize: 13),
            ),
          );
        }
        if (seeds.isEmpty) {
          return Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text('暂无种子记录', style: TextStyle(color: context.dimText)),
            ),
          );
        }
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            if (notice.isNotEmpty)
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
                child: Text(
                  notice,
                  style: TextStyle(fontSize: 12, color: context.dimText),
                ),
              ),
            for (final s in seeds)
              ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: Text(
                  s.title.isEmpty ? s.sourceKey : s.title,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 13),
                ),
                subtitle: Text(
                  '${_fmtBytes(s.size)}  ·  加入 ${_fmtDate(s.addedAt)}',
                  style: TextStyle(fontSize: 12, color: context.dimText),
                ),
              ),
          ],
        );
      },
    );
  }

  Future<void> _openCreate() async {
    await _ensureRefs();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '新建刷流任务',
      builder: (_) => _BrushFlowForm(
        sites: _sites,
        downloaders: _downloaders,
        filterGroups: _filterGroups,
        onSubmit: (body) async {
          try {
            await _api.create(body);
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

  Future<void> _openEdit(BrushFlowTask t) async {
    await _ensureRefs();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '编辑刷流任务',
      builder: (_) => _BrushFlowForm(
        sites: _sites,
        downloaders: _downloaders,
        filterGroups: _filterGroups,
        initial: t,
        onSubmit: (body) async {
          try {
            await _api.update(t.id, body);
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
      title: '刷流',
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
          if (_loading && _list.isNotEmpty) const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _content()),
        ],
      ),
    );
  }

}
