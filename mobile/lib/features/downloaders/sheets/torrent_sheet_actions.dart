// _TorrentSheetState 的动作/编辑对话框扩展：菜单、分享限制、分类/标签/限额编辑、
// 子表入口、添加种子、toast（无 setState 的段；从 torrent_sheet.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

extension on _TorrentSheetState {
  Future<void> _editShareLimits(Torrent t) async {
    String ratioMode = 'global';
    String seedMode = 'global';
    String ratioInput = '';
    String seedInput = '';
    final ok = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setS) => Padding(
          // 键盘避让：sheet 底部按视图插入区抬高，输入框不被遮挡。
          padding: EdgeInsets.only(
            bottom: MediaQuery.of(ctx).viewInsets.bottom,
          ),
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 20),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  '做种限制',
                  style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 16),
                const Text('分享比率', style: TextStyle(fontSize: 13)),
                DropdownButton<String>(
                  isExpanded: true,
                  value: ratioMode,
                  items: const [
                    DropdownMenuItem(value: 'global', child: Text('全局默认')),
                    DropdownMenuItem(value: 'unlimited', child: Text('无限')),
                    DropdownMenuItem(value: 'custom', child: Text('自定义')),
                  ],
                  onChanged: (v) => setS(() => ratioMode = v!),
                ),
                if (ratioMode == 'custom')
                  TextField(
                    keyboardType: const TextInputType.numberWithOptions(
                      decimal: true,
                    ),
                    decoration: const InputDecoration(hintText: '如 2.0'),
                    onChanged: (v) => ratioInput = v,
                  ),
                const SizedBox(height: 12),
                Text(
                  '做种时长${_type == 'tr' ? '（空闲）' : ''}',
                  style: const TextStyle(fontSize: 13),
                ),
                DropdownButton<String>(
                  isExpanded: true,
                  value: seedMode,
                  items: const [
                    DropdownMenuItem(value: 'global', child: Text('全局默认')),
                    DropdownMenuItem(value: 'unlimited', child: Text('无限')),
                    DropdownMenuItem(value: 'custom', child: Text('自定义')),
                  ],
                  onChanged: (v) => setS(() => seedMode = v!),
                ),
                if (seedMode == 'custom')
                  TextField(
                    keyboardType: TextInputType.number,
                    decoration: const InputDecoration(hintText: '分钟'),
                    onChanged: (v) => seedInput = v,
                  ),
                if (_type == 'tr')
                  Padding(
                    padding: EdgeInsets.only(top: 8),
                    child: Text(
                      'TR 做种时长为「空闲时长」语义',
                      style: TextStyle(fontSize: 11, color: context.dimText),
                    ),
                  ),
                const SizedBox(height: 20),
                Row(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: [
                    TextButton(
                      onPressed: () => Navigator.pop(ctx, false),
                      child: const Text('取消'),
                    ),
                    const SizedBox(width: 8),
                    FilledButton(
                      onPressed: () => Navigator.pop(ctx, true),
                      child: const Text('确定'),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
    if (ok != true) return;
    final ratio = ratioMode == 'global'
        ? -1.0
        : ratioMode == 'unlimited'
        ? -2.0
        : (double.tryParse(ratioInput) ?? 0);
    final seed = seedMode == 'global'
        ? -1
        : seedMode == 'unlimited'
        ? -2
        : (int.tryParse(seedInput) ?? 0);
    _act(
      () => _api.setShareLimits(_id, [
        t.hash,
      ], ShareLimits(ratioLimit: ratio, seedingTimeLimit: seed)),
      '已设置',
    );
  }

  void _onMenu(String val, Torrent t) async {
    switch (val) {
      case 'detail':
        _showDetail(t);
      case 'pause':
        _act(() => _api.pause(_id, [t.hash]), '已暂停');
      case 'resume':
        _act(() => _api.resume(_id, [t.hash]), '已恢复');
      case 'recheck':
        _act(() => _api.recheck(_id, [t.hash]), '已发起校验');
      case 'reannounce':
        _act(() => _api.reannounce(_id, [t.hash]), '已重新汇报');
      case 'forceStart':
        _act(() => _api.setForceStart(_id, [t.hash], !t.forceStart), '已切换强制开始');
      case 'top':
        _act(() => _api.setQueuePriority(_id, [t.hash], 'top'), '已置顶');
      case 'up':
        _act(() => _api.setQueuePriority(_id, [t.hash], 'up'), '已上移');
      case 'down':
        _act(() => _api.setQueuePriority(_id, [t.hash], 'down'), '已下移');
      case 'bottom':
        _act(() => _api.setQueuePriority(_id, [t.hash], 'bottom'), '已置底');
      case 'seq':
        _act(() => _api.toggleSequential(_id, [t.hash]), '已切换顺序下载');
      case 'firstlast':
        _act(() => _api.toggleFirstLast(_id, [t.hash]), '已切换首尾优先');
      case 'copyMagnet':
        _copy(t.magnet, '磁力链');
      case 'copyHash':
        _copy(t.infoHash.isNotEmpty ? t.infoHash : t.hash, '哈希');
      case 'remove':
        final df = await _confirmDelete(t.name);
        if (df != null) {
          _act(
            () => _api.removeTorrents(_id, [t.hash], deleteFiles: df),
            '已删除',
          );
        }
      case 'category':
        await _editCategory(t);
      case 'tags':
        await _editTags(t);
      case 'limits':
        await _editLimits(t);
      case 'shareLimits':
        await _editShareLimits(t);
      case 'files':
        _showFiles(t);
      case 'trackers':
        _showTrackers(t);
      case 'peers':
        _showPeers(t);
      case 'manageCats':
        _showCategoryManager();
    }
  }

  Future<void> _editCategory(Torrent t) async {
    try {
      final cats = await _api.categories(_id);
      if (!mounted) return;
      final sel = await showDialog<String>(
        context: context,
        builder: (ctx) => SimpleDialog(
          title: const Text('设置分类'),
          children: [
            ...cats.map(
              (c) => SimpleDialogOption(
                onPressed: () => Navigator.pop(ctx, c),
                child: Text(c),
              ),
            ),
            SimpleDialogOption(
              onPressed: () => Navigator.pop(ctx, ''),
              child: Text('（清除）', style: TextStyle(color: context.dimText)),
            ),
          ],
        ),
      );
      if (sel != null) _act(() => _api.setCategory(_id, [t.hash], sel), '已设置');
    } catch (_) {
      _toast('加载分类失败');
    }
  }

  Future<void> _editTags(Torrent t) async {
    final s = await _input('设置标签', '逗号分隔，覆盖现有', initial: t.tags.join(','));
    if (s == null) return;
    final tags = s
        .split(',')
        .map((e) => e.trim())
        .where((e) => e.isNotEmpty)
        .toList();
    _act(() => _api.editTags(_id, [t.hash], tags, 'set'), '已设置');
  }

  Future<void> _editLimits(Torrent t) async {
    final up = await _input('上传限速', 'KB/s，0=不限');
    if (up == null) return;
    final dl = await _input('下载限速', 'KB/s，0=不限');
    if (dl == null) return;
    final upB = (int.tryParse(up) ?? 0) * 1024;
    final dlB = (int.tryParse(dl) ?? 0) * 1024;
    _act(
      () => _api.setLimits(_id, [
        t.hash,
      ], Limits(uploadLimit: upB, downloadLimit: dlB)),
      '已设置',
    );
  }

  void _showPeers(Torrent t) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.82,
        child: _PeersSheet(id: _id, hash: t.hash),
      ),
    );
  }

  void _showDetail(Torrent t) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      useSafeArea: true,
      builder: (_) => DraggableScrollableSheet(
        expand: false,
        initialChildSize: 0.55,
        minChildSize: 0.35,
        maxChildSize: 0.92,
        builder: (ctx, scrollCtrl) => _DetailSheet(
          id: _id,
          type: _type,
          torrent: t,
          scrollController: scrollCtrl,
        ),
      ),
    );
  }

  void _showCategoryManager() {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.82,
        child: _CategorySheet(id: _id),
      ),
    );
  }

  void _showFiles(Torrent t) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.7,
        child: _FilesSheet(id: _id, hash: t.hash, type: _type),
      ),
    );
  }

  void _showTrackers(Torrent t) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.7,
        child: _TrackersSheet(id: _id, hash: t.hash),
      ),
    );
  }

  void _add() {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => Padding(
        padding: EdgeInsets.only(
          bottom: MediaQuery.of(context).viewInsets.bottom,
        ),
        child: _AddTorrentSheet(downloader: widget.downloader),
      ),
    );
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  String _cleanErr(Object e) => e.toString().replaceFirst('Exception: ', '');

  Future<void> _act(Future<void> Function() fn, String ok) async {
    try {
      await fn();
      _toast(ok);
    } catch (e) {
      _toast(_cleanErr(e));
    }
  }

  Future<bool?> _confirmDelete(String name) async {
    bool delFiles = false;
    return showDialog<bool>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setS) => AlertDialog(
          title: const Text('删除任务'),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text('确认删除「$name」？'),
              CheckboxListTile(
                value: delFiles,
                onChanged: (v) => setS(() => delFiles = v ?? false),
                title: const Text('同时删除文件'),
                controlAffinity: ListTileControlAffinity.leading,
                dense: true,
                contentPadding: EdgeInsets.zero,
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx, null),
              child: const Text('取消'),
            ),
            TextButton(
              onPressed: () => Navigator.pop(ctx, delFiles),
              child: const Text('删除'),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _copy(String text, String label) async {
    await Clipboard.setData(ClipboardData(text: text));
    _toast('$label已复制');
  }

}
