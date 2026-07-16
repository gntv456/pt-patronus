import 'dart:async';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

// 下载器类型 → 品牌图标/配色/标签（与 Web Downloaders.vue 的 DL_META 1:1）。
// 新增下载器类型时在此加一条 case，卡片徽标与表单下拉会自动用上对应图标。
({String glyph, Color from, Color to, String label}) _dlMeta(String t) {
  switch (t) {
    case 'tr':
      return (
        glyph: 'transmission',
        from: const Color(0xFFE8443C),
        to: const Color(0xFFA82424),
        label: 'Transmission',
      );
    case 'ut':
      return (
        glyph: 'utorrent',
        from: const Color(0xFF5BB04C),
        to: const Color(0xFF3A7A2E),
        label: 'uTorrent',
      );
    case 'dl':
      return (
        glyph: 'deluge',
        from: const Color(0xFF4FB6E8),
        to: const Color(0xFF2A7AB3),
        label: 'Deluge',
      );
    default:
      return (
        glyph: 'qbittorrent',
        from: const Color(0xFF72B4F5),
        to: const Color(0xFF356EBF),
        label: 'qBittorrent',
      );
  }
}

// 表单类型选项：品牌 GlassIcon + 名称（下拉选中态与展开列表均显示）。
Widget _dlOption(({String glyph, Color from, Color to, String label}) m) {
  return Row(
    children: [
      GlassIcon(glyph: m.glyph, from: m.from, to: m.to, size: 22),
      const SizedBox(width: 8),
      Text(m.label),
    ],
  );
}

// 下载器能力位（与 Web caps.ts 1:1）：UI 按能力门控，新增类型在此加一条 case。
({bool category, bool categoryCRUD, bool forceStart, bool queueFull, bool sequential, bool firstLast, bool swarm, bool tags, bool trackers, List<({String label, int value})> filePrio}) _dlCaps(String t) {
  switch (t) {
    case 'tr':
      return (category: false, categoryCRUD: false, forceStart: false, queueFull: false, sequential: false, firstLast: false, swarm: false, tags: true, trackers: true, filePrio: const [(label: '低', value: -1), (label: '正常', value: 0), (label: '高', value: 1)]);
    case 'ut':
      return (category: true, categoryCRUD: false, forceStart: true, queueFull: true, sequential: false, firstLast: false, swarm: true, tags: false, trackers: false, filePrio: const [(label: '跳过', value: 0), (label: '低', value: 1), (label: '正常', value: 2), (label: '高', value: 3)]);
    case 'dl':
      return (category: true, categoryCRUD: false, forceStart: false, queueFull: true, sequential: false, firstLast: false, swarm: true, tags: false, trackers: true, filePrio: const [(label: '跳过', value: 0), (label: '低', value: 1), (label: '正常', value: 2), (label: '高', value: 3)]);
    default: // qb
      return (category: true, categoryCRUD: true, forceStart: true, queueFull: true, sequential: true, firstLast: true, swarm: true, tags: true, trackers: true, filePrio: const [(label: '不下载', value: 0), (label: '正常', value: 2), (label: '高', value: 1), (label: '最高', value: 7)]);
  }
}

// 子表（文件/Tracker/Peer/分类）加载失败的统一错误视图：文案 + 重试按钮，
// 与「真空态」（无文件/无 Tracker…）区分，避免失败被误显示为空。
Widget _sheetErrorView(String error, VoidCallback onRetry) {
  return Center(
    child: Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            '加载失败：$error',
            style: const TextStyle(color: Colors.redAccent, fontSize: 13),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 10),
          OutlinedButton(onPressed: onRetry, child: const Text('重试')),
        ],
      ),
    ),
  );
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
        _error = e.toString();
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
                      style: const TextStyle(color: Colors.grey, fontSize: 12),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Text(
            d.url,
            style: const TextStyle(color: Colors.grey, fontSize: 12),
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

// 下载器添加/编辑表单（BottomSheet）：name/type/url/username/password/is_default。
// 编辑态 downloader 非 null，密码留空=不修改（api 仅非空才传）。新建态不显示「默认」开关（add 固定 false）。
class _DownloaderFormSheet extends StatefulWidget {
  final Downloader? downloader;
  const _DownloaderFormSheet({this.downloader});
  @override
  State<_DownloaderFormSheet> createState() => _DownloaderFormSheetState();
}

class _DownloaderFormSheetState extends State<_DownloaderFormSheet> {
  final _api = DownloaderApi();
  late final TextEditingController _name, _url, _username, _password;
  String _type = 'qb';
  bool _isDefault = false;
  bool _saving = false;

  bool get _isEdit => widget.downloader != null;

  @override
  void initState() {
    super.initState();
    final d = widget.downloader;
    _name = TextEditingController(text: d?.name ?? '');
    _url = TextEditingController(text: d?.url ?? '');
    _username = TextEditingController(text: d?.username ?? '');
    _password = TextEditingController(); // 编辑态留空=不修改
    _type = d?.type ?? 'qb';
    _isDefault = d?.isDefault ?? false;
  }

  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    _username.dispose();
    _password.dispose();
    super.dispose();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _submit() async {
    final name = _name.text.trim();
    final url = _url.text.trim();
    if (name.isEmpty || url.isEmpty) {
      _toast('名称与地址必填');
      return;
    }
    if (!RegExp(r'^https?://.+').hasMatch(url)) {
      _toast('地址需以 http:// 或 https:// 开头');
      return;
    }
    setState(() => _saving = true);
    try {
      if (_isEdit) {
        await _api.update(
          widget.downloader!.id,
          name: name,
          type: _type,
          url: url,
          username: _username.text.trim(),
          password: _password.text,
          isDefault: _isDefault,
        );
        _toast('已更新');
      } else {
        await _api.add(
          name: name,
          type: _type,
          url: url,
          username: _username.text.trim(),
          password: _password.text,
        );
        _toast('已添加');
      }
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    // 标题与键盘避让由 showAdaptiveSheet 浮层容器统一处理（宽屏对话框 / 窄屏底栏）。
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: _name,
          decoration: const InputDecoration(labelText: '名称'),
        ),
        const SizedBox(height: 12),
        DropdownButtonFormField<String>(
          initialValue: _type,
          decoration: const InputDecoration(labelText: '类型'),
          items: [
            DropdownMenuItem(value: 'qb', child: _dlOption(_dlMeta('qb'))),
            DropdownMenuItem(value: 'tr', child: _dlOption(_dlMeta('tr'))),
            DropdownMenuItem(value: 'ut', child: _dlOption(_dlMeta('ut'))),
            DropdownMenuItem(value: 'dl', child: _dlOption(_dlMeta('dl'))),
          ],
          onChanged: (v) => setState(() => _type = v ?? 'qb'),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _url,
          decoration: const InputDecoration(
            labelText: '地址',
            hintText: 'http://127.0.0.1:8080',
          ),
          keyboardType: TextInputType.url,
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _username,
          decoration: const InputDecoration(labelText: '用户名'),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _password,
          obscureText: true,
          decoration: InputDecoration(
            labelText: '密码',
            hintText: _isEdit ? '留空不修改' : null,
          ),
        ),
        if (_isEdit)
          SwitchListTile(
            value: _isDefault,
            onChanged: (v) => setState(() => _isDefault = v),
            title: const Text('设为默认下载器'),
            contentPadding: EdgeInsets.zero,
            dense: true,
          ),
        const SizedBox(height: 20),
        Row(
          mainAxisAlignment: MainAxisAlignment.end,
          children: [
            TextButton(
              onPressed: _saving ? null : () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            const SizedBox(width: 8),
            FilledButton(
              onPressed: _saving ? null : _submit,
              child: Text(_isEdit ? '保存' : '添加'),
            ),
          ],
        ),
      ],
    );
  }
}

// 任务弹窗：订阅 WsClient 实时种子流，顶部速率面板 + 添加入口 + 每行操作菜单。
class _TorrentSheet extends StatefulWidget {
  final Downloader downloader;
  const _TorrentSheet({required this.downloader});
  @override
  State<_TorrentSheet> createState() => _TorrentSheetState();
}

class _TorrentSheetState extends State<_TorrentSheet> {
  final _ws = WsClient();
  final _api = DownloaderApi();
  List<Torrent> _torrents = [];
  int? _dlRate;
  int? _upRate;
  String _error = '';
  bool _connecting = true;
  StreamSubscription? _sub;

  int get _id => widget.downloader.id;
  String get _type => widget.downloader.type;

  @override
  void initState() {
    super.initState();
    _ws.connect();
    _sub = _ws.snapshots.listen((list) {
      final match = list.where((s) => s.downloaderId == _id).toList();
      if (match.isEmpty) return; // 本次广播不含此下载器，保留上次显示
      final s = match.first;
      if (!mounted) return;
      setState(() {
        _torrents = s.torrents;
        _dlRate = s.dlRate;
        _upRate = s.upRate;
        _error = s.error;
        _connecting = false;
      });
    });
  }

  @override
  void dispose() {
    _sub?.cancel(); // 仅取消本页订阅，不断开共享的 WsClient
    super.dispose();
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

  int get _active =>
      _torrents.where((t) => t.state.contains('download')).length;
  int get _seeding => _torrents
      .where((t) => t.state.contains('upload') || t.state.contains('seed'))
      .length;
  int get _paused => _torrents
      .where((t) => t.state.contains('pause') || t.state.contains('stop'))
      .length;

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
        _copy(t.hash, '哈希');
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

  Future<void> _copy(String text, String label) async {
    await Clipboard.setData(ClipboardData(text: text));
    _toast('$label已复制');
  }

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
                  const Padding(
                    padding: EdgeInsets.only(top: 8),
                    child: Text(
                      'TR 做种时长为「空闲时长」语义',
                      style: TextStyle(fontSize: 11, color: Colors.grey),
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
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.9,
        child: _DetailSheet(id: _id, type: _type, torrent: t),
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
              child: const Text('（清除）', style: TextStyle(color: Colors.grey)),
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

  @override
  Widget build(BuildContext context) {
    final d = widget.downloader;
    Widget body;
    if (_connecting) {
      body = const Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CircularProgressIndicator(),
            SizedBox(height: 12),
            Text('连接实时推送…', style: TextStyle(color: Colors.grey)),
          ],
        ),
      );
    } else if (_error.isNotEmpty) {
      body = Center(
        child: Text(
          '拉取失败：$_error',
          style: const TextStyle(color: Colors.redAccent),
        ),
      );
    } else if (_torrents.isEmpty) {
      body = const Center(
        child: Text('无任务', style: TextStyle(color: Colors.grey)),
      );
    } else {
      body = ListView.separated(
        itemCount: _torrents.length,
        separatorBuilder: (_, _) =>
            const Divider(height: 1, indent: 12, endIndent: 12),
        itemBuilder: (_, i) => _row(_torrents[i]),
      );
    }
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(14),
          child: Row(
            children: [
              Text(
                '${d.name} · 任务',
                style: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const Spacer(),
              if (_dlCaps(_type).categoryCRUD)
                IconButton(
                  icon: const Icon(Icons.category_outlined),
                  onPressed: _showCategoryManager,
                  tooltip: '分类管理',
                ),
              IconButton(
                icon: const Icon(Icons.add_circle_outline),
                onPressed: _add,
                tooltip: '添加种子',
              ),
              const SizedBox(width: 4),
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                  color: _ws.isConnected ? PTColors.success : Colors.grey,
                  shape: BoxShape.circle,
                ),
              ),
            ],
          ),
        ),
        _statsRow(),
        const Divider(height: 1),
        Expanded(child: body),
      ],
    );
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
      style: const TextStyle(color: Colors.grey, fontSize: 12),
    );
  }

  Widget _row(Torrent t) {
    return ListTile(
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
          const PopupMenuItem(value: 'files', child: Text('文件优先级…')),
          if (_dlCaps(_type).trackers)
            const PopupMenuItem(value: 'trackers', child: Text('Tracker…')),
          const PopupMenuItem(value: 'peers', child: Text('Peers…')),
          const PopupMenuDivider(),
          const PopupMenuItem(value: 'copyMagnet', child: Text('复制磁力链')),
          const PopupMenuItem(value: 'copyHash', child: Text('复制哈希')),
        ],
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
}

// 添加种子：URL / 磁力 + .torrent 文件选择 + 高级（暂停）。
class _AddTorrentSheet extends StatefulWidget {
  final Downloader downloader;
  const _AddTorrentSheet({required this.downloader});
  @override
  State<_AddTorrentSheet> createState() => _AddTorrentSheetState();
}

class _AddTorrentSheetState extends State<_AddTorrentSheet> {
  final _api = DownloaderApi();
  final _urlCtrl = TextEditingController();
  bool _loading = false;
  bool _paused = false;

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _addUrl() async {
    if (_urlCtrl.text.isEmpty) return;
    setState(() => _loading = true);
    try {
      await _api.addTorrent(
        widget.downloader.id,
        _urlCtrl.text,
        opts: {'paused': _paused},
      );
      _toast('已添加');
      if (mounted) Navigator.pop(context);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _pickFile() async {
    setState(() => _loading = true);
    try {
      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['torrent'],
      );
      if (result == null || result.files.isEmpty) return;
      final f = result.files.first;
      http.MultipartFile mf;
      if (f.path != null) {
        mf = await http.MultipartFile.fromPath('file', f.path!);
      } else {
        mf = http.MultipartFile.fromBytes(
          'file',
          f.bytes ?? <int>[],
          filename: f.name,
        );
      }
      await _api.uploadTorrent(widget.downloader.id, mf);
      _toast('已上传');
      if (mounted) Navigator.pop(context);
    } catch (e) {
      // 鸿蒙等无原生实现时，file_picker 会抛 MissingPluginException，提示用链接
      _toast('选文件不可用，请用链接添加');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            '添加种子到「${widget.downloader.name}」',
            style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 14),
          TextField(
            controller: _urlCtrl,
            decoration: const InputDecoration(
              hintText: 'magnet:?xt=... 或 .torrent 下载链接',
              border: OutlineInputBorder(),
            ),
            minLines: 1,
            maxLines: 3,
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              Expanded(
                child: FilledButton.icon(
                  onPressed: _loading ? null : _addUrl,
                  icon: const Icon(Icons.link),
                  label: const Text('添加链接'),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: _loading ? null : _pickFile,
                  icon: const Icon(Icons.upload_file),
                  label: const Text('.torrent 文件'),
                ),
              ),
            ],
          ),
          SwitchListTile(
            value: _paused,
            onChanged: _loading ? null : (v) => setState(() => _paused = v),
            title: const Text('以暂停状态添加'),
            contentPadding: EdgeInsets.zero,
            dense: true,
          ),
        ],
      ),
    );
  }
}

// 文件优先级管理。
class _FilesSheet extends StatefulWidget {
  final int id;
  final String hash;
  final String type;
  const _FilesSheet({required this.id, required this.hash, required this.type});
  @override
  State<_FilesSheet> createState() => _FilesSheetState();
}

class _FilesSheetState extends State<_FilesSheet> {
  final _api = DownloaderApi();
  List<TorrentFile> _files = [];
  bool _loading = true;
  String _error = ''; // 加载失败文案；非空时显示「加载失败+重试」而非「无文件」（区分失败与真空）

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final f = await _api.files(widget.id, widget.hash);
      if (mounted) {
        setState(() {
          _files = f;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  List<DropdownMenuItem<int>> _prioOpts() => [
        for (final o in _dlCaps(widget.type).filePrio)
          DropdownMenuItem(value: o.value, child: Text(o.label)),
      ];

  Future<void> _setPrio(TorrentFile f, int p) async {
    try {
      await _api.setFilePriority(widget.id, widget.hash, [f.index], p);
      _toast('已更新');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.all(14),
          child: Text(
            '文件优先级',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
        ),
        const Divider(height: 1),
        Expanded(
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _files.isEmpty
              ? const Center(
                  child: Text('无文件', style: TextStyle(color: Colors.grey)),
                )
              : ListView.builder(
                  itemCount: _files.length,
                  itemBuilder: (_, i) {
                    final f = _files[i];
                    return ListTile(
                      dense: true,
                      title: Text(
                        f.name,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                      subtitle: Text(
                        '${_fmtBytes(f.size)} · ${(f.progress * 100).toStringAsFixed(0)}%',
                        style: const TextStyle(fontSize: 12),
                      ),
                      trailing: DropdownButton<int>(
                        value: f.priority,
                        items: _prioOpts(),
                        onChanged: (p) => p != null ? _setPrio(f, p) : null,
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}

// Tracker 管理：列表 + 增/删。
class _TrackersSheet extends StatefulWidget {
  final int id;
  final String hash;
  const _TrackersSheet({required this.id, required this.hash});
  @override
  State<_TrackersSheet> createState() => _TrackersSheetState();
}

class _TrackersSheetState extends State<_TrackersSheet> {
  final _api = DownloaderApi();
  final _newCtrl = TextEditingController();
  List<Tracker> _trackers = [];
  bool _loading = true;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final t = await _api.trackers(widget.id, widget.hash);
      if (mounted) {
        setState(() {
          _trackers = t;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  Future<void> _add() async {
    if (_newCtrl.text.trim().isEmpty) return;
    try {
      await _api.addTrackers(widget.id, widget.hash, [_newCtrl.text.trim()]);
      _newCtrl.clear();
      _toast('已添加');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  Future<void> _remove(String url) async {
    try {
      await _api.removeTrackers(widget.id, widget.hash, [url]);
      _toast('已移除');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.all(14),
          child: Text(
            'Tracker 管理',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _newCtrl,
                  decoration: const InputDecoration(
                    hintText: 'https://tracker.../announce',
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(icon: const Icon(Icons.add), onPressed: _add),
            ],
          ),
        ),
        const Divider(height: 16),
        Expanded(
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _trackers.isEmpty
              ? const Center(
                  child: Text(
                    '无 Tracker',
                    style: TextStyle(color: Colors.grey),
                  ),
                )
              : ListView.builder(
                  itemCount: _trackers.length,
                  itemBuilder: (_, i) {
                    final t = _trackers[i];
                    return ListTile(
                      dense: true,
                      title: Text(
                        t.url,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontSize: 13),
                      ),
                      trailing: IconButton(
                        icon: const Icon(Icons.delete_outline, size: 20),
                        onPressed: () => _remove(t.url),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}

String _fmtBytes(int n) {
  if (n <= 0) return '-';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  double v = n.toDouble();
  int i = 0;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return '${v.toStringAsFixed(1)} ${u[i]}';
}

String _fmtSpeed(int? n) {
  if (n == null || n <= 0) return '0 B/s';
  return '${_fmtBytes(n)}/s';
}

String _fmtIso(String iso) {
  final dt = DateTime.tryParse(iso);
  if (dt == null || dt.year < 1971) return '-';
  String p(int v) => v.toString().padLeft(2, '0');
  return '${dt.year}-${p(dt.month)}-${p(dt.day)} ${p(dt.hour)}:${p(dt.minute)}';
}

bool _isCompleted(String iso) =>
    iso.isNotEmpty && !iso.startsWith('0001') && !iso.startsWith('1970');

// 任务详情：概览信息 + qB 标志 + Peers/文件/Tracker 子入口。
class _DetailSheet extends StatelessWidget {
  final int id;
  final String type;
  final Torrent torrent;
  const _DetailSheet({
    required this.id,
    required this.type,
    required this.torrent,
  });

  Widget _kv(String k, String v) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 3),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 84,
          child: Text(
            k,
            style: const TextStyle(color: Colors.grey, fontSize: 12),
          ),
        ),
        Expanded(child: Text(v, style: const TextStyle(fontSize: 13))),
      ],
    ),
  );

  Widget _flag(String label, bool on) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
    decoration: BoxDecoration(
      borderRadius: BorderRadius.circular(6),
      color: on
          ? Colors.deepPurple.withValues(alpha: 0.18)
          : Colors.grey.withValues(alpha: 0.15),
    ),
    child: Text(
      label,
      style: TextStyle(
        fontSize: 11,
        color: on ? Colors.deepPurple : Colors.grey,
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
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            children: [
              _kv('大小', _fmtBytes(t.size)),
              _kv(
                '已下载/上传',
                '${_fmtBytes(t.downloaded)} / ${_fmtBytes(t.uploaded)}',
              ),
              _kv('分享率', t.ratio.toStringAsFixed(2)),
              _kv('进度', '${(t.progress * 100).toStringAsFixed(1)}%'),
              if (_dlCaps(type).swarm)
                _kv(
                  '可用性',
                  t.availability > 0 ? t.availability.toStringAsFixed(2) : '-',
                ),
              _kv('做种/下载', '${t.numSeeds} / ${t.numLeechs}'),
              if (_dlCaps(type).swarm)
                _kv('Swarm 做/下', '${t.numComplete} / ${t.numIncomplete}'),
              if (_dlCaps(type).swarm) _kv('队列位置', '${t.priority}'),
              if (t.addedAt.isNotEmpty) _kv('创建', _fmtIso(t.addedAt)),
              if (_isCompleted(t.completedAt))
                _kv('完成', _fmtIso(t.completedAt)),
              if (_dlCaps(type).forceStart || _dlCaps(type).sequential || _dlCaps(type).firstLast) ...[
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    _flag('强制开始', t.forceStart),
                    _flag('顺序下载', t.sequential),
                    _flag('首尾优先', t.firstLastPiece),
                    _flag('自动管理', t.autoTmm),
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

// Peers 列表。
class _PeersSheet extends StatefulWidget {
  final int id;
  final String hash;
  const _PeersSheet({required this.id, required this.hash});
  @override
  State<_PeersSheet> createState() => _PeersSheetState();
}

class _PeersSheetState extends State<_PeersSheet> {
  final _api = DownloaderApi();
  List<Peer> _peers = [];
  bool _loading = true;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final p = await _api.peers(widget.id, widget.hash);
      if (mounted) {
        setState(() {
          _peers = p;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(14),
          child: Row(
            children: [
              const Text(
                'Peers',
                style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
              ),
              const Spacer(),
              Text(
                '${_peers.length}',
                style: const TextStyle(color: Colors.grey, fontSize: 13),
              ),
              IconButton(
                icon: const Icon(Icons.refresh, size: 20),
                onPressed: _load,
              ),
            ],
          ),
        ),
        const Divider(height: 1),
        Expanded(
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _peers.isEmpty
              ? const Center(
                  child: Text('无连接的对等体', style: TextStyle(color: Colors.grey)),
                )
              : ListView.builder(
                  itemCount: _peers.length,
                  itemBuilder: (_, i) {
                    final p = _peers[i];
                    return ListTile(
                      dense: true,
                      title: Text(p.ip, style: const TextStyle(fontSize: 13)),
                      subtitle: Text(
                        '${p.client.isEmpty ? '-' : p.client} · ${(p.progress * 100).toStringAsFixed(0)}% · ↓${_fmtSpeed(p.dlRate)} ↑${_fmtSpeed(p.upRate)}',
                        style: const TextStyle(fontSize: 11),
                      ),
                      trailing: Text(
                        p.flags,
                        style: const TextStyle(
                          fontSize: 11,
                          color: Colors.grey,
                        ),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}

// 分类管理（qB）：列表 + 新建（含保存路径）/删除。
class _CategorySheet extends StatefulWidget {
  final int id;
  const _CategorySheet({required this.id});
  @override
  State<_CategorySheet> createState() => _CategorySheetState();
}

class _CategorySheetState extends State<_CategorySheet> {
  final _api = DownloaderApi();
  final _nameCtrl = TextEditingController();
  final _pathCtrl = TextEditingController();
  List<String> _cats = [];
  bool _loading = true;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final c = await _api.categories(widget.id);
      if (mounted) {
        setState(() {
          _cats = c;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _add() async {
    final name = _nameCtrl.text.trim();
    if (name.isEmpty) return;
    try {
      await _api.createCategory(widget.id, name, _pathCtrl.text.trim());
      _nameCtrl.clear();
      _pathCtrl.clear();
      _toast('已添加');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  Future<void> _del(String name) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除分类'),
        content: Text('确认删除分类「$name」？该分类下任务将变为无分类。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.deleteCategories(widget.id, [name]);
      _toast('已删除');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.all(14),
          child: Text(
            '分类管理',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _nameCtrl,
                  decoration: const InputDecoration(
                    hintText: '新分类名',
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                  onSubmitted: (_) => _add(),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: TextField(
                  controller: _pathCtrl,
                  decoration: const InputDecoration(
                    hintText: '保存路径（可空）',
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(icon: const Icon(Icons.add), onPressed: _add),
            ],
          ),
        ),
        const Divider(height: 16),
        Expanded(
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _cats.isEmpty
              ? const Center(
                  child: Text('无分类', style: TextStyle(color: Colors.grey)),
                )
              : ListView.builder(
                  itemCount: _cats.length,
                  itemBuilder: (_, i) {
                    final c = _cats[i];
                    return ListTile(
                      dense: true,
                      title: Text(c, style: const TextStyle(fontSize: 13)),
                      trailing: IconButton(
                        icon: const Icon(Icons.delete_outline, size: 20),
                        onPressed: () => _del(c),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}
