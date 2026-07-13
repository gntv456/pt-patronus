import 'package:flutter/material.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/transfer.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 媒体整理页（对齐 Web Transfer.vue）：下载完成 → 识别 → 重命名 → 硬链/移动 → 刮削入库。
/// 历史 + 配置 + 手动整理 + 扫描兜底。复用后端 /transfer/*。
class TransferPage extends StatefulWidget {
  const TransferPage({super.key});
  @override
  State<TransferPage> createState() => _TransferPageState();
}

class _TransferPageState extends State<TransferPage> {
  final _api = TransferApi();
  final _dlApi = DownloaderApi();

  List<TransferHistory> _list = [];
  bool _loading = true;
  final String _status = ''; // '' | success | failed
  final _redoing = <int>{};

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      _list = await _api.history(_status, 200);
    } catch (_) {
      _toast('加载失败');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _sweep() async {
    try {
      await _api.sweep();
      _toast('已触发扫描，稍后刷新');
      await Future.delayed(const Duration(seconds: 4));
      _load();
    } catch (_) {
      _toast('扫描失败');
    }
  }

  Future<void> _redo(TransferHistory h) async {
    setState(() => _redoing.add(h.id));
    try {
      final r = await _api.redo(h.id);
      final ok = r?['Success'] == true;
      _toast(ok ? '重做完成' : '重做未成功：${r?['ErrMsg'] ?? '未知'}');
      _load();
    } catch (_) {
      _toast('重做失败');
    } finally {
      if (mounted) setState(() => _redoing.remove(h.id));
    }
  }

  Future<void> _delete(TransferHistory h) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除历史'),
        content: Text('删除「${h.title}」？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.deleteHistory(h.id);
      _load();
    } catch (_) {
      _toast('删除失败');
    }
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg), duration: const Duration(seconds: 2)));
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: const Text('媒体整理', style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700)),
      actions: [
        PTBarAction(icon: Icons.refresh, label: '刷新', loading: _loading, onPressed: _loading ? null : _load),
        PTBarAction(icon: Icons.cleaning_services, label: '扫描兜底', onPressed: _sweep),
        PTBarAction(icon: Icons.post_add, label: '手动整理', onPressed: _openOrganize),
        PTBarAction(icon: Icons.settings, label: '整理设置', onPressed: _openConfig),
      ],
      body: RefreshIndicator(
        onRefresh: _load,
        child: _list.isEmpty && !_loading
            ? _empty()
            : ListView.separated(
                padding: const EdgeInsets.fromLTRB(14, 4, 14, 16),
                itemCount: _list.length,
                separatorBuilder: (_, _) => const SizedBox(height: 8),
                itemBuilder: (_, i) => _historyCard(_list[i]),
              ),
      ),
    );
  }

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

  Widget _miniTag(String t) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
      decoration: BoxDecoration(color: Theme.of(context).colorScheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(4)),
      child: Text(t, style: const TextStyle(fontSize: 10)),
    );
  }

  String _modeLabel(String m) => const {'link': '硬链', 'softlink': '软链', 'move': '移动', 'copy': '复制'}[m] ?? m;

  // 带标签的下拉（对齐代码库 DropdownButton + label 习惯，避开 DropdownButtonFormField.value 弃用）。
  Widget _drop(String label, String value, Map<String, String> opts, ValueChanged<String?> onChanged) {
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Padding(padding: const EdgeInsets.only(bottom: 4), child: Text(label, style: const TextStyle(fontSize: 13))),
      DropdownButton<String>(
        value: value, isExpanded: true,
        items: opts.entries.map((e) => DropdownMenuItem(value: e.key, child: Text(e.value))).toList(),
        onChanged: onChanged,
      ),
    ]);
  }

  String _fmtTime(String s) {
    final t = DateTime.tryParse(s);
    return t == null ? '' : '${t.month.toString().padLeft(2, '0')}-${t.day.toString().padLeft(2, '0')} ${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}';
  }

  // —— 配置 ——
  void _openConfig() async {
    final TransferConfig cfg;
    try {
      cfg = await _api.getConfig();
    } catch (_) {
      _toast('加载配置失败');
      return;
    }
    if (!mounted) return;
    final catsCtrl = TextEditingController(text: cfg.categories.join(','));
    final skipCtrl = TextEditingController(text: cfg.skipTags.join(','));
    final movieDirCtrl = TextEditingController(text: cfg.movieDir);
    final tvDirCtrl = TextEditingController(text: cfg.tvDir);
    final saved = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      builder: (ctx) => StatefulBuilder(builder: (ctx, setS) {
        return SafeArea(
          child: Padding(
            padding: EdgeInsets.fromLTRB(16, 12, 16, 16 + MediaQuery.of(ctx).viewInsets.bottom),
            child: SingleChildScrollView(
              child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                const Text('整理设置', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
                const SizedBox(height: 12),
                SwitchListTile(value: cfg.enabled, onChanged: (v) => setS(() => cfg.enabled = v), title: const Text('启用整理')),
                SwitchListTile(value: cfg.refreshLibrary, onChanged: (v) => setS(() => cfg.refreshLibrary = v), title: const Text('整理后扫库')),
                SwitchListTile(value: cfg.typeFolder, onChanged: (v) => setS(() => cfg.typeFolder = v), title: const Text('库下加 movie/tv 子目录')),
                SwitchListTile(value: cfg.overwriteFiles, onChanged: (v) => setS(() => cfg.overwriteFiles = v), title: const Text('覆盖已存在文件')),
                const SizedBox(height: 4),
                _drop('整理模式', cfg.mode, const {'link': '硬链接（保种，推荐）', 'softlink': '软链接', 'move': '移动（断种）', 'copy': '复制'},
                    (v) => setS(() => cfg.mode = v ?? 'link')),
                const SizedBox(height: 8),
                _drop('刮削策略', cfg.scrape, const {'missing_only': '仅缺失时写', 'overwrite': '覆盖', 'skip': '不刮削'},
                    (v) => setS(() => cfg.scrape = v ?? 'missing_only')),
                const SizedBox(height: 8),
                TextField(controller: movieDirCtrl, decoration: const InputDecoration(labelText: '电影库目录', isDense: true)),
                const SizedBox(height: 8),
                TextField(controller: tvDirCtrl, decoration: const InputDecoration(labelText: '剧集库目录', isDense: true)),
                const SizedBox(height: 8),
                TextField(controller: catsCtrl, decoration: const InputDecoration(labelText: '整理分类（逗号分隔）', isDense: true)),
                const SizedBox(height: 8),
                TextField(controller: skipCtrl, decoration: const InputDecoration(labelText: '跳过标签（逗号分隔）', isDense: true)),
                const SizedBox(height: 16),
                FilledButton(onPressed: () async {
                  cfg.movieDir = movieDirCtrl.text.trim();
                  cfg.tvDir = tvDirCtrl.text.trim();
                  cfg.categories = catsCtrl.text.split(',').map((s) => s.trim()).where((s) => s.isNotEmpty).toList();
                  cfg.skipTags = skipCtrl.text.split(',').map((s) => s.trim()).where((s) => s.isNotEmpty).toList();
                  try {
                    await _api.setConfig(cfg);
                    if (ctx.mounted) Navigator.pop(ctx, true);
                  } catch (_) {
                    _toast('保存失败');
                  }
                }, child: const Text('保存')),
              ]),
            ),
          ),
        );
      }),
    );
    catsCtrl.dispose();
    skipCtrl.dispose();
    movieDirCtrl.dispose();
    tvDirCtrl.dispose();
    if (saved == true) _toast('配置已保存');
  }

  // —— 手动整理：选下载器 → 选种子 ——
  void _openOrganize() async {
    List<Downloader> dls;
    try {
      dls = await _dlApi.list();
    } catch (_) {
      _toast('加载下载器失败');
      return;
    }
    if (!mounted) return;
    if (dls.isEmpty) {
      _toast('无下载器');
      return;
    }
    int selDl = dls.first.id;
    List<Torrent> torrents = [];
    bool loadingT = true;
    String selHash = '';

    Future<void> loadT(setS) async {
      setS(() => loadingT = true);
      try {
        torrents = await _dlApi.torrents(selDl);
      } catch (_) {
        torrents = [];
      }
      if (mounted) setS(() => loadingT = false);
    }

    await showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (ctx) => StatefulBuilder(builder: (ctx, setS) {
        if (loadingT && torrents.isEmpty) loadT(setS);
        return SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 12, 16, 16),
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              const Text('手动整理', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
              const SizedBox(height: 8),
              DropdownButton<int>(
                value: selDl, isExpanded: true,
                items: dls.map((d) => DropdownMenuItem(value: d.id, child: Text(d.name))).toList(),
                onChanged: (v) { if (v != null) { selDl = v; torrents = []; setS(() {}); loadT(setS); } },
              ),
              const SizedBox(height: 8),
              SizedBox(
                height: 320,
                child: loadingT
                    ? const Center(child: CircularProgressIndicator())
                    : torrents.isEmpty
                        ? const Center(child: Text('无种子'))
                        : ListView.builder(
                            itemCount: torrents.length,
                            itemBuilder: (_, i) {
                              final t = torrents[i];
                              final selected = t.hash == selHash;
                              return ListTile(
                                dense: true,
                                leading: Icon(selected ? Icons.check_circle : Icons.radio_button_unchecked, size: 20, color: selected ? PTColors.success : null),
                                title: Text(t.name, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 13)),
                                subtitle: Text('${t.category} · ${t.state}', style: const TextStyle(fontSize: 11)),
                                selected: selected,
                                onTap: () => setS(() => selHash = t.hash),
                              );
                            },
                          ),
              ),
              FilledButton(
                onPressed: selHash.isEmpty
                    ? null
                    : () async {
                        try {
                          final r = await _api.organize(selDl, selHash);
                          final ok = r?['Success'] == true;
                          _toast(ok ? '整理完成：${(r?['Files'] as List?)?.length ?? 0} 文件' : '未成功：${r?['ErrMsg'] ?? '未知'}');
                          if (ctx.mounted) Navigator.pop(ctx);
                          _load();
                        } catch (_) {
                          _toast('整理失败');
                        }
                      },
                child: const Text('整理'),
              ),
            ]),
          ),
        );
      }),
    );
  }
}
