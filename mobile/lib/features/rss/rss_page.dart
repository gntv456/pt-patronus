import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/rss.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/theme/glass.dart';

class RssPage extends StatefulWidget {
  const RssPage({super.key});
  @override
  State<RssPage> createState() => _RssPageState();
}

class _RssPageState extends State<RssPage> {
  final _api = RssApi();
  List<RssSubscription> _list = [];
  List<Site> _sites = const [];
  List<Downloader> _downloaders = const [];
  bool _loading = false;
  int? _checking;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      _list = await _api.list();
      if (_sites.isEmpty) _sites = await SiteApi().list().catchError((_) => <Site>[]);
      if (_downloaders.isEmpty) {
        _downloaders = await DownloaderApi().list().catchError((_) => <Downloader>[]);
      }
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _check(RssSubscription r) async {
    setState(() => _checking = r.id);
    try {
      final n = await _api.check(r.id);
      _toast('新增 $n 条');
      await _load();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _checking = null);
    }
  }

  Future<void> _delete(RssSubscription r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('确认删除'),
        content: Text('删除 RSS「${r.name}」？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(r.id);
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _openForm([RssSubscription? r]) async {
    final res = await Navigator.of(context).push<bool>(
      MaterialPageRoute(builder: (_) => _RssFormPage(
            editing: r,
            sites: _sites,
            downloaders: _downloaders,
          )),
    );
    if (res == true) _load();
  }

  Future<void> _openItems(RssSubscription r) async {
    final items = await _api.items(r.id);
    if (!mounted) return;
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => DraggableScrollableSheet(
        expand: false,
        initialChildSize: 0.6,
        builder: (context, controller) => ListView(
          controller: controller,
          padding: const EdgeInsets.all(12),
          children: [
            Text('条目：${r.name}',
                style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
            const SizedBox(height: 8),
            ...items.map((it) => ListTile(
                  dense: true,
                  title: Text(it.title, maxLines: 2, overflow: TextOverflow.ellipsis),
                  subtitle: Text('${_fmtSize(it.size)} · ${it.downloaded ? "已下载" : "未下载"}'),
                  trailing: it.link.isNotEmpty
                      ? const Icon(Icons.open_in_new, size: 16, color: Colors.grey)
                      : null,
                )),
          ],
        ),
      ),
    );
  }

  String _fmtTime(String? t) =>
      t == null ? '—' : t.substring(0, t.length > 16 ? 16 : t.length).replaceAll('T', ' ');
  String _fmtSize(int b) {
    if (b <= 0) return '—';
    if (b >= 1 << 30) return '${(b / (1 << 30)).toStringAsFixed(2)} GB';
    if (b >= 1 << 20) return '${(b / (1 << 20)).toStringAsFixed(1)} MB';
    return '$b B';
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('RSS 订阅'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [IconButton(onPressed: () => _openForm(), icon: const Icon(Icons.add))],
        ),
        body: _loading && _list.isEmpty
            ? const Center(child: CircularProgressIndicator())
            : _list.isEmpty
                ? const Center(child: Text('暂无 RSS 订阅', style: TextStyle(color: Colors.grey)))
                : RefreshIndicator(
                    onRefresh: _load,
                    child: ListView.builder(
                      padding: const EdgeInsets.all(12),
                      itemCount: _list.length,
                      itemBuilder: (_, i) {
                        final r = _list[i];
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 10),
                          child: GlassCard(
                            padding: const EdgeInsets.all(14),
                            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                              Row(children: [
                                Expanded(
                                  child: Text(r.name,
                                      style: const TextStyle(
                                          fontSize: 16, fontWeight: FontWeight.w700)),
                                ),
                                _chip(r.enabled ? '启用' : '停用',
                                    r.enabled ? PTColors.success : Colors.grey),
                                if (r.autoDownload) _chip('自动下载', PTColors.warn),
                                if (r.resolution.isNotEmpty) _chip(r.resolution, PTColors.primary2),
                              ]),
                              const SizedBox(height: 4),
                              Text(r.url,
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  style: const TextStyle(fontSize: 12, color: Colors.grey)),
                              Text(
                                  '间隔 ${r.intervalMin} 分钟 · 上次 ${_fmtTime(r.lastCheckedAt)}',
                                  style: const TextStyle(fontSize: 12, color: Colors.grey)),
                              const SizedBox(height: 8),
                              Wrap(spacing: 8, children: [
                                FilledButton.icon(
                                  onPressed: _checking == r.id ? null : () => _check(r),
                                  icon: _checking == r.id
                                      ? const SizedBox(
                                          width: 14,
                                          height: 14,
                                          child: CircularProgressIndicator(strokeWidth: 2))
                                      : const Icon(Icons.refresh, size: 16),
                                  label: const Text('检查'),
                                ),
                                OutlinedButton(
                                    onPressed: () => _openItems(r), child: const Text('条目')),
                                OutlinedButton(
                                    onPressed: () => _openForm(r), child: const Text('编辑')),
                                TextButton(
                                  onPressed: () => _delete(r),
                                  child: const Text('删除',
                                      style: TextStyle(color: PTColors.danger)),
                                ),
                              ]),
                            ]),
                          ),
                        );
                      },
                    ),
                  ),
      ),
    );
  }

  Widget _chip(String text, Color c) => Container(
        margin: const EdgeInsets.only(left: 4),
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
        decoration: BoxDecoration(color: c.withValues(alpha: 0.18), borderRadius: BorderRadius.circular(999)),
        child: Text(text, style: TextStyle(color: c, fontSize: 12, fontWeight: FontWeight.w600)),
      );
}

/// 新增/编辑订阅表单页。
class _RssFormPage extends StatefulWidget {
  final RssSubscription? editing;
  final List<Site> sites;
  final List<Downloader> downloaders;
  const _RssFormPage({this.editing, required this.sites, required this.downloaders});
  @override
  State<_RssFormPage> createState() => _RssFormPageState();
}

class _RssFormPageState extends State<_RssFormPage> {
  final _api = RssApi();
  final _formKey = GlobalKey<FormState>();
  late final _name = TextEditingController(text: widget.editing?.name ?? '');
  late final _url = TextEditingController(text: widget.editing?.url ?? '');
  late final _include = TextEditingController(text: widget.editing?.titleInclude ?? '');
  late final _exclude = TextEditingController(text: widget.editing?.titleExclude ?? '');
  late final _tags = TextEditingController(text: widget.editing?.tags ?? '');
  late final _path = TextEditingController(text: widget.editing?.savePath ?? '');
  late final _category = TextEditingController(text: widget.editing?.category ?? 'rss');
  int _siteId = 0;
  int _downloaderId = 0;
  int _interval = 30;
  String _resolution = '';
  bool _enabled = true;
  bool _autoDownload = false;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final e = widget.editing;
    if (e != null) {
      _siteId = e.siteId;
      _downloaderId = e.downloaderId;
      _interval = e.intervalMin;
      _resolution = e.resolution;
      _enabled = e.enabled;
      _autoDownload = e.autoDownload;
    }
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _saving = true);
    final body = {
      'name': _name.text.trim(),
      'url': _url.text.trim(),
      'site_id': _siteId,
      'enabled': _enabled,
      'interval_min': _interval,
      'title_include': _include.text.trim(),
      'title_exclude': _exclude.text.trim(),
      'resolution': _resolution,
      'auto_download': _autoDownload,
      'downloader_id': _downloaderId,
      'save_path': _path.text.trim(),
      'category': _category.text.trim(),
      'tags': _tags.text.trim(),
    };
    try {
      if (widget.editing != null) {
        await _api.update(widget.editing!.id, body);
      } else {
        await _api.create(body);
      }
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          title: Text(widget.editing == null ? '添加订阅' : '编辑订阅'),
          backgroundColor: Colors.transparent,
          elevation: 0,
        ),
        body: Form(
          key: _formKey,
          child: ListView(
            padding: const EdgeInsets.all(14),
            children: [
              TextFormField(
                controller: _name,
                decoration: const InputDecoration(labelText: '名称', border: OutlineInputBorder()),
                validator: (v) => (v == null || v.trim().isEmpty) ? '必填' : null,
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _url,
                decoration:
                    const InputDecoration(labelText: 'URL', border: OutlineInputBorder()),
                validator: (v) => (v == null || v.trim().isEmpty) ? '必填' : null,
              ),
              const SizedBox(height: 10),
              Row(children: [
                const Text('关联站点：'),
                Expanded(
                  child: DropdownButton<int>(
                    isExpanded: true,
                    value: _siteId,
                    items: [
                      const DropdownMenuItem(value: 0, child: Text('不关联（匿名 RSS）')),
                      ...widget.sites.map((s) => DropdownMenuItem(value: s.id, child: Text(s.name))),
                    ],
                    onChanged: (v) => setState(() => _siteId = v ?? 0),
                  ),
                ),
              ]),
              const SizedBox(height: 10),
              Row(children: [
                const Text('间隔(分钟)'),
                const SizedBox(width: 12),
                Expanded(
                  child: Slider(
                    value: _interval.toDouble(),
                    min: 5,
                    max: 240,
                    divisions: 47,
                    label: '$_interval',
                    onChanged: (v) => setState(() => _interval = v.round()),
                  ),
                ),
                Text('$_interval'),
              ]),
              const SizedBox(height: 10),
              TextFormField(
                controller: _include,
                decoration: const InputDecoration(
                    labelText: '标题包含（| 分隔）', border: OutlineInputBorder()),
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _exclude,
                decoration: const InputDecoration(
                    labelText: '标题排除（| 分隔）', border: OutlineInputBorder()),
              ),
              const SizedBox(height: 10),
              Row(children: [
                const Text('分辨率：'),
                Expanded(
                  child: DropdownButton<String>(
                    isExpanded: true,
                    value: _resolution,
                    items: const [
                      DropdownMenuItem(value: '', child: Text('不限')),
                      DropdownMenuItem(value: '1080p', child: Text('1080p')),
                      DropdownMenuItem(value: '2160p', child: Text('2160p')),
                    ],
                    onChanged: (v) => setState(() => _resolution = v ?? ''),
                  ),
                ),
              ]),
              const SizedBox(height: 6),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('启用'),
                value: _enabled,
                onChanged: (v) => setState(() => _enabled = v),
              ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('自动下载'),
                value: _autoDownload,
                onChanged: (v) => setState(() => _autoDownload = v),
              ),
              if (_autoDownload) ...[
                Row(children: [
                  const Text('下载器：'),
                  Expanded(
                    child: DropdownButton<int>(
                      isExpanded: true,
                      value: _downloaderId,
                      items: [
                        const DropdownMenuItem(value: 0, child: Text('默认下载器')),
                        ...widget.downloaders
                            .map((d) => DropdownMenuItem(value: d.id, child: Text(d.name))),
                      ],
                      onChanged: (v) => setState(() => _downloaderId = v ?? 0),
                    ),
                  ),
                ]),
                const SizedBox(height: 10),
                TextFormField(
                  controller: _category,
                  decoration: const InputDecoration(labelText: '分类', border: OutlineInputBorder()),
                ),
                const SizedBox(height: 10),
                TextFormField(
                  controller: _path,
                  decoration:
                      const InputDecoration(labelText: '保存路径', border: OutlineInputBorder()),
                ),
                const SizedBox(height: 10),
                TextFormField(
                  controller: _tags,
                  decoration: const InputDecoration(labelText: '标签（逗号分隔）', border: OutlineInputBorder()),
                ),
              ],
              const SizedBox(height: 16),
              FilledButton(
                onPressed: _saving ? null : _save,
                child: _saving
                    ? const SizedBox(
                        height: 18, width: 18, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Text('保存'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
