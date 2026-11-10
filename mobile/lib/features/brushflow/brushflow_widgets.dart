// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// BrushFlowPage 的展示层扩展（无 setState 段；从 brushflow_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/brushflow/brushflow_page.dart';

extension on _BrushFlowPageState {
  String _siteName(int id) {
    for (final s in _sites) {
      if (s.id == id) return s.name;
    }
    return '站点 #$id';
  }

  String _dlName(int id) {
    for (final d in _downloaders) {
      if (d.id == id) return d.name;
    }
    return '下载器 #$id';
  }

  String _fmtBytes(int n) {
    if (n <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    double s = n.toDouble();
    int i = 0;
    while (s >= 1024 && i < units.length - 1) {
      s /= 1024;
      i++;
    }
    return i <= 1 ? '${s.toInt()} ${units[i]}' : '${s.toStringAsFixed(1)} ${units[i]}';
  }

  String _fmtDate(DateTime? d) {
    if (d == null) return '-';
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
  }

  Widget _content() {
    if (_loading && _list.isEmpty) {
      return const PTLoadingView();
    }
    if (_error != null && _list.isEmpty) {
      return PTEmptyState(
        icon: Icons.error_outline,
        text: '加载失败',
        description: _error,
        onRetry: _load,
      );
    }
    if (_list.isEmpty) {
      return const PTEmptyState(
        icon: Icons.bolt_outlined,
        text: '暂无刷流任务',
        description: '点右上「新建」创建第一个刷流任务',
      );
    }
    return PTMasonry<BrushFlowTask>(
      items: _list,
      minItemWidth: 300,
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      itemBuilder: (_, t) => _taskCard(t),
    );
  }

  Widget _taskCard(BrushFlowTask t) {
    final busy = _busy.contains(t.id);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 头部：图标 + 名称 + 站点→下载器 meta + 启用开关。
          Row(
            children: [
              const GlassIcon(
                glyph: 'brushflow',
                from: Color(0xFFF7971E),
                to: Color(0xFFFF6A00),
                size: 32,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      t.name,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '${_siteName(t.siteId)} → ${_dlName(t.downloaderId)}',
                      style: TextStyle(color: context.dimText, fontSize: 12),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
              Switch(value: t.enabled, onChanged: (v) => _toggleEnabled(t, v)),
            ],
          ),
          const SizedBox(height: 10),
          // 统计三格：加入 / 删除 / 上传。
          Row(
            children: [
              _statCell('加入', '${t.totalAdded}'),
              _statCell('删除', '${t.totalDeleted}'),
              _statCell('上传', _fmtBytes(t.totalUpload)),
            ],
          ),
          if (t.lastError.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              t.lastError,
              style: const TextStyle(color: PTColors.danger, fontSize: 12),
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
            ),
          ],
          const SizedBox(height: 10),
          // 操作按钮行：选种 / 扫删 / 历史 / 编辑 / 删除。
          Wrap(
            spacing: 6,
            runSpacing: 4,
            children: [
              FilledButton.tonal(
                onPressed: busy ? null : () => _select(t),
                child: const Text('选种', style: TextStyle(fontSize: 13)),
              ),
              FilledButton.tonal(
                onPressed: busy ? null : () => _sweep(t),
                child: const Text('扫删', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _showSeeds(t),
                child: const Text('历史', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _openEdit(t),
                child: const Text('编辑', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _delete(t),
                child: const Text(
                  '删除',
                  style: TextStyle(fontSize: 13, color: PTColors.danger),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _statCell(String label, String value) {
    return Expanded(
      child: Column(
        children: [
          Text(
            value,
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 16,
              color: PTColors.primary,
            ),
          ),
          const SizedBox(height: 2),
          Text(label, style: TextStyle(color: context.dimText, fontSize: 12)),
        ],
      ),
    );
  }

}
