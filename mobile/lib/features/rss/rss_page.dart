import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/rss.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/filter_rules.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// RSS 订阅页：对齐 Web Rss.vue。订阅卡片列表 + 检查/条目/编辑/删除；
/// 表单走自适应浮层（宽屏对话框 / 窄屏底栏），含体积范围(min_size/max_size，修编辑丢数据)。
class RssPage extends StatefulWidget {
  const RssPage({super.key});
  @override
  State<RssPage> createState() => _RssPageState();
}

class _RssPageState extends State<RssPage> {
  final _api = RssApi();
  final _filterApi = FilterRuleApi();
  List<RssSubscription> _list = [];
  List<Site> _sites = const [];
  List<Downloader> _downloaders = const [];
  List<String> _filterGroups = const [];
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
      if (_sites.isEmpty) {
        _sites = await SiteApi().list().catchError((_) => <Site>[]);
      }
      if (_downloaders.isEmpty) {
        _downloaders = await DownloaderApi().list().catchError(
          (_) => <Downloader>[],
        );
      }
      if (_filterGroups.isEmpty) {
        final gs = await _filterApi.groups().catchError((_) => <FilterRuleGroup>[]);
        _filterGroups = gs.map((g) => g.name).toList();
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
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
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
    final saved = await showAdaptiveSheet<bool>(
      context,
      title: r == null ? '添加订阅' : '编辑订阅',
      wideWidth: 560,
      builder: (ctx) => _RssForm(
        editing: r,
        sites: _sites,
        downloaders: _downloaders,
        filterGroups: _filterGroups,
        onSaved: () => Navigator.pop(ctx, true),
      ),
    );
    if (saved == true) _load();
  }

  Future<void> _openItems(RssSubscription r) async {
    List<RssItem> items;
    try {
      items = await _api.items(r.id);
    } catch (e) {
      _toast('$e');
      return;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '条目：${r.name}',
      wideWidth: 720,
      builder: (_) => items.isEmpty
          ? const PTEmptyState(text: '暂无条目')
          : PTAdaptiveTable<RssItem>(
              items: items,
              columns: [
                PTColumn(
                  label: '标题',
                  flex: 4,
                  cell: (c, it) => it.link.isNotEmpty
                      ? InkWell(
                          onTap: () => _launchUrl(it.link),
                          child: Text(
                            it.title,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              fontSize: 13,
                              color: PTColors.primary2,
                              decoration: TextDecoration.underline,
                            ),
                          ),
                        )
                      : Text(
                          it.title,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontSize: 13),
                        ),
                ),
                PTColumn(
                  label: '大小',
                  flex: 1,
                  numeric: true,
                  cell: (_, it) => Text(
                    _fmtSize(it.size),
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
                PTColumn(
                  label: '已下载',
                  flex: 1,
                  cell: (ctx, it) => Text(
                    it.downloaded ? '是' : '否',
                    style: TextStyle(
                      fontSize: 12,
                      color: it.downloaded ? PTColors.success : ctx.dimText,
                    ),
                  ),
                ),
                PTColumn(
                  label: '发现时间',
                  flex: 2,
                  cell: (ctx, it) => Text(
                    _fmtTime(it.createdAt),
                    style: TextStyle(fontSize: 12, color: ctx.dimText),
                  ),
                ),
              ],
              emptyText: '暂无条目',
              narrowCard: (ctx, it) => GlassCard(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (it.link.isNotEmpty)
                      InkWell(
                        onTap: () => _launchUrl(it.link),
                        child: Text(
                          it.title,
                          style: const TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w600,
                            color: PTColors.primary2,
                          ),
                        ),
                      )
                    else
                      Text(
                        it.title,
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    const SizedBox(height: 4),
                    Text(
                      '${_fmtSize(it.size)} · ${it.downloaded ? "已下载" : "未下载"} · ${_fmtTime(it.createdAt)}',
                      style: TextStyle(fontSize: 12, color: ctx.dimText),
                    ),
                  ],
                ),
              ),
            ),
    );
  }

  void _launchUrl(String url) {
    // 移动端无内嵌浏览器跳转需求时仅复制/忽略；保持与 Web 新标签打开一致的最小实现。
    _toast(url);
  }

  String _fmtTime(String? t) => t == null
      ? '—'
      : t.substring(0, t.length > 16 ? 16 : t.length).replaceAll('T', ' ');
  String _fmtSize(int b) {
    if (b <= 0) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    double n = b.toDouble();
    int u = 0;
    while (n >= 1024 && u < units.length - 1) {
      n /= 1024;
      u++;
    }
    return u <= 1
        ? '${n.toStringAsFixed(0)} ${units[u]}'
        : '${n.toStringAsFixed(2)} ${units[u]}';
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: 'RSS 订阅',
      actions: [
        PTBarAction(onPressed: _load, icon: Icons.refresh, label: '刷新'),
        PTBarAction(
          onPressed: () => _openForm(),
          icon: Icons.add,
          label: '添加订阅',
        ),
      ],
      body: _loading && _list.isEmpty
          ? const PTLoadingView()
          : _list.isEmpty
          ? RefreshIndicator(
              onRefresh: _load,
              child: PTEmptyState(
                icon: Icons.rss_feed,
                text: '暂无 RSS 订阅',
                description: '订阅 RSS/Atom 源，按关键词/大小/分辨率过滤后自动下载',
                actionLabel: '添加订阅',
                onAction: () => _openForm(),
              ),
            )
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
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              Expanded(
                                child: Text(
                                  r.name,
                                  style: const TextStyle(
                                    fontSize: 16,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                              _chip(
                                r.enabled ? '启用' : '停用',
                                r.enabled ? PTColors.success : context.dimText,
                              ),
                              if (r.autoDownload) _chip('自动下载', PTColors.warn),
                              if (r.resolution.isNotEmpty)
                                _chip(r.resolution, PTColors.primary2),
                            ],
                          ),
                          const SizedBox(height: 4),
                          Text(
                            r.url,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 12,
                              color: context.dimText,
                            ),
                          ),
                          Text(
                            '间隔 ${r.intervalMin} 分钟 · 上次 ${_fmtTime(r.lastCheckedAt)}${r.titleInclude.isNotEmpty ? ' · 包含: ${r.titleInclude}' : ''}',
                            style: TextStyle(
                              fontSize: 12,
                              color: context.dimText,
                            ),
                          ),
                          const SizedBox(height: 8),
                          Wrap(
                            spacing: 8,
                            children: [
                              FilledButton.icon(
                                onPressed: _checking == r.id
                                    ? null
                                    : () => _check(r),
                                icon: _checking == r.id
                                    ? const SizedBox(
                                        width: 14,
                                        height: 14,
                                        child: CircularProgressIndicator(
                                          strokeWidth: 2,
                                        ),
                                      )
                                    : const Icon(Icons.refresh, size: 16),
                                label: const Text('检查'),
                              ),
                              OutlinedButton(
                                onPressed: () => _openItems(r),
                                child: const Text('条目'),
                              ),
                              OutlinedButton(
                                onPressed: () => _openForm(r),
                                child: const Text('编辑'),
                              ),
                              TextButton(
                                onPressed: () => _delete(r),
                                child: const Text(
                                  '删除',
                                  style: TextStyle(color: PTColors.danger),
                                ),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  );
                },
              ),
            ),
    );
  }

  Widget _chip(String text, Color c) => Container(
    margin: const EdgeInsets.only(left: 4),
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
    decoration: BoxDecoration(
      color: c.withValues(alpha: 0.18),
      borderRadius: BorderRadius.circular(999),
    ),
    child: Text(
      text,
      style: TextStyle(color: c, fontSize: 12, fontWeight: FontWeight.w600),
    ),
  );
}

/// 新增/编辑订阅表单（自适应浮层内容）。含体积范围 min_size/max_size（修编辑丢数据 bug）。
class _RssForm extends StatefulWidget {
  final RssSubscription? editing;
  final List<Site> sites;
  final List<Downloader> downloaders;
  final List<String> filterGroups;
  final VoidCallback onSaved;
  const _RssForm({
    this.editing,
    required this.sites,
    required this.downloaders,
    required this.filterGroups,
    required this.onSaved,
  });
  @override
  State<_RssForm> createState() => _RssFormState();
}

class _RssFormState extends State<_RssForm> {
  final _api = RssApi();
  final _formKey = GlobalKey<FormState>();
  late final _name = TextEditingController(text: widget.editing?.name ?? '');
  late final _url = TextEditingController(text: widget.editing?.url ?? '');
  late final _include = TextEditingController(
    text: widget.editing?.titleInclude ?? '',
  );
  late final _exclude = TextEditingController(
    text: widget.editing?.titleExclude ?? '',
  );
  late final _tags = TextEditingController(text: widget.editing?.tags ?? '');
  late final _path = TextEditingController(
    text: widget.editing?.savePath ?? '',
  );
  late final _category = TextEditingController(
    text: widget.editing?.category ?? 'rss',
  );
  late final _minSize = TextEditingController(
    text: '${widget.editing?.minSize ?? 0}',
  );
  late final _maxSize = TextEditingController(
    text: '${widget.editing?.maxSize ?? 0}',
  );
  late final _interval = TextEditingController(
    text: '${widget.editing?.intervalMin ?? 30}',
  );
  int _siteId = 0;
  int _downloaderId = 0;
  String _resolution = '';
  bool _enabled = true;
  bool _autoDownload = false;
  bool _saving = false;
  // 选中的过滤规则组名（提交时 join('|')）。
  final _selectedGroups = <String>{};

  @override
  void initState() {
    super.initState();
    final e = widget.editing;
    if (e != null) {
      _siteId = e.siteId;
      _downloaderId = e.downloaderId;
      _resolution = e.resolution;
      _enabled = e.enabled;
      _autoDownload = e.autoDownload;
      _selectedGroups.addAll(
          e.filterGroups.split('|').map((s) => s.trim()).where((s) => s.isNotEmpty));
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
      'interval_min': int.tryParse(_interval.text.trim()) ?? 30,
      'title_include': _include.text.trim(),
      'title_exclude': _exclude.text.trim(),
      'resolution': _resolution,
      'min_size': int.tryParse(_minSize.text.trim()) ?? 0,
      'max_size': int.tryParse(_maxSize.text.trim()) ?? 0,
      'auto_download': _autoDownload,
      'downloader_id': _downloaderId,
      'save_path': _path.text.trim(),
      'category': _category.text.trim(),
      'tags': _tags.text.trim(),
      'filter_groups': _selectedGroups.join('|'),
    };
    try {
      if (widget.editing != null) {
        await _api.update(widget.editing!.id, body);
      } else {
        await _api.create(body);
      }
      widget.onSaved();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('$e')));
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Form(
      key: _formKey,
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextFormField(
              controller: _name,
              decoration: const InputDecoration(labelText: '名称'),
              validator: (v) => (v == null || v.trim().isEmpty) ? '必填' : null,
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _url,
              decoration: const InputDecoration(
                labelText: 'URL',
                hintText: 'https://site.com/rss.xml',
              ),
              validator: (v) => (v == null || v.trim().isEmpty) ? '必填' : null,
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<int>(
              initialValue: _siteId,
              isExpanded: true,
              decoration: const InputDecoration(labelText: '关联站点'),
              items: [
                const DropdownMenuItem(value: 0, child: Text('不关联（匿名 RSS）')),
                ...widget.sites.map(
                  (s) => DropdownMenuItem(value: s.id, child: Text(s.name)),
                ),
              ],
              onChanged: (v) => setState(() => _siteId = v ?? 0),
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _interval,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(labelText: '间隔（分钟，最小 5）'),
              validator: (v) {
                final n = int.tryParse(v ?? '');
                if (n == null || n < 5) return '最小 5 分钟';
                return null;
              },
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _include,
              decoration: const InputDecoration(labelText: '标题包含（| 分隔）'),
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _exclude,
              decoration: const InputDecoration(labelText: '标题排除（| 分隔）'),
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<String>(
              initialValue: _resolution,
              isExpanded: true,
              decoration: const InputDecoration(labelText: '分辨率'),
              items: const [
                DropdownMenuItem(value: '', child: Text('不限')),
                DropdownMenuItem(value: '1080p', child: Text('1080p')),
                DropdownMenuItem(value: '2160p', child: Text('2160p')),
              ],
              onChanged: (v) => setState(() => _resolution = v ?? ''),
            ),
            const SizedBox(height: 10),
            Align(
              alignment: Alignment.centerLeft,
              child: Text(
                '过滤规则组（留空=不过滤）',
                style: TextStyle(fontSize: 12, color: context.dimText),
              ),
            ),
            const SizedBox(height: 6),
            widget.filterGroups.isEmpty
                ? Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: Text(
                      '暂无可用规则组，可在「过滤规则」页创建',
                      style: TextStyle(fontSize: 12, color: context.dimText),
                    ),
                  )
                : Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: Wrap(
                      spacing: 8,
                      runSpacing: 4,
                      children: [
                        for (final g in widget.filterGroups)
                          FilterChip(
                            label: Text(g),
                            selected: _selectedGroups.contains(g),
                            onSelected: (v) => setState(() => v
                                ? _selectedGroups.add(g)
                                : _selectedGroups.remove(g)),
                          ),
                      ],
                    ),
                  ),
            Row(
              children: [
                Expanded(
                  child: TextFormField(
                    controller: _minSize,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    decoration: const InputDecoration(labelText: '最小体积'),
                  ),
                ),
                const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 8),
                  child: Text('~'),
                ),
                Expanded(
                  child: TextFormField(
                    controller: _maxSize,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    decoration: const InputDecoration(labelText: '最大体积'),
                  ),
                ),
              ],
            ),
            Padding(
              padding: const EdgeInsets.only(top: 2, bottom: 8),
              child: Text(
                '字节（0=不限）',
                style: TextStyle(fontSize: 11, color: context.dimText),
              ),
            ),
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
              DropdownButtonFormField<int>(
                initialValue: _downloaderId,
                isExpanded: true,
                decoration: const InputDecoration(labelText: '下载器'),
                items: [
                  const DropdownMenuItem(value: 0, child: Text('默认下载器')),
                  ...widget.downloaders.map(
                    (d) => DropdownMenuItem(value: d.id, child: Text(d.name)),
                  ),
                ],
                onChanged: (v) => setState(() => _downloaderId = v ?? 0),
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _path,
                decoration: const InputDecoration(labelText: '保存路径'),
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _category,
                decoration: const InputDecoration(labelText: '分类'),
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _tags,
                decoration: const InputDecoration(labelText: '标签（逗号分隔）'),
              ),
            ],
            const SizedBox(height: 16),
            FilledButton(
              onPressed: _saving ? null : _save,
              child: _saving
                  ? const SizedBox(
                      height: 18,
                      width: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('保存'),
            ),
          ],
        ),
      ),
    );
  }
}
