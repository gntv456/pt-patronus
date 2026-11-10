// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// CloudPage 的展示层扩展（无 setState 段；从 cloud_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/cloud/cloud_page.dart';

extension on _CloudPageState {
  // ---- Tabs ----
  Widget _drivesTab() {
    if (_drives.isEmpty) {
      return const PTEmptyState(
        icon: Icons.cloud_outlined,
        text: '尚未添加网盘',
        description: '点右上「添加」接入 115 / 夸克 / 123',
      );
    }
    return RefreshIndicator(
      onRefresh: _load,
      child: PTMasonry<CloudDrive>(
        items: _drives,
        minItemWidth: 300,
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
        itemBuilder: (_, d) => _driveCard(d),
      ),
    );
  }

  Widget _driveCard(CloudDrive d) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            const GlassIcon(glyph: 'cloud', from: Color(0xFF4CC9F0), to: Color(0xFF4361EE), size: 32),
            const SizedBox(width: 10),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Row(children: [
                  Flexible(
                    child: Text(d.name, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15),
                        maxLines: 1, overflow: TextOverflow.ellipsis),
                  ),
                  const SizedBox(width: 6),
                  _tag(cloudTypeLabel[d.type] ?? d.type),
                  if (d.isDefault) _tag('默认', accent: true),
                ]),
                const SizedBox(height: 2),
                Text('根目录：${d.rootPath}', style: TextStyle(color: context.dimText, fontSize: 12), maxLines: 1, overflow: TextOverflow.ellipsis),
                if (_quotas[d.id] != null)
                  Text('容量：${_quotas[d.id]}', style: TextStyle(color: context.dimText, fontSize: 12)),
              ]),
            ),
          ]),
          const SizedBox(height: 10),
          Wrap(spacing: 8, runSpacing: 8, children: [
            OutlinedButton.icon(
              icon: _testing.contains(d.id)
                  ? const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.cable, size: 16),
              label: const Text('测试'),
              onPressed: _testing.contains(d.id) ? null : () => _test(d),
            ),
            OutlinedButton.icon(
              icon: const Icon(Icons.storage_outlined, size: 16),
              label: const Text('容量'),
              onPressed: () => _quota(d),
            ),
            OutlinedButton.icon(
              icon: const Icon(Icons.folder_open_outlined, size: 16),
              label: const Text('浏览'),
              onPressed: () => _browse(d),
            ),
            TextButton.icon(
              icon: const Icon(Icons.edit_outlined, size: 16),
              label: const Text('编辑'),
              onPressed: () => _openEditDrive(d),
            ),
            TextButton.icon(
              icon: Icon(Icons.delete_outline, size: 16, color: PTColors.danger),
              label: Text('删除', style: TextStyle(color: PTColors.danger)),
              onPressed: () => _deleteDrive(d),
            ),
          ]),
        ],
      ),
    );
  }

  Widget _rulesTab() {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
          child: Row(children: [
            TextButton.icon(icon: const Icon(Icons.bolt_outlined, size: 16), label: const Text('生成全部'), onPressed: _generateAll),
            TextButton.icon(icon: const Icon(Icons.sync_outlined, size: 16), label: const Text('订阅追更'), onPressed: _syncSubs),
          ]),
        ),
        if (_rules.isEmpty)
          const Expanded(child: PTEmptyState(icon: Icons.rule_folder_outlined, text: '尚未配置 STRM 规则', description: '点右上「添加」创建规则'))
        else
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.fromLTRB(12, 4, 12, 16),
              itemCount: _rules.length,
              itemBuilder: (_, i) => _ruleCard(_rules[i]),
            ),
          ),
      ],
    );
  }

  Widget _ruleCard(StrmRule r) {
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Expanded(child: Text(r.name, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14))),
            _tag(r.enabled ? '启用' : '停用', accent: r.enabled),
          ]),
          const SizedBox(height: 4),
          Text('网盘：${_driveName(r.driveId)} · ${r.cloudPath}', style: TextStyle(fontSize: 12, color: context.dimText), maxLines: 1, overflow: TextOverflow.ellipsis),
          Text('输出：${r.outputDir}', style: TextStyle(fontSize: 12, color: context.dimText), maxLines: 1, overflow: TextOverflow.ellipsis),
          if (r.mountRoot.isNotEmpty)
            Text('挂载：${r.mountRoot}', style: TextStyle(fontSize: 12, color: context.dimText), maxLines: 1, overflow: TextOverflow.ellipsis),
          const SizedBox(height: 8),
          Row(children: [
            TextButton.icon(icon: const Icon(Icons.play_arrow_outlined, size: 16), label: const Text('生成'), onPressed: () => _generate(r)),
            TextButton.icon(icon: const Icon(Icons.edit_outlined, size: 16), label: const Text('编辑'), onPressed: () => _openEditRule(r)),
            TextButton.icon(icon: Icon(Icons.delete_outline, size: 16, color: PTColors.danger), label: Text('删除', style: TextStyle(color: PTColors.danger)), onPressed: () => _deleteRule(r)),
          ]),
        ]),
      ),
    );
  }

  Widget _recordsTab() {
    if (_records.isEmpty) {
      return const PTEmptyState(icon: Icons.history_outlined, text: '暂无 STRM 记录', description: '生成 STRM 后此处显示已生成文件');
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      itemCount: _records.length,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (_, i) {
        final r = _records[i];
        return ListTile(
          leading: Icon(r.subscriptionId > 0 ? Icons.favorite : Icons.insert_drive_file_outlined, size: 22, color: context.dimText),
          title: Text(r.cloudPath, style: const TextStyle(fontSize: 13), maxLines: 1, overflow: TextOverflow.ellipsis),
          subtitle: Text('${_driveName(r.driveId)} · ${_fmtSize(r.size)}${r.subscriptionId > 0 ? ' · 追更' : ''}', style: TextStyle(fontSize: 11, color: context.dimText)),
          trailing: IconButton(
            tooltip: '删除该 STRM 记录',
            icon: Icon(Icons.delete_outline, size: 18, color: PTColors.danger),
            onPressed: () => _deleteRecord(r),
          ),
        );
      },
    );
  }

  Widget _tag(String text, {bool accent = false}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: accent ? PTColors.success.withValues(alpha: 0.18) : context.dimText.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(text, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }

}
