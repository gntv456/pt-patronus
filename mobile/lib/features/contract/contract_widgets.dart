// ContractPage 的展示层扩展（无 setState 段；从 contract_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/contract/contract_page.dart';

extension on _ContractPageState {

  String _fmtDate(DateTime? d) {
    if (d == null) return '-';
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
  }

  Color _statusColor(String s) {
    switch (s) {
      case 'ok':
        return PTColors.success;
      case 'breach':
        return PTColors.warn;
      case 'error':
        return PTColors.danger;
      default:
        return context.dimText;
    }
  }


  Widget _statusBadge(String s) {
    final c = _statusColor(s);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: c.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: c.withValues(alpha: 0.4)),
      ),
      child: Text(
        _statusLabel(s),
        style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: c),
      ),
    );
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
        icon: Icons.assignment_outlined,
        text: '暂无保种契约',
        description: '点右上「新建」为站点设置分享率/做种/上传下限',
      );
    }
    return PTMasonry<ContractRule>(
      items: _list,
      minItemWidth: 300,
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      itemBuilder: (_, r) => _ruleCard(r),
    );
  }

  Widget _ruleCard(ContractRule r) {
    final busy = _busy.contains(r.id);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 头部：图标 + 站点名 + 下限 meta + 启用开关。
          Row(
            children: [
              const GlassIcon(
                glyph: 'contract',
                from: PTColors.primary,
                to: PTColors.primary2,
                size: 32,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      _siteName(r.siteId),
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '分享率≥${r.minRatio} · 做种≥${r.minSeeding} · 上传≥${r.minUploadGb}GiB'
                      '${r.officialOnly ? " · 仅官种" : ""}'
                      '${r.minSeedingSizeGb > 0 ? " · 体积≥${r.minSeedingSizeGb}GiB" : ""}'
                      '${r.period == "weekly" ? " · 每周" : r.period == "monthly" ? " · 每月" : ""}',
                      style: TextStyle(color: context.dimText, fontSize: 12),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
              Switch(
                value: r.enabled,
                onChanged: (v) => _toggleEnabled(r, v),
                materialTapTargetSize: MaterialTapTargetSize.padded,
              ),
            ],
          ),
          const SizedBox(height: 10),
          // 状态徽章 + 上次检查时间。
          Row(
            children: [
              _statusBadge(r.lastStatus),
              const SizedBox(width: 8),
              Flexible(
                child: Text(
                  '检查于 ${_fmtDate(r.lastCheckedAt)}',
                  style: TextStyle(color: context.dimText, fontSize: 12),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          if (r.lastDetail.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              r.lastDetail,
              style: const TextStyle(fontSize: 12),
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
            ),
          ],
          const SizedBox(height: 10),
          // 操作行：检查 / 编辑 / 删除。
          Wrap(
            spacing: 6,
            runSpacing: 4,
            children: [
              FilledButton.tonal(
                onPressed: busy ? null : () => _check(r),
                child: const Text('检查', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _openEdit(r),
                child: const Text('编辑', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _delete(r),
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
