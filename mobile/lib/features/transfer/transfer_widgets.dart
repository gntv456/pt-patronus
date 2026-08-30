// TransferPage 的展示层扩展（无 setState 段；从 transfer_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/transfer/transfer_page.dart';

extension on _TransferPageState {
  Widget _empty() {
    return ListView(children: [
      const SizedBox(height: 80),
      PTEmptyState(icon: Icons.inbox, title: '暂无整理记录', description: '订阅/RSS 下载完成后会自动整理；也可点「手动整理」'),
    ]);
  }

  Widget _historyCard(TransferHistory h) {
    final success = h.status == 'success';
    return GlassCard(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 10, 10, 10),
        child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          _poster(h.posterUrl),
          const SizedBox(width: 12),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Row(children: [
                Expanded(
                  child: Text('${h.title}${h.year > 0 ? ' (${h.year})' : ''}',
                      style: const TextStyle(fontWeight: FontWeight.w600), maxLines: 1, overflow: TextOverflow.ellipsis),
                ),
                _statusChip(success),
              ]),
              const SizedBox(height: 4),
              Wrap(spacing: 6, runSpacing: 4, children: [
                _miniTag(h.type == 'tv' ? '剧集' : '电影'),
                if (h.mode.isNotEmpty) _miniTag(_modeLabel(h.mode)),
                if (h.episodes.isNotEmpty) _miniTag(h.episodes),
              ]),
              const SizedBox(height: 4),
              Text(h.dest, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 11, color: Theme.of(context).hintColor)),
              if (!success && h.errorMsg.isNotEmpty)
                Padding(padding: const EdgeInsets.only(top: 2), child: Text(h.errorMsg, maxLines: 2, overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 11, color: PTColors.danger))),
              Row(children: [
                Expanded(child: Text(_fmtTime(h.createdAt), style: TextStyle(fontSize: 11, color: Theme.of(context).hintColor))),
                IconButton(
                  icon: const Icon(Icons.refresh, size: 18),
                  tooltip: '重做',
                  onPressed: _redoing.contains(h.id) ? null : () => _redo(h),
                ),
                IconButton(icon: const Icon(Icons.delete_outline, size: 18), tooltip: '删除', onPressed: () => _delete(h)),
              ]),
            ]),
          ),
        ]),
      ),
    );
  }

  Widget _poster(String url) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(6),
      child: url.isEmpty
          ? Container(width: 40, height: 58, color: Theme.of(context).colorScheme.surfaceContainerHighest, child: const Icon(Icons.movie, size: 20))
          : Image.network(url, width: 40, height: 58, fit: BoxFit.cover, errorBuilder: (_, _, _) => Container(width: 40, height: 58, color: Theme.of(context).colorScheme.surfaceContainerHighest, child: const Icon(Icons.movie, size: 20))),
    );
  }

  Widget _statusChip(bool success) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(color: (success ? PTColors.success : PTColors.danger).withValues(alpha: 0.15), borderRadius: BorderRadius.circular(4)),
      child: Text(success ? '成功' : '失败', style: TextStyle(fontSize: 10, color: success ? PTColors.success : PTColors.danger)),
    );
  }

}
