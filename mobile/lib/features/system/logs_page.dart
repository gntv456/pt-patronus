// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/system_log.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:ptpatronus/core/err_text.dart';

/// 运行日志页（对齐 Web Settings 日志 tab）：展示后端环形缓冲日志，
/// 支持级别过滤、刷新与清空。后端 API：GET/DELETE /system/logs。
class LogsPage extends StatefulWidget {
  const LogsPage({super.key});
  @override
  State<LogsPage> createState() => _LogsPageState();
}

class _LogsPageState extends State<LogsPage> {
  final _api = SystemLogApi();
  List<LogEntry> _entries = [];
  bool _loading = true;
  String? _error;
  String _level = ''; // ''=全部

  @override
  void initState() {
    super.initState();
    // GET/DELETE /system/logs 是 admin-only：非管理员不发必然 403 的请求。
    if (ApiClient().isAdmin) _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _entries = await _api.list(level: _level);
    } catch (e) {
      _error = errTextOf(e) ?? '加载失败，请检查网络';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _clear() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('清空日志'),
        content: const Text('确认清空后端环形缓冲中的全部日志？此操作不可撤销。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('清空'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.clear();
      if (mounted) setState(() => _entries = []);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('清空失败：$e')));
      }
    }
  }

  Future<void> _copyAll() async {
    if (_entries.isEmpty) return;
    final text = _entries
        .map((e) => '${e.time.toIso8601String()} [${e.level}] ${e.message}')
        .join('\n');
    await Clipboard.setData(ClipboardData(text: text));
    if (mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('已复制 ${_entries.length} 条日志')));
    }
  }

  Color _levelColor(String lv) => switch (lv) {
    'ERROR' => const Color(0xFFF56C6C),
    'WARN' => const Color(0xFFE6A23C),
    'DEBUG' => context.dimText,
    _ => const Color(0xFF6C5CE7),
  };

  String _fmtTime(DateTime t) {
    String two(int n) => n.toString().padLeft(2, '0');
    return '${two(t.hour)}:${two(t.minute)}:${two(t.second)}';
  }

  @override
  Widget build(BuildContext context) {
    // 整页拦截（后端仍是权威鉴权，这里只挡 UI 入口；对齐系统设置页的既有惯例）。
    if (!ApiClient().isAdmin) {
      return const PTPageScaffold(
        title: '运行日志',
        body: PTEmptyState(
          icon: Icons.admin_panel_settings_outlined,
          text: '仅管理员可访问',
        ),
      );
    }
    return PTPageScaffold(
      title: '运行日志${_entries.isNotEmpty ? "（${_entries.length} 条）" : ""}',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _load,
        ),
        PTBarAction(
          icon: Icons.copy,
          label: '复制全部',
          onPressed: _copyAll,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.delete_sweep,
          label: '清空日志',
          onPressed: _clear,
          overflow: true,
        ),
      ],
      filterDimensions: [
        PTFilterDimension.choice(
          label: '级别',
          selected: _level,
          isDefault: _level.isEmpty,
          onChanged: (v) {
            setState(() => _level = (v as String?) ?? '');
            _load();
          },
          options: const [
            PTFilterOption<Object?>(value: '', label: '全部'),
            PTFilterOption<Object?>(value: 'error', label: 'ERROR'),
            PTFilterOption<Object?>(value: 'warn', label: 'WARN'),
            PTFilterOption<Object?>(value: 'info', label: 'INFO'),
            PTFilterOption<Object?>(value: 'debug', label: 'DEBUG'),
          ],
        ),
      ],
      body: RefreshIndicator(
        onRefresh: _load,
        child: _loading
            ? const PTLoadingView()
            : _error != null
            ? PTEmptyState(text: _error!, onRetry: _load)
            : _entries.isEmpty
            ? PTEmptyState(icon: Icons.article_outlined, text: '暂无日志')
            : ListView.builder(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 4,
                ),
                itemCount: _entries.length,
                itemBuilder: (_, i) => PTListItemEntrance(index: i, child: _row(_entries[i])),
              ),
      ),
    );
  }

  Widget _row(LogEntry e) {
    final hasAttrs = e.attrs.isNotEmpty;
    final lc = _levelColor(e.level);
    return GlassCard(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                _fmtTime(e.time),
                style: TextStyle(color: context.dimText, fontSize: 12),
              ),
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
                decoration: BoxDecoration(
                  color: lc.withValues(alpha: 0.16),
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  e.level,
                  style: TextStyle(
                    color: lc,
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  e.message,
                  style: const TextStyle(fontSize: 13, height: 1.4),
                ),
              ),
            ],
          ),
          if (hasAttrs)
            Padding(
              padding: const EdgeInsets.only(top: 4, left: 60),
              child: Wrap(
                spacing: 8,
                runSpacing: 2,
                children: e.attrs
                    .map(
                      (a) => Text(
                        '${a.key}=${a.value}',
                        style: TextStyle(
                          color: context.dimText,
                          fontSize: 11,
                        ),
                      ),
                    )
                    .toList(),
              ),
            ),
        ],
      ),
    );
  }
}
