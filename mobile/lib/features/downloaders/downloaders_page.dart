// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';

import 'package:ptpatronus/core/platform/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:ptpatronus/core/err_text.dart';

part 'sheets/add_torrent_sheet.dart';
part 'sheets/downloader_form_sheet.dart';
part 'sheets/torrent_detail_sheet.dart';
part 'sheets/torrent_sheet.dart';
part 'sheets/torrent_sheet_actions.dart';
part 'sheets/torrent_sheet_widgets.dart';
part 'sheets/files_sheet.dart';
part 'sheets/trackers_sheet.dart';
part 'sheets/peers_sheet.dart';
part 'sheets/category_sheet.dart';
part 'downloaders_utils.dart';

// 下载器能力位（与 Web caps.ts 1:1）：UI 按能力门控，新增类型在此加一条 case。
({bool category, bool categoryCRUD, bool forceStart, bool queueFull, bool sequential, bool firstLast, bool swarm, bool tags, bool trackers, bool files, bool peers, List<({String label, int value})> filePrio}) _dlCaps(String t) {
  switch (t) {
    case 'tr':
      return (category: false, categoryCRUD: false, forceStart: false, queueFull: false, sequential: false, firstLast: false, swarm: false, tags: true, trackers: true, files: true, peers: true, filePrio: const [(label: '低', value: -1), (label: '正常', value: 0), (label: '高', value: 1)]);
    case 'ut':
      return (category: true, categoryCRUD: false, forceStart: true, queueFull: true, sequential: false, firstLast: false, swarm: true, tags: false, trackers: false, files: true, peers: true, filePrio: const [(label: '跳过', value: 0), (label: '低', value: 1), (label: '正常', value: 2), (label: '高', value: 3)]);
    case 'dl':
      return (category: true, categoryCRUD: false, forceStart: false, queueFull: true, sequential: false, firstLast: false, swarm: true, tags: false, trackers: true, files: true, peers: true, filePrio: const [(label: '跳过', value: 0), (label: '低', value: 1), (label: '正常', value: 2), (label: '高', value: 3)]);
    case 'ar':
      return (category: false, categoryCRUD: false, forceStart: false, queueFull: true, sequential: false, firstLast: false, swarm: false, tags: false, trackers: false, files: true, peers: true, filePrio: const [(label: '跳过', value: 0), (label: '下载', value: 1)]);
    case 'rt':
      return (category: true, categoryCRUD: false, forceStart: false, queueFull: false, sequential: false, firstLast: false, swarm: false, tags: false, trackers: false, files: false, peers: false, filePrio: const []);
    case 'fl':
      return (category: true, categoryCRUD: false, forceStart: false, queueFull: false, sequential: false, firstLast: false, swarm: true, tags: true, trackers: false, files: true, peers: true, filePrio: const [(label: '跳过', value: 0), (label: '低', value: 1), (label: '正常', value: 2), (label: '高', value: 3)]);
    default: // qb
      return (category: true, categoryCRUD: true, forceStart: true, queueFull: true, sequential: true, firstLast: true, swarm: true, tags: true, trackers: true, files: true, peers: true, filePrio: const [(label: '不下载', value: 0), (label: '正常', value: 2), (label: '高', value: 1), (label: '最高', value: 7)]);
  }
}

class DownloadersPage extends StatefulWidget {
  const DownloadersPage({super.key});
  @override
  State<DownloadersPage> createState() => _DownloadersPageState();
}

class _DownloadersPageState extends State<DownloadersPage> {
  List<Downloader> _list = [];
  bool _loading = true; // 首访即显示加载态，避免短暂空白像页面坏了
  String? _error; // 加载失败时的友好文案；null=无错误

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _list = await DownloaderApi().list();
    } catch (e) {
      // 保留已加载的旧数据；首次加载（_list 空）时展示错误态 + 重试，而非空白
      if (_list.isEmpty) {
        _error = errTextOf(e) ?? '加载失败，请检查网络';
      } else {
        _toast(e.toString());
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _open(Downloader d) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.92,
        child: _TorrentSheet(downloader: d),
      ),
    );
  }

  // —— 下载器级操作：添加 / 编辑 / 测试 / 删除（任务级操作在 _TorrentSheet 里）——
  Future<void> _addDownloader() => _editDownloader(null);

  Future<void> _editDownloader(Downloader? d) async {
    // 宽屏=居中对话框（对齐 Web el-dialog 460），窄屏=底栏；浮层容器自带标题/键盘避让。
    final changed = await showAdaptiveSheet<bool>(
      context,
      title: d == null ? '添加下载器' : '编辑下载器',
      builder: (_) => _DownloaderFormSheet(downloader: d),
    );
    if (changed == true) _load();
  }

  Future<void> _testDownloader(Downloader d) async {
    try {
      final r = await DownloaderApi().test(d.id);
      final ok = r['ok'] == true;
      _toast(ok ? '连接成功' : '连接失败：${r['error'] ?? '未知错误'}');
    } catch (e) {
      _toast('测试失败：${e.toString().replaceFirst('Exception: ', '')}');
    }
  }

  Future<void> _deleteDownloader(Downloader d) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除下载器'),
        content: Text('确认删除下载器「${d.name}」？仅移除配置，不影响已下载文件。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: TextButton.styleFrom(foregroundColor: Colors.redAccent),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await DownloaderApi().remove(d.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：${e.toString().replaceFirst('Exception: ', '')}');
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

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
          onPressed: _load,
        ),
      ],
    ),
  );

  Widget _emptyView() =>
      PTEmptyState(icon: Icons.download_outlined, text: '暂无下载器');

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '下载器',
      actions: [
        PTBarAction(icon: Icons.refresh, label: '刷新', onPressed: _load),
        PTBarAction(icon: Icons.add, label: '添加下载器', onPressed: _addDownloader),
      ],
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_loading) const LinearProgressIndicator(minHeight: 2),
          Expanded(
            child: (_list.isEmpty && !_loading)
                ? (_error != null ? _errorView() : _emptyView())
                : RefreshIndicator(
                    onRefresh: _load,
                    child: PTMasonry<Downloader>(
                      items: _list,
                      minItemWidth: 280,
                      padding: const EdgeInsets.all(12),
                      itemBuilder: (_, d) => _downloaderCard(d),
                    ),
                  ),
          ),
        ],
      ),
    );
  }

  // 下载器卡片：对齐 Web Downloaders.vue —— 渐变图标 + 名称/类型 + 地址 + 2×2 操作按钮网格。
  // 删除按钮仅 admin 可见（后端 RequireAdmin 守卫，UI 与 Web auth.isAdmin 对齐）。
  Widget _downloaderCard(Downloader d) {
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              _dlBadge(d.type),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      d.name,
                      style: const TextStyle(
                        fontWeight: FontWeight.w600,
                        fontSize: 16,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                    Text(
                      _typeLabel(d.type),
                      style: TextStyle(color: context.dimText, fontSize: 12),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Text(
            d.url,
            style: TextStyle(color: context.dimText, fontSize: 12),
            softWrap: true,
          ),
          const SizedBox(height: 14),
          _actionRow([
            OutlinedButton(
              onPressed: () => _testDownloader(d),
              child: const Text('连接测试'),
            ),
            FilledButton(onPressed: () => _open(d), child: const Text('任务列表')),
          ]),
          const SizedBox(height: 8),
          _actionRow([
            OutlinedButton(
              onPressed: () => _editDownloader(d),
              child: const Text('编辑'),
            ),
            if (ApiClient().isAdmin)
              OutlinedButton(
                style: OutlinedButton.styleFrom(
                  foregroundColor: Colors.redAccent,
                  side: const BorderSide(color: Colors.redAccent),
                ),
                onPressed: () => _deleteDownloader(d),
                child: const Text('删除'),
              ),
          ]),
        ],
      ),
    );
  }

  // 两列等宽按钮行（对齐 Web .actions 的 grid-template-columns: repeat(2, 1fr)）。
  Widget _actionRow(List<Widget> buttons) {
    return Row(
      children: [
        for (int i = 0; i < buttons.length; i++) ...[
          if (i > 0) const SizedBox(width: 8),
          Expanded(child: buttons[i]),
        ],
      ],
    );
  }

  // 类型图标徽章：GlassIcon 渲染品牌字形（qb 蓝 / tr 红，与 Web 卡片完全一致）。
  Widget _dlBadge(String t) {
    final m = _dlMeta(t);
    return GlassIcon(glyph: m.glyph, from: m.from, to: m.to, size: 40);
  }

  String _typeLabel(String t) => _dlMeta(t).label;
}
