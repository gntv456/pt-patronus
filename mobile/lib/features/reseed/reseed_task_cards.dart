// _ReseedPageState 的任务卡片区扩展：任务列表区/单任务卡。
// （从 reseed_widgets.dart 拆出；行为不变。）
part of 'package:ptpatronus/features/reseed/reseed_page.dart';

extension on _ReseedPageState {
  // 任务区：宽屏表格 / 窄屏卡片，由 PTAdaptiveTable 统一收敛。
  Widget _tasksArea({required bool narrow}) {
    if (_loading) {
      return const PTLoadingView();
    }
    if (_tasks.isEmpty) {
      return PTEmptyState(
        icon: Icons.eco_outlined,
        text: '暂无候选',
        description: '点「开始辅种」扫描跨站可辅助种子',
      );
    }
    return PTAdaptiveTable<ReseedTask>(
      items: _tasks,
      columns: [
        PTColumn<ReseedTask>(
          label: '源种子',
          flex: 3,
          cell: (_, t) => Text(
            t.sourceName.isEmpty ? t.infoHash : t.sourceName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 13),
          ),
        ),
        PTColumn<ReseedTask>(
          label: '候选站点',
          flex: 2,
          cell: (_, t) =>
              Text(t.candidateSite, style: const TextStyle(fontSize: 13)),
        ),
        PTColumn<ReseedTask>(
          label: '下载链接',
          flex: 2,
          cell: (_, t) => t.candidateUrl.isEmpty
              ? Text(
                  '—',
                  style: TextStyle(color: context.dimText, fontSize: 12),
                )
              : InkWell(
                  onTap: () => _copyLink(t.candidateUrl),
                  child: const Text(
                    '复制链接',
                    style: TextStyle(
                      fontSize: 12,
                      color: PTColors.primary2,
                      decoration: TextDecoration.underline,
                    ),
                  ),
                ),
        ),
        PTColumn<ReseedTask>(
          label: '保存路径',
          flex: 2,
          cell: (_, t) => Text(
            t.savePath.isEmpty ? '-' : t.savePath,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
        ),
        PTColumn<ReseedTask>(
          label: '状态',
          flex: 1,
          cell: (_, t) => Container(
            padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
            decoration: BoxDecoration(
              color: _statusColor(t.status).withValues(alpha: 0.16),
              borderRadius: BorderRadius.circular(999),
            ),
            child: Text(
              _statusLabel(t.status),
              style: TextStyle(
                color: _statusColor(t.status),
                fontSize: 11,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ),
        PTColumn<ReseedTask>(
          label: '操作',
          flex: 2,
          cell: (_, t) => t.status == 'pending'
              ? Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    FilledButton.tonal(
                      onPressed: _resolving.contains(t.id)
                          ? null
                          : () => _resolve(t, 'add'),
                      child: const Text('添加', style: TextStyle(fontSize: 12)),
                    ),
                    const SizedBox(width: 6),
                    TextButton(
                      onPressed: _resolving.contains(t.id)
                          ? null
                          : () => _resolve(t, 'skip'),
                      child: const Text('跳过', style: TextStyle(fontSize: 12)),
                    ),
                  ],
                )
              : Text(
                  '—',
                  style: TextStyle(color: context.dimText, fontSize: 12),
                ),
        ),
      ],
      narrowCard: (_, t) => _taskCard(t),
      emptyText: '暂无候选',
      embedded: true, // 窄屏嵌在 _narrowBody ListView：shrinkWrap 避免塌缩（宽屏分支不受影响）
    );
  }

  Widget _taskCard(ReseedTask t) {
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  t.sourceName.isEmpty ? t.infoHash : t.sourceName,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 14,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                decoration: BoxDecoration(
                  color: _statusColor(t.status).withValues(alpha: 0.16),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  _statusLabel(t.status),
                  style: TextStyle(
                    color: _statusColor(t.status),
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Text(
            '候选站点：${t.candidateSite}  ·  保存：${t.savePath.isEmpty ? '-' : t.savePath}',
            style: TextStyle(color: context.dimText, fontSize: 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
          if (t.candidateUrl.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: GestureDetector(
                onTap: () => _copyLink(t.candidateUrl),
                child: const Text(
                  '复制下载链接',
                  style: TextStyle(
                    fontSize: 12,
                    color: PTColors.primary2,
                    decoration: TextDecoration.underline,
                  ),
                ),
              ),
            ),
          if (t.status == 'pending') ...[
            const SizedBox(height: 8),
            Row(
              children: [
                FilledButton.tonal(
                  onPressed: _resolving.contains(t.id)
                      ? null
                      : () => _resolve(t, 'add'),
                  child: const Text('添加辅种', style: TextStyle(fontSize: 13)),
                ),
                const SizedBox(width: 8),
                TextButton(
                  onPressed: _resolving.contains(t.id)
                      ? null
                      : () => _resolve(t, 'skip'),
                  child: const Text('跳过', style: TextStyle(fontSize: 13)),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}
