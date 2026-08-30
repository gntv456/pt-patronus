// _TorrentSheetState 的展示层扩展：种子行/状态标签/速率面板/输入工具。
// 同库 extension 可访问私有成员；从 torrent_sheet.dart 拆出，行为不变。
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

extension on _TorrentSheetState {
  /// 左滑删除（可 SnackBar 撤销 4s）；右滑暂停/恢复（不移除行）。
  Widget _row(Torrent t) {
    final paused = _isPaused(t);
    return Dismissible(
      key: ValueKey('torrent-${t.hash}'),
      direction: DismissDirection.horizontal,
      confirmDismiss: (dir) async {
        if (dir == DismissDirection.startToEnd) {
          // 右滑：暂停 / 恢复，不移除列表项
          if (paused) {
            await _act(() => _api.resume(_id, [t.hash]), '已恢复');
          } else {
            await _act(() => _api.pause(_id, [t.hash]), '已暂停');
          }
          return false;
        }
        // 左滑：删除（确认是否删文件后，乐观移除 + 撤销）
        return _swipeDelete(t);
      },
      background: Container(
        color: (paused ? Colors.green : Colors.orange).withValues(alpha: 0.88),
        alignment: Alignment.centerLeft,
        padding: const EdgeInsets.only(left: 20),
        child: Row(
          children: [
            Icon(paused ? Icons.play_arrow : Icons.pause, color: Colors.white),
            const SizedBox(width: 8),
            Text(
              paused ? '恢复' : '暂停',
              style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600),
            ),
          ],
        ),
      ),
      secondaryBackground: Container(
        color: Colors.red.withValues(alpha: 0.88),
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 20),
        child: const Row(
          mainAxisAlignment: MainAxisAlignment.end,
          children: [
            Text('删除', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w600)),
            SizedBox(width: 8),
            Icon(Icons.delete_outline, color: Colors.white),
          ],
        ),
      ),
      child: ListTile(
        onTap: () => _showDetail(t),
        title: Text(t.name, maxLines: 1, overflow: TextOverflow.ellipsis),
        subtitle: Text(
          '${(t.progress * 100).toStringAsFixed(0)}% · ${_stateLabel(t.state)} · ↓${_fmtSpeed(t.downRate)} ↑${_fmtSpeed(t.upRate)}'
          '${t.tags.isNotEmpty ? ' · ${t.tags.join(",")}' : ''}',
          style: const TextStyle(fontSize: 12),
        ),
        trailing: PopupMenuButton<String>(
          icon: const Icon(Icons.more_vert),
          onSelected: (v) => _onMenu(v, t),
          itemBuilder: (_) => [
            const PopupMenuItem(value: 'detail', child: Text('详情…')),
            const PopupMenuItem(value: 'pause', child: Text('暂停')),
            const PopupMenuItem(value: 'resume', child: Text('恢复')),
            const PopupMenuItem(value: 'recheck', child: Text('重新校验')),
            const PopupMenuItem(value: 'reannounce', child: Text('重新汇报')),
            if (_dlCaps(_type).forceStart || _dlCaps(_type).queueFull || _dlCaps(_type).sequential || _dlCaps(_type).firstLast)
              const PopupMenuDivider(),
            if (_dlCaps(_type).forceStart)
              PopupMenuItem(
                value: 'forceStart',
                child: Text(t.forceStart ? '取消强制开始' : '强制开始'),
              ),
            if (_dlCaps(_type).queueFull) ...[
              const PopupMenuItem(value: 'top', child: Text('队列置顶')),
              const PopupMenuItem(value: 'up', child: Text('队列上移')),
              const PopupMenuItem(value: 'down', child: Text('队列下移')),
              const PopupMenuItem(value: 'bottom', child: Text('队列置底')),
            ],
            if (_dlCaps(_type).sequential)
              PopupMenuItem(
                value: 'seq',
                child: Text(t.sequential ? '取消顺序下载' : '顺序下载'),
              ),
            if (_dlCaps(_type).firstLast)
              PopupMenuItem(
                value: 'firstlast',
                child: Text(t.firstLastPiece ? '取消首尾优先' : '首尾优先'),
              ),
            const PopupMenuDivider(),
            const PopupMenuItem(value: 'remove', child: Text('删除…')),
            if (_dlCaps(_type).category)
              const PopupMenuItem(value: 'category', child: Text('设置分类…')),
            if (_dlCaps(_type).tags)
              const PopupMenuItem(value: 'tags', child: Text('设置标签…')),
            const PopupMenuItem(value: 'limits', child: Text('限速…')),
            const PopupMenuItem(value: 'shareLimits', child: Text('做种限制…')),
            if (_dlCaps(_type).files)
              const PopupMenuItem(value: 'files', child: Text('文件优先级…')),
            if (_dlCaps(_type).trackers)
              const PopupMenuItem(value: 'trackers', child: Text('Tracker…')),
            if (_dlCaps(_type).peers)
              const PopupMenuItem(value: 'peers', child: Text('Peers…')),
            const PopupMenuDivider(),
            const PopupMenuItem(value: 'copyMagnet', child: Text('复制磁力链')),
            const PopupMenuItem(value: 'copyHash', child: Text('复制哈希')),
          ],
        ),
      ),
    );
  }

  String _stateLabel(String s) {
    const m = {
      'downloading': '下载中',
      'uploading': '做种',
      'paused': '暂停',
      'stopped': '停止',
      'stalled': '等待',
      'error': '错误',
      'checking': '校验',
      'queued': '排队',
    };
    for (final e in m.entries) {
      if (s.contains(e.key)) return e.value;
    }
    return s;
  }

  Widget _statsRow() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(14, 4, 14, 10),
      child: Row(
        children: [
          _stat('↓', _fmtSpeed(_dlRate), PTColors.primary2),
          const SizedBox(width: 16),
          _stat('↑', _fmtSpeed(_upRate), PTColors.success),
          const Spacer(),
          _count('下载', _active),
          const SizedBox(width: 12),
          _count('做种', _seeding),
          const SizedBox(width: 12),
          _count('暂停', _paused),
        ],
      ),
    );
  }

  Widget _stat(String arrow, String speed, Color color) {
    return Row(
      children: [
        Text(arrow, style: TextStyle(color: color, fontSize: 14)),
        const SizedBox(width: 4),
        Text(
          speed,
          style: TextStyle(
            color: color,
            fontWeight: FontWeight.w600,
            fontSize: 13,
          ),
        ),
      ],
    );
  }

  Widget _count(String label, int n) {
    return Text(
      '$label $n',
      style: TextStyle(color: context.dimText, fontSize: 12),
    );
  }

  bool _isPaused(Torrent t) =>
      t.state.contains('pause') || t.state.contains('stop');

  /// 左滑删除（可 SnackBar 撤销 4s）；右滑暂停/恢复（不移除行）。

  Future<String?> _input(
    String title,
    String hint, {
    String initial = '',
  }) async {
    final ctrl = TextEditingController(text: initial);
    return showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(title),
        content: TextField(
          controller: ctrl,
          decoration: InputDecoration(hintText: hint),
          autofocus: true,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, ctrl.text.trim()),
            child: const Text('确定'),
          ),
        ],
      ),
    );
  }
}
