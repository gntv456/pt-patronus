// 从 downloaders_page.dart 拆出（part of 主文件，共享私有类型/工具函数；行为不变）。
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// 任务详情：概览信息 + qB 标志 + Peers/文件/Tracker 子入口。
class _DetailSheet extends StatelessWidget {
  final int id;
  final String type;
  final Torrent torrent;
  final ScrollController? scrollController;
  const _DetailSheet({
    required this.id,
    required this.type,
    required this.torrent,
    this.scrollController,
  });

  Widget _kv(BuildContext context, String k, String v) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 3),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 84,
          child: Text(
            k,
            style: TextStyle(color: context.dimText, fontSize: 12),
          ),
        ),
        Expanded(child: Text(v, style: const TextStyle(fontSize: 13))),
      ],
    ),
  );

  Widget _flag(BuildContext context, String label, bool on) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
    decoration: BoxDecoration(
      borderRadius: BorderRadius.circular(6),
      color: on
          ? Colors.deepPurple.withValues(alpha: 0.18)
          : context.dimText.withValues(alpha: 0.15),
    ),
    child: Text(
      label,
      style: TextStyle(
        fontSize: 11,
        color: on ? Colors.deepPurple : context.dimText,
      ),
    ),
  );

  void _openPeers(BuildContext context) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.7,
        child: _PeersSheet(id: id, hash: torrent.hash),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final t = torrent;
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(14),
          child: Text(
            t.name,
            style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        const Divider(height: 1),
        Expanded(
          child: ListView(
            controller: scrollController,
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            children: [
              _kv(context, '大小', _fmtBytes(t.size)),
              _kv(context,
                '已下载/上传',
                '${_fmtBytes(t.downloaded)} / ${_fmtBytes(t.uploaded)}',
              ),
              _kv(context, '分享率', t.ratio.toStringAsFixed(2)),
              _kv(context, '进度', '${(t.progress * 100).toStringAsFixed(1)}%'),
              if (_dlCaps(type).swarm)
                _kv(context,
                  '可用性',
                  t.availability > 0 ? t.availability.toStringAsFixed(2) : '-',
                ),
              _kv(context, '做种/下载', '${t.numSeeds} / ${t.numLeechs}'),
              if (_dlCaps(type).swarm)
                _kv(context, 'Swarm 做/下', '${t.numComplete} / ${t.numIncomplete}'),
              if (_dlCaps(type).swarm) _kv(context, '队列位置', '${t.priority}'),
              if (t.addedAt.isNotEmpty) _kv(context, '创建', _fmtIso(t.addedAt)),
              if (_isCompleted(t.completedAt))
                _kv(context, '完成', _fmtIso(t.completedAt)),
              if (_dlCaps(type).forceStart || _dlCaps(type).sequential || _dlCaps(type).firstLast) ...[
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    _flag(context, '强制开始', t.forceStart),
                    _flag(context, '顺序下载', t.sequential),
                    _flag(context, '首尾优先', t.firstLastPiece),
                    _flag(context, '自动管理', t.autoTmm),
                  ],
                ),
              ],
              const SizedBox(height: 16),
              OutlinedButton.icon(
                icon: const Icon(Icons.people_outline, size: 18),
                label: const Text('Peers'),
                onPressed: () => _openPeers(context),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
